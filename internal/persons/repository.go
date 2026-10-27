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

const columns = `document_number, first_names, paternal_surname, maternal_surname, verification_digit,
	birth_date::text, sex, email, phone, address, district, province, department, ubigeo,
	lookup_source, created_at, updated_at`

var errNotFound = errors.New("person not found")

func scan(row pgx.Row) (*Person, error) {
	var p Person
	err := row.Scan(&p.DocumentNumber, &p.FirstNames, &p.PaternalSurname, &p.MaternalSurname,
		&p.VerificationDigit, &p.BirthDate, &p.Sex, &p.Email, &p.Phone, &p.Address, &p.District,
		&p.Province, &p.Department, &p.Ubigeo, &p.LookupSource, &p.CreatedAt, &p.UpdatedAt)
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

func (r *Repository) Find(ctx context.Context, documentNumber string) (*Person, error) {
	return scan(r.pool.QueryRow(ctx,
		`SELECT `+columns+` FROM persons WHERE document_number = $1`, documentNumber))
}

// InsertUpstream stores a record fetched from APIs Peru. It never overwrites an
// existing row, so concurrent inserts of the same document are safe.
func (r *Repository) InsertUpstream(ctx context.Context, u *apisperu.Person) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO persons (document_number, first_names, paternal_surname, maternal_surname,
			verification_digit, lookup_source, looked_up_at, field_sources)
		VALUES ($1, $2, $3, NULLIF($4, ''), NULLIF($5, ''), 'apisperu', now(),
			'{"first_names":"apisperu","paternal_surname":"apisperu","maternal_surname":"apisperu"}'::jsonb)
		ON CONFLICT (document_number) DO NOTHING`,
		u.DocumentNumber, u.FirstNames, u.PaternalSurname, u.MaternalSurname, u.VerificationDigit)
	return err
}

// RefreshUpstream updates only the columns APIs Peru owns.
func (r *Repository) RefreshUpstream(ctx context.Context, u *apisperu.Person) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE persons SET first_names = $2, paternal_surname = $3, maternal_surname = NULLIF($4, ''),
			verification_digit = NULLIF($5, ''), looked_up_at = now(), updated_at = now()
		WHERE document_number = $1`,
		u.DocumentNumber, u.FirstNames, u.PaternalSurname, u.MaternalSurname, u.VerificationDigit)
	return err
}
