package persons

import (
	"context"
	"errors"
	"strings"

	"atm-identity-api/internal/apisperu"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct{ pool *pgxpool.Pool }

func NewRepository(pool *pgxpool.Pool) *Repository { return &Repository{pool: pool} }

var errNotFound = errors.New("person not found")

func (r *Repository) Find(ctx context.Context, documentNumber string) (*Person, error) {
	var p Person
	err := r.pool.QueryRow(ctx, `
		SELECT document_number, first_names, paternal_surname, maternal_surname, created_at, updated_at
		FROM persons WHERE document_number = $1`, documentNumber).
		Scan(&p.DocumentNumber, &p.FirstNames, &p.PaternalSurname, &p.MaternalSurname, &p.CreatedAt, &p.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, errNotFound
	}
	if err != nil {
		return nil, err
	}
	p.CreatedAt, p.UpdatedAt = p.CreatedAt.UTC(), p.UpdatedAt.UTC()
	p.FullName = fullName(&p)
	return &p, nil
}

func fullName(p *Person) string {
	parts := []string{p.FirstNames, p.PaternalSurname}
	if p.MaternalSurname != nil {
		parts = append(parts, *p.MaternalSurname)
	}
	return strings.Join(parts, " ")
}

// InsertUpstream stores a record fetched from APIs Peru. It never overwrites an
// existing row, so concurrent inserts of the same document are safe.
func (r *Repository) InsertUpstream(ctx context.Context, u *apisperu.Person) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO persons (document_number, first_names, paternal_surname, maternal_surname, looked_up_at)
		VALUES ($1, $2, $3, NULLIF($4, ''), now())
		ON CONFLICT (document_number) DO NOTHING`,
		u.DocumentNumber, u.FirstNames, u.PaternalSurname, u.MaternalSurname)
	return err
}

// RefreshUpstream overwrites the stored names with the current upstream values.
func (r *Repository) RefreshUpstream(ctx context.Context, u *apisperu.Person) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE persons SET first_names = $2, paternal_surname = $3, maternal_surname = NULLIF($4, ''),
			looked_up_at = now(), updated_at = now()
		WHERE document_number = $1`,
		u.DocumentNumber, u.FirstNames, u.PaternalSurname, u.MaternalSurname)
	return err
}
