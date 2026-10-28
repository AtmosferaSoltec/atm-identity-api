package companies

import (
	"context"
	"errors"
	"time"

	"atm-identity-api/internal/apisperu"
	"atm-identity-api/internal/lookup"
	"atm-identity-api/internal/platform/apierror"

	"golang.org/x/sync/singleflight"
)

const (
	SourceDatabase = "database"
	SourceUpstream = "apisperu"

	documentType = "ruc"
)

type Service struct {
	repo     *Repository
	upstream apisperu.Lookup
	guard    *lookup.Guard
	flight   singleflight.Group
}

func NewService(repo *Repository, upstream apisperu.Lookup, guard *lookup.Guard) *Service {
	return &Service{repo: repo, upstream: upstream, guard: guard}
}

// Resolve returns the company and where it came from. Database first; APIs Peru
// only when the document is unknown, or when refresh is requested for a record
// older than lookup.RefreshCooldown.
func (s *Service) Resolve(ctx context.Context, documentNumber string, refresh bool) (*Company, string, error) {
	existing, err := s.repo.Find(ctx, documentNumber)
	found := err == nil
	if err != nil && !errors.Is(err, errNotFound) {
		return nil, "", apierror.Internal(err)
	}
	if found && (!refresh || time.Since(existing.LookedUpAt) < lookup.RefreshCooldown) {
		return existing, SourceDatabase, nil
	}
	if !found {
		miss, err := s.guard.IsKnownMiss(ctx, documentType, documentNumber)
		if err != nil {
			return nil, "", apierror.Internal(err)
		}
		if miss {
			return nil, "", apierror.DocumentNotFound()
		}
	}

	// One upstream call per document, even if several systems ask at once.
	// The shared fetch must not die with the first caller's request context.
	res, err, _ := s.flight.Do(documentNumber, func() (any, error) {
		return s.fetch(context.WithoutCancel(ctx), documentNumber, found)
	})
	if err != nil {
		return nil, "", err
	}
	return res.(*Company), SourceUpstream, nil
}

func (s *Service) fetch(ctx context.Context, documentNumber string, refresh bool) (*Company, error) {
	if err := s.guard.Reserve(ctx); err != nil {
		if errors.Is(err, lookup.ErrQuotaReached) {
			return nil, lookup.MapError(err)
		}
		return nil, apierror.Internal(err)
	}

	u, err := s.upstream.Company(ctx, documentNumber)
	if errors.Is(err, apisperu.ErrNotFound) {
		if err := s.guard.RecordMiss(ctx, documentType, documentNumber); err != nil {
			return nil, apierror.Internal(err)
		}
	}
	if err != nil {
		return nil, lookup.MapError(err)
	}

	if err := s.repo.InsertUpstream(ctx, u); err != nil {
		return nil, apierror.Internal(err)
	}
	if refresh {
		if err := s.repo.RefreshUpstream(ctx, u); err != nil {
			return nil, apierror.Internal(err)
		}
	}
	c, err := s.repo.Find(ctx, documentNumber)
	if err != nil {
		return nil, apierror.Internal(err)
	}
	return c, nil
}
