package companies

import (
	"context"
	"errors"

	"atm-identity-api/internal/apisperu"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct{ pool *pgxpool.Pool }

func NewRepository(pool *pgxpool.Pool) *Repository { return &Repository{pool: pool} }

var errNotFound = errors.New("company not found")

func (r *Repository) Find(ctx context.Context, documentNumber string) (*Company, error) {
	var c Company
	err := r.pool.QueryRow(ctx, `
		SELECT document_number, legal_name, trade_name, address, looked_up_at, created_at, updated_at
		FROM companies WHERE document_number = $1`, documentNumber).
		Scan(&c.DocumentNumber, &c.LegalName, &c.TradeName, &c.Address, &c.LookedUpAt, &c.CreatedAt, &c.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, errNotFound
	}
	if err != nil {
		return nil, err
	}
	c.CreatedAt, c.UpdatedAt = c.CreatedAt.UTC(), c.UpdatedAt.UTC()
	return &c, nil
}

// InsertUpstream stores a record fetched from APIs Peru without overwriting an existing row.
func (r *Repository) InsertUpstream(ctx context.Context, u *apisperu.Company) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO companies (document_number, legal_name, trade_name, address, looked_up_at)
		VALUES ($1, $2, NULLIF($3,''), NULLIF($4,''), now())
		ON CONFLICT (document_number) DO NOTHING`,
		u.DocumentNumber, u.LegalName, u.TradeName, u.Address)
	return err
}

// RefreshUpstream overwrites the stored values with the current upstream ones.
func (r *Repository) RefreshUpstream(ctx context.Context, u *apisperu.Company) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE companies SET legal_name = $2, trade_name = NULLIF($3,''), address = NULLIF($4,''),
			looked_up_at = now(), updated_at = now()
		WHERE document_number = $1`,
		u.DocumentNumber, u.LegalName, u.TradeName, u.Address)
	return err
}
