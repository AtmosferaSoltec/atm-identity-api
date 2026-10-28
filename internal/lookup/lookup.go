// Package lookup holds the rules shared by every upstream lookup: the monthly
// quota guard, the negative cache for nonexistent documents, and the mapping of
// provider errors to API errors.
package lookup

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"atm-identity-api/internal/apisperu"
	"atm-identity-api/internal/platform/apierror"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	// MissTTL is how long a nonexistent document is remembered.
	MissTTL = 24 * time.Hour
	// RefreshCooldown is the minimum age of a record before ?refresh=true may call the provider again.
	RefreshCooldown = 24 * time.Hour
	// warnRatio is the share of the monthly limit at which a warning is logged.
	warnRatio = 0.8
)

// ErrQuotaReached means our own monthly limit for the provider is used up.
var ErrQuotaReached = errors.New("lookup: monthly provider limit reached")

type Guard struct {
	pool     *pgxpool.Pool
	provider string
	limit    int
}

func NewGuard(pool *pgxpool.Pool, provider string, monthlyLimit int) *Guard {
	return &Guard{pool: pool, provider: provider, limit: monthlyLimit}
}

// Reserve counts one upstream call for the current month (America/Lima) and
// fails with ErrQuotaReached, without counting, once the limit is used up. It is
// a single atomic statement, so concurrent callers can never overshoot the limit.
func (g *Guard) Reserve(ctx context.Context) error {
	var calls int
	err := g.pool.QueryRow(ctx, `
		INSERT INTO provider_usage (provider, month, calls)
		SELECT $1, date_trunc('month', now() AT TIME ZONE 'America/Lima')::date, 1
		WHERE $2 > 0
		ON CONFLICT (provider, month) DO UPDATE
			SET calls = provider_usage.calls + 1, updated_at = now()
			WHERE provider_usage.calls < $2
		RETURNING calls`, g.provider, g.limit).Scan(&calls)
	if errors.Is(err, pgx.ErrNoRows) {
		slog.Error("provider monthly limit reached", "provider", g.provider, "limit", g.limit)
		return ErrQuotaReached
	}
	if err != nil {
		return err
	}
	switch {
	case calls == g.limit:
		slog.Warn("provider monthly limit used up", "provider", g.provider, "calls", calls, "limit", g.limit)
	case calls == int(float64(g.limit)*warnRatio):
		slog.Warn("provider monthly limit at 80%", "provider", g.provider, "calls", calls, "limit", g.limit)
	}
	return nil
}

// IsKnownMiss reports whether the document was recently found to not exist.
func (g *Guard) IsKnownMiss(ctx context.Context, documentType, documentNumber string) (bool, error) {
	var hit bool
	err := g.pool.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM lookup_misses WHERE document_type = $1 AND document_number = $2 AND expires_at > now())`,
		documentType, documentNumber).Scan(&hit)
	return hit, err
}

// RecordMiss remembers a nonexistent document for MissTTL.
func (g *Guard) RecordMiss(ctx context.Context, documentType, documentNumber string) error {
	_, err := g.pool.Exec(ctx, `
		INSERT INTO lookup_misses (document_type, document_number, expires_at)
		VALUES ($1, $2, now() + $3::interval)
		ON CONFLICT (document_type, document_number) DO UPDATE SET expires_at = EXCLUDED.expires_at`,
		documentType, documentNumber, MissTTL.String())
	return err
}

// MapError converts a provider or guard error into the API error to return.
func MapError(err error) *apierror.Error {
	switch {
	case errors.Is(err, apisperu.ErrNotFound):
		return apierror.DocumentNotFound()
	case errors.Is(err, ErrQuotaReached), errors.Is(err, apisperu.ErrQuotaExceeded):
		return apierror.UpstreamQuotaExceeded(err)
	case errors.Is(err, apisperu.ErrUnauthorized):
		return apierror.UpstreamUnauthorized(err)
	case errors.Is(err, apisperu.ErrTimeout):
		return apierror.UpstreamTimeout(err)
	default:
		return apierror.UpstreamFailed(err)
	}
}
