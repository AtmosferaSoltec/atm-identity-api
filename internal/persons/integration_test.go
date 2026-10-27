package persons

import (
	"context"
	"os"
	"sync"
	"sync/atomic"
	"testing"

	"atm-identity-api/internal/apisperu"
	"atm-identity-api/internal/platform/apierror"
	"atm-identity-api/internal/platform/database"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Runs against a real Postgres (TEST_DATABASE_URL, migrated). Skipped otherwise.
// It only touches document numbers starting with 9999 and cleans up after itself.

type fakeUpstream struct {
	calls  atomic.Int32
	person *apisperu.Person
	err    error
}

func (f *fakeUpstream) Person(ctx context.Context, doc string) (*apisperu.Person, error) {
	f.calls.Add(1)
	if f.err != nil {
		return nil, f.err
	}
	p := *f.person
	p.DocumentNumber = doc
	return &p, nil
}
func (f *fakeUpstream) Company(ctx context.Context, doc string) (*apisperu.Company, error) {
	return nil, apisperu.ErrNotFound
}

func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	pool, err := database.Connect(context.Background(), url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	cleanup := func() {
		_, _ = pool.Exec(context.Background(),
			`DELETE FROM record_changes WHERE entity_type='person' AND entity_id IN (SELECT id FROM persons WHERE document_number LIKE '9999%')`)
		_, _ = pool.Exec(context.Background(), `DELETE FROM persons WHERE document_number LIKE '9999%'`)
	}
	cleanup()
	t.Cleanup(cleanup)
	return pool
}

func strp(s string) *string { return &s }

func TestResolveCachesAndEnriches(t *testing.T) {
	pool := testPool(t)
	up := &fakeUpstream{person: &apisperu.Person{FirstNames: "ANA", PaternalSurname: "PEREZ", MaternalSurname: "LOPEZ"}}
	svc := NewService(NewRepository(pool), up)
	ctx := context.Background()

	p, source, err := svc.Resolve(ctx, "99990001", false)
	if err != nil || source != SourceUpstream || p.FirstNames != "ANA" || p.Email != nil {
		t.Fatalf("first resolve: %+v %s %v", p, source, err)
	}
	if _, source, _ = svc.Resolve(ctx, "99990001", false); source != SourceDatabase || up.calls.Load() != 1 {
		t.Fatalf("second resolve must come from the database (source=%s calls=%d)", source, up.calls.Load())
	}

	// Enrichment by "hotel", read back by anyone.
	p, err = svc.Patch(ctx, "99990001", "hotel", PatchInput{Email: strp("ana@x.pe"), BirthDate: strp("1990-05-17"), Phone: strp("999888777")})
	if err != nil || *p.Email != "ana@x.pe" || *p.BirthDate != "1990-05-17" {
		t.Fatalf("patch: %+v %v", p, err)
	}
	// Omitted fields never erase; a second system adds others.
	p, err = svc.Patch(ctx, "99990001", "iris", PatchInput{District: strp("MIRAFLORES")})
	if err != nil || *p.Email != "ana@x.pe" || *p.District != "MIRAFLORES" {
		t.Fatalf("second patch must keep earlier data: %+v %v", p, err)
	}

	var sources string
	_ = pool.QueryRow(ctx, `SELECT field_sources->>'email' FROM persons WHERE document_number='99990001'`).Scan(&sources)
	if sources != "system:hotel" {
		t.Fatalf("field_sources email = %q", sources)
	}
	var audit int
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM record_changes rc JOIN persons p ON p.id=rc.entity_id WHERE rc.entity_type='person' AND p.document_number='99990001'`).Scan(&audit)
	if audit != 2 {
		t.Fatalf("want 2 audit rows, got %d", audit)
	}

	// Identical patch is a no-op (no extra audit row).
	_, _ = svc.Patch(ctx, "99990001", "hotel", PatchInput{Email: strp("ana@x.pe")})
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM record_changes rc JOIN persons p ON p.id=rc.entity_id WHERE p.document_number='99990001'`).Scan(&audit)
	if audit != 2 {
		t.Fatalf("no-op patch must not audit, got %d rows", audit)
	}

	// Refresh updates upstream-owned columns but keeps enriched ones.
	up.person = &apisperu.Person{FirstNames: "ANA MARIA", PaternalSurname: "PEREZ", MaternalSurname: "LOPEZ"}
	p, source, err = svc.Resolve(ctx, "99990001", true)
	if err != nil || source != SourceUpstream || p.FirstNames != "ANA MARIA" || *p.Email != "ana@x.pe" {
		t.Fatalf("refresh: %+v %s %v", p, source, err)
	}
}

func TestResolveSingleUpstreamCallUnderConcurrency(t *testing.T) {
	pool := testPool(t)
	up := &fakeUpstream{person: &apisperu.Person{FirstNames: "LUIS", PaternalSurname: "DIAZ"}}
	svc := NewService(NewRepository(pool), up)

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, _, err := svc.Resolve(context.Background(), "99990002", false); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	var rows int
	_ = pool.QueryRow(context.Background(), `SELECT count(*) FROM persons WHERE document_number='99990002'`).Scan(&rows)
	if rows != 1 {
		t.Fatalf("want 1 row, got %d", rows)
	}
	if n := up.calls.Load(); n > 3 {
		t.Fatalf("expected few upstream calls under concurrency, got %d", n)
	}
}

func TestErrors(t *testing.T) {
	pool := testPool(t)
	up := &fakeUpstream{err: apisperu.ErrNotFound}
	svc := NewService(NewRepository(pool), up)
	ctx := context.Background()

	_, _, err := svc.Resolve(ctx, "99990003", false)
	if e, ok := err.(*apierror.Error); !ok || e.Code != apierror.CodeDocumentNotFound {
		t.Fatalf("want DOCUMENT_NOT_FOUND, got %v", err)
	}
	var rows int
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM persons WHERE document_number='99990003'`).Scan(&rows)
	if rows != 0 {
		t.Fatal("a not-found document must not be stored")
	}

	up.err = apisperu.ErrTimeout
	if _, _, err = svc.Resolve(ctx, "99990003", false); err.(*apierror.Error).Code != apierror.CodeUpstreamTimeout {
		t.Fatalf("want UPSTREAM_TIMEOUT, got %v", err)
	}

	_, err = svc.Patch(ctx, "99990003", "hotel", PatchInput{Email: strp("x@y.pe")})
	if err.(*apierror.Error).Code != apierror.CodeDocumentNotFound {
		t.Fatalf("patch on unknown document: %v", err)
	}
	_, err = svc.Patch(ctx, "99990003", "hotel", PatchInput{Email: strp("bad"), Sex: strp("x")})
	if e := err.(*apierror.Error); e.Code != apierror.CodeValidationFailed || len(e.Details) != 2 {
		t.Fatalf("want 2 validation details, got %v", err)
	}
}
