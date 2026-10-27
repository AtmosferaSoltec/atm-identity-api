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

const columns = `document_number, legal_name, trade_name, company_type, taxpayer_status,
	taxpayer_condition, email, phone, address, district, province, department, ubigeo,
	lookup_source, created_at, updated_at`

var errNotFound = errors.New("company not found")

func (r *Repository) Find(ctx context.Context, documentNumber string) (*Company, error) {
	var c Company
	err := r.pool.QueryRow(ctx, `SELECT `+columns+` FROM companies WHERE document_number = $1`, documentNumber).
		Scan(&c.DocumentNumber, &c.LegalName, &c.TradeName, &c.CompanyType, &c.TaxpayerStatus,
			&c.TaxpayerCondition, &c.Email, &c.Phone, &c.Address, &c.District, &c.Province,
			&c.Department, &c.Ubigeo, &c.LookupSource, &c.CreatedAt, &c.UpdatedAt)
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
		INSERT INTO companies (document_number, legal_name, trade_name, company_type, taxpayer_status,
			taxpayer_condition, phone, address, district, province, department, ubigeo,
			lookup_source, looked_up_at, field_sources)
		VALUES ($1, $2, NULLIF($3,''), NULLIF($4,''), NULLIF($5,''), NULLIF($6,''), NULLIF($7,''),
			NULLIF($8,''), NULLIF($9,''), NULLIF($10,''), NULLIF($11,''), NULLIF($12,''),
			'apisperu', now(),
			jsonb_strip_nulls(jsonb_build_object(
				'legal_name', 'apisperu',
				'trade_name', CASE WHEN NULLIF($3,'') IS NOT NULL THEN 'apisperu' END,
				'address', CASE WHEN NULLIF($8,'') IS NOT NULL THEN 'apisperu' END,
				'district', CASE WHEN NULLIF($9,'') IS NOT NULL THEN 'apisperu' END,
				'province', CASE WHEN NULLIF($10,'') IS NOT NULL THEN 'apisperu' END,
				'department', CASE WHEN NULLIF($11,'') IS NOT NULL THEN 'apisperu' END,
				'ubigeo', CASE WHEN NULLIF($12,'') IS NOT NULL THEN 'apisperu' END,
				'phone', CASE WHEN NULLIF($7,'') IS NOT NULL THEN 'apisperu' END)))
		ON CONFLICT (document_number) DO NOTHING`,
		u.DocumentNumber, u.LegalName, u.TradeName, u.CompanyType, u.TaxpayerStatus,
		u.TaxpayerCondition, u.Phone, u.Address, u.District, u.Province, u.Department, u.Ubigeo)
	return err
}

// RefreshUpstream updates the columns APIs Peru owns. Contact and location
// columns are only overwritten when APIs Peru (or nobody) wrote them last, so a
// refresh never clobbers a value enriched by a client system.
func (r *Repository) RefreshUpstream(ctx context.Context, u *apisperu.Company) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE companies SET legal_name = $2, trade_name = NULLIF($3,''), company_type = NULLIF($4,''),
			taxpayer_status = NULLIF($5,''), taxpayer_condition = NULLIF($6,''),
			address    = CASE WHEN coalesce(field_sources->>'address', 'apisperu') = 'apisperu' THEN NULLIF($7,'') ELSE address END,
			district   = CASE WHEN coalesce(field_sources->>'district', 'apisperu') = 'apisperu' THEN NULLIF($8,'') ELSE district END,
			province   = CASE WHEN coalesce(field_sources->>'province', 'apisperu') = 'apisperu' THEN NULLIF($9,'') ELSE province END,
			department = CASE WHEN coalesce(field_sources->>'department', 'apisperu') = 'apisperu' THEN NULLIF($10,'') ELSE department END,
			ubigeo     = CASE WHEN coalesce(field_sources->>'ubigeo', 'apisperu') = 'apisperu' THEN NULLIF($11,'') ELSE ubigeo END,
			looked_up_at = now(), updated_at = now()
		WHERE document_number = $1`,
		u.DocumentNumber, u.LegalName, u.TradeName, u.CompanyType, u.TaxpayerStatus,
		u.TaxpayerCondition, u.Address, u.District, u.Province, u.Department, u.Ubigeo)
	return err
}
