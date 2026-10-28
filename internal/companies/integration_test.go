package companies

import (
	"context"
	"os"
	"testing"

	"atm-identity-api/internal/apisperu"
	"atm-identity-api/internal/lookup"
	"atm-identity-api/internal/platform/apierror"
	"atm-identity-api/internal/platform/database"
)

// Runs against a real, migrated Postgres (TEST_DATABASE_URL). Skipped otherwise.
// It only touches document numbers starting with 9999 and cleans up after itself.

type fakeUpstream struct {
	calls   int
	company *apisperu.Company
	err     error
}

func (f *fakeUpstream) Person(ctx context.Context, doc string) (*apisperu.Person, error) {
	return nil, apisperu.ErrNotFound
}

func (f *fakeUpstream) Company(ctx context.Context, doc string) (*apisperu.Company, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	c := *f.company
	c.DocumentNumber = doc
	return &c, nil
}

func TestResolveStoresOnce(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	pool, err := database.Connect(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	cleanup := func() {
		_, _ = pool.Exec(ctx, `DELETE FROM companies WHERE document_number LIKE '9999%'`)
		_, _ = pool.Exec(ctx, `DELETE FROM lookup_misses WHERE document_type='ruc' AND document_number LIKE '9999%'`)
		_, _ = pool.Exec(ctx, `DELETE FROM provider_usage WHERE provider = 'test-companies'`)
	}
	cleanup()
	defer cleanup()

	up := &fakeUpstream{company: &apisperu.Company{LegalName: "ACME SAC", TradeName: "ACME", Address: "AV. X 123"}}
	svc := NewService(NewRepository(pool), up, lookup.NewGuard(pool, "test-companies", 1000))

	c, source, err := svc.Resolve(ctx, "99999999901", false)
	if err != nil || source != SourceUpstream || c.LegalName != "ACME SAC" || *c.Address != "AV. X 123" {
		t.Fatalf("first resolve: %+v %s %v", c, source, err)
	}
	if _, source, _ = svc.Resolve(ctx, "99999999901", false); source != SourceDatabase || up.calls != 1 {
		t.Fatalf("second resolve must come from the database (source=%s calls=%d)", source, up.calls)
	}

	up.err = apisperu.ErrNotFound
	_, _, err = svc.Resolve(ctx, "99999999902", false)
	if e, ok := err.(*apierror.Error); !ok || e.Code != apierror.CodeDocumentNotFound {
		t.Fatalf("want DOCUMENT_NOT_FOUND, got %v", err)
	}
}
