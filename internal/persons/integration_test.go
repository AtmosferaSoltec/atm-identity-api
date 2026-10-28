package persons

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"testing"

	"atm-identity-api/internal/apisperu"
	"atm-identity-api/internal/lookup"
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
		_, _ = pool.Exec(context.Background(), `DELETE FROM lookup_misses WHERE document_type='dni' AND document_number LIKE '9999%'`)
		_, _ = pool.Exec(context.Background(), `DELETE FROM provider_usage WHERE provider = 'test-persons'`)
	}
	cleanup()
	t.Cleanup(cleanup)
	return pool
}

func TestResolveStoresOnceAndRefreshes(t *testing.T) {
	pool := testPool(t)
	up := &fakeUpstream{person: &apisperu.Person{FirstNames: "ANA", PaternalSurname: "PEREZ", MaternalSurname: "LOPEZ"}}
	svc := NewService(NewRepository(pool), up, lookup.NewGuard(pool, "test-persons", 1000))
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

	_, _ = pool.Exec(ctx, `UPDATE persons SET looked_up_at = now() - interval '2 days' WHERE document_number='99990001'`)
	up.person = &apisperu.Person{FirstNames: "ANA MARIA", PaternalSurname: "PEREZ", MaternalSurname: "LOPEZ"}
	p, source, err = svc.Resolve(ctx, "99990001", true)
	if err != nil || source != SourceUpstream || p.FirstNames != "ANA MARIA" {
		t.Fatalf("refresh: %+v %s %v", p, source, err)
	}
}

func TestResolveSingleRowUnderConcurrency(t *testing.T) {
	pool := testPool(t)
	up := &fakeUpstream{person: &apisperu.Person{FirstNames: "LUIS", PaternalSurname: "DIAZ"}}
	svc := NewService(NewRepository(pool), up, lookup.NewGuard(pool, "test-persons", 1000))

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
	svc := NewService(NewRepository(pool), up, lookup.NewGuard(pool, "test-persons", 1000))
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
	if _, _, err = svc.Resolve(ctx, "99990004", false); err.(*apierror.Error).Code != apierror.CodeUpstreamTimeout {
		t.Fatalf("want UPSTREAM_TIMEOUT, got %v", err)
	}
}

func TestMonthlyLimitStopsUpstreamCalls(t *testing.T) {
	pool := testPool(t)
	up := &fakeUpstream{person: &apisperu.Person{FirstNames: "ANA", PaternalSurname: "PEREZ"}}
	svc := NewService(NewRepository(pool), up, lookup.NewGuard(pool, "test-persons", 2))
	ctx := context.Background()

	for _, doc := range []string{"99990010", "99990011"} {
		if _, _, err := svc.Resolve(ctx, doc, false); err != nil {
			t.Fatalf("%s: %v", doc, err)
		}
	}
	_, _, err := svc.Resolve(ctx, "99990012", false)
	if e, ok := err.(*apierror.Error); !ok || e.Code != apierror.CodeUpstreamQuotaExceeded || e.Status != 503 {
		t.Fatalf("want UPSTREAM_QUOTA_EXCEEDED 503, got %v", err)
	}
	if n := up.calls.Load(); n != 2 {
		t.Fatalf("the limit must stop the third call, upstream calls = %d", n)
	}
	// Already stored documents keep working when the budget is gone.
	if _, source, err := svc.Resolve(ctx, "99990010", false); err != nil || source != SourceDatabase {
		t.Fatalf("stored document must still resolve: %s %v", source, err)
	}
}

func TestMonthlyLimitHoldsUnderConcurrency(t *testing.T) {
	pool := testPool(t)
	up := &fakeUpstream{person: &apisperu.Person{FirstNames: "ANA", PaternalSurname: "PEREZ"}}
	svc := NewService(NewRepository(pool), up, lookup.NewGuard(pool, "test-persons", 5))

	var wg sync.WaitGroup
	for i := 0; i < 30; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, _, _ = svc.Resolve(context.Background(), fmt.Sprintf("999900%02d", 20+i), false)
		}(i)
	}
	wg.Wait()
	if n := up.calls.Load(); n != 5 {
		t.Fatalf("limit 5 must allow exactly 5 upstream calls, got %d", n)
	}
}

func TestNotFoundIsRememberedAndNotRecharged(t *testing.T) {
	pool := testPool(t)
	up := &fakeUpstream{err: apisperu.ErrNotFound}
	svc := NewService(NewRepository(pool), up, lookup.NewGuard(pool, "test-persons", 1000))
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		if _, _, err := svc.Resolve(ctx, "99990040", false); err.(*apierror.Error).Code != apierror.CodeDocumentNotFound {
			t.Fatalf("attempt %d: %v", i, err)
		}
	}
	if n := up.calls.Load(); n != 1 {
		t.Fatalf("a nonexistent document must cost 1 upstream call, got %d", n)
	}
	_, _ = pool.Exec(ctx, `UPDATE lookup_misses SET expires_at = now() - interval '1 second' WHERE document_number='99990040'`)
	_, _, _ = svc.Resolve(ctx, "99990040", false)
	if n := up.calls.Load(); n != 2 {
		t.Fatalf("after the miss expires the provider is asked again, calls = %d", n)
	}
}

func TestRefreshHasCooldown(t *testing.T) {
	pool := testPool(t)
	up := &fakeUpstream{person: &apisperu.Person{FirstNames: "ANA", PaternalSurname: "PEREZ"}}
	svc := NewService(NewRepository(pool), up, lookup.NewGuard(pool, "test-persons", 1000))
	ctx := context.Background()

	_, _, _ = svc.Resolve(ctx, "99990050", false)
	if _, source, _ := svc.Resolve(ctx, "99990050", true); source != SourceDatabase || up.calls.Load() != 1 {
		t.Fatalf("refresh of a fresh record must not call the provider (source=%s calls=%d)", source, up.calls.Load())
	}
	_, _ = pool.Exec(ctx, `UPDATE persons SET looked_up_at = now() - interval '2 days' WHERE document_number='99990050'`)
	if _, source, _ := svc.Resolve(ctx, "99990050", true); source != SourceUpstream || up.calls.Load() != 2 {
		t.Fatalf("refresh of a stale record must call the provider (source=%s calls=%d)", source, up.calls.Load())
	}
}

func TestProviderFailureCodes(t *testing.T) {
	pool := testPool(t)
	up := &fakeUpstream{}
	svc := NewService(NewRepository(pool), up, lookup.NewGuard(pool, "test-persons", 1000))
	for _, c := range []struct {
		err  error
		code apierror.Code
	}{
		{apisperu.ErrQuotaExceeded, apierror.CodeUpstreamQuotaExceeded},
		{apisperu.ErrUnauthorized, apierror.CodeUpstreamUnauthorized},
		{apisperu.ErrTimeout, apierror.CodeUpstreamTimeout},
		{errors.New("boom"), apierror.CodeUpstreamFailed},
	} {
		up.err = c.err
		_, _, err := svc.Resolve(context.Background(), "99990060", false)
		if e := err.(*apierror.Error); e.Code != c.code {
			t.Errorf("%v -> %s, want %s", c.err, e.Code, c.code)
		}
	}
}
