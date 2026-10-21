package store

import (
	"context"
	"fmt"
	"net/url"
	"sync"
	"testing"
	"time"
)

// freshSchemaStores creates an empty schema and returns n stores, each with its
// own connection pool, that see only that schema, like n separate processes
// on a new install. The schema is dropped after the test.
func freshSchemaStores(t *testing.T, n int) []*Store {
	t.Helper()
	admin := openOrSkip(t)
	ctx := context.Background()

	schema := fmt.Sprintf("migrate_test_%d", time.Now().UnixNano())
	if _, err := admin.pool.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatalf("create schema: %v", err)
	}
	t.Cleanup(func() {
		if _, err := admin.pool.Exec(context.Background(), "DROP SCHEMA "+schema+" CASCADE"); err != nil {
			t.Errorf("drop schema %s: %v", schema, err)
		}
	})

	// Point new connections at the empty schema only.
	u, err := url.Parse(TestDSN())
	if err != nil {
		t.Fatalf("parse dsn: %v", err)
	}
	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()

	var stores []*Store
	for i := 0; i < n; i++ {
		s, err := Open(ctx, u.String())
		if err != nil {
			t.Fatalf("open store %d: %v", i, err)
		}
		t.Cleanup(s.Close)
		stores = append(stores, s)
	}
	return stores
}

// together runs f once per store, all released at the same moment, and
// returns each call's error. f gets the store's index.
func together(stores []*Store, f func(int, *Store) error) []error {
	errs := make([]error, len(stores))
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i, s := range stores {
		wg.Add(1)
		go func(i int, s *Store) {
			defer wg.Done()
			<-start
			errs[i] = f(i, s)
		}(i, s)
	}
	close(start)
	wg.Wait()
	return errs
}

// TestMigrateConcurrent runs Migrate from four separate connection pools at
// once on an empty schema, as two packages' tests (or two CLI commands on a new
// install) do on a fresh database. Every call must succeed.
func TestMigrateConcurrent(t *testing.T) {
	ctx := context.Background()
	stores := freshSchemaStores(t, 4)

	errs := together(stores, func(_ int, s *Store) error { return s.Migrate(ctx) })
	for i, err := range errs {
		if err != nil {
			t.Errorf("migrate %d: %v", i, err)
		}
	}

	var n int
	if err := stores[0].pool.QueryRow(ctx, "SELECT count(*) FROM schema_migrations").Scan(&n); err != nil {
		t.Fatalf("count migrations: %v", err)
	}
	files, _ := migrationFiles()
	if n != len(files) {
		t.Errorf("schema_migrations has %d rows, want %d (one per file)", n, len(files))
	}
}

// TestEnsureLearnerConcurrent calls EnsureLearner from four connection pools
// at once on a new install. There must be exactly one learner afterwards, and
// every call must return it.
func TestEnsureLearnerConcurrent(t *testing.T) {
	ctx := context.Background()
	stores := freshSchemaStores(t, 4)
	if err := stores[0].Migrate(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	ids := make([]int, len(stores))
	errs := together(stores, func(i int, s *Store) error {
		l, err := s.EnsureLearner(ctx, "learner")
		ids[i] = l.ID
		return err
	})
	for i, err := range errs {
		if err != nil {
			t.Errorf("ensure learner %d: %v", i, err)
		}
	}

	var n int
	if err := stores[0].pool.QueryRow(ctx, "SELECT count(*) FROM learner").Scan(&n); err != nil {
		t.Fatalf("count learners: %v", err)
	}
	if n != 1 {
		t.Errorf("%d learners after concurrent EnsureLearner, want 1", n)
	}
	for i, id := range ids {
		if id != ids[0] {
			t.Errorf("call %d returned learner %d, call 0 returned %d", i, id, ids[0])
		}
	}
}
