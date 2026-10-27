package persons

import (
	"context"
	"errors"

	"atm-identity-api/internal/apisperu"
	"atm-identity-api/internal/platform/apierror"

	"golang.org/x/sync/singleflight"
)

const (
	SourceDatabase = "database"
	SourceUpstream = "apisperu"
)

type Service struct {
	repo     *Repository
	upstream apisperu.Lookup
	flight   singleflight.Group
}

func NewService(repo *Repository, upstream apisperu.Lookup) *Service {
	return &Service{repo: repo, upstream: upstream}
}

// Resolve returns the person and where it came from. Database first; APIs Peru
// only when the document is unknown (or refresh is requested).
func (s *Service) Resolve(ctx context.Context, documentNumber string, refresh bool) (*Person, string, error) {
	if !refresh {
		p, err := s.repo.Find(ctx, documentNumber)
		if err == nil {
			return p, SourceDatabase, nil
		}
		if !errors.Is(err, errNotFound) {
			return nil, "", apierror.Internal(err)
		}
	}

	// One upstream call per document, even if several systems ask at once.
	// The shared fetch must not die with the first caller's request context.
	res, err, _ := s.flight.Do(documentNumber, func() (any, error) {
		return s.fetch(context.WithoutCancel(ctx), documentNumber, refresh)
	})
	if err != nil {
		return nil, "", err
	}
	return res.(*Person), SourceUpstream, nil
}

func (s *Service) fetch(ctx context.Context, documentNumber string, refresh bool) (*Person, error) {
	u, err := s.upstream.Person(ctx, documentNumber)
	switch {
	case errors.Is(err, apisperu.ErrNotFound):
		return nil, apierror.DocumentNotFound()
	case errors.Is(err, apisperu.ErrTimeout):
		return nil, apierror.UpstreamTimeout(err)
	case err != nil:
		return nil, apierror.UpstreamFailed(err)
	}

	if err := s.repo.InsertUpstream(ctx, u); err != nil {
		return nil, apierror.Internal(err)
	}
	if refresh {
		if err := s.repo.RefreshUpstream(ctx, u); err != nil {
			return nil, apierror.Internal(err)
		}
	}
	p, err := s.repo.Find(ctx, documentNumber)
	if err != nil {
		return nil, apierror.Internal(err)
	}
	return p, nil
}
