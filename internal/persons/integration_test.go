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

// Runs against a real, migrated Postgres (TEST_DATABASE_URL). Skipped otherwise.
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
		_, _ = pool.Exec(context.Background(), `DELETE FROM persons WHERE document_number LIKE '9999%'`)
	}
	cleanup()
	t.Cleanup(cleanup)
	return pool
}

func TestResolveStoresOnceAndRefreshes(t *testing.T) {
	pool := testPool(t)
	up := &fakeUpstream{person: &apisperu.Person{FirstNames: "ANA", PaternalSurname: "PEREZ", MaternalSurname: "LOPEZ"}}
	svc := NewService(NewRepository(pool), up)
	ctx := context.Background()

	p, source, err := svc.Resolve(ctx, "99990001", false)
	if err != nil || source != SourceUpstream || p.FullName != "ANA PEREZ LOPEZ" {
		t.Fatalf("first resolve: %+v %s %v", p, source, err)
	}
	for i := 0; i < 9; i++ {
		if _, source, _ = svc.Resolve(ctx, "99990001", false); source != SourceDatabase {
			t.Fatalf("resolve %d must come from the database, got %s", i+2, source)
		}
	}
	if n := up.calls.Load(); n != 1 {
		t.Fatalf("10 lookups must cost 1 upstream call, got %d", n)
	}

	up.person = &apisperu.Person{FirstNames: "ANA MARIA", PaternalSurname: "PEREZ", MaternalSurname: "LOPEZ"}
	p, source, err = svc.Resolve(ctx, "99990001", true)
	if err != nil || source != SourceUpstream || p.FirstNames != "ANA MARIA" {
		t.Fatalf("refresh: %+v %s %v", p, source, err)
	}
}

func TestResolveSingleRowUnderConcurrency(t *testing.T) {
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

func TestUpstreamErrors(t *testing.T) {
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
}
