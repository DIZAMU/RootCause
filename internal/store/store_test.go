package store

import (
	"context"
	"errors"
	"testing"
	"time"
)

// openOrSkip connects to the test database. It skips only when
// RC_TEST_DATABASE_URL is unset, and fails when it is set but unsafe or
// unreachable (the rule internal/testenv applies to the other packages, which
// this package's tests cannot import). The store is closed by t.Cleanup.
// Cleanups run in reverse order, so this one runs after every cleanup the test
// registers later, which still need the connection. (A deferred Close would
// run before them.)
func openOrSkip(t *testing.T) *Store {
	t.Helper()
	dsn, err := TestDSN()
	if err != nil {
		t.Fatal(err)
	}
	if dsn == "" {
		t.Skip("RC_TEST_DATABASE_URL not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	s, err := Open(ctx, dsn)
	if err != nil {
		t.Fatalf("test database not reachable: %v", err)
	}
	t.Cleanup(s.Close)
	return s
}

func TestMigrateAndSync(t *testing.T) {
	s := openOrSkip(t)
	ctx := context.Background()

	// Migrate is idempotent.
	if err := s.Migrate(ctx); err != nil {
		t.Fatalf("first migrate: %v", err)
	}
	if err := s.Migrate(ctx); err != nil {
		t.Fatalf("second migrate (should be a no-op): %v", err)
	}

	// The four source tables exist.
	for _, table := range []string{"learner", "exercise", "session", "event"} {
		var exists bool
		if err := s.pool.QueryRow(ctx,
			`SELECT EXISTS (SELECT 1 FROM information_schema.tables
			               WHERE table_name = $1)`, table).Scan(&exists); err != nil {
			t.Fatalf("check table %s: %v", table, err)
		}
		if !exists {
			t.Errorf("table %q was not created", table)
		}
	}

	// EnsureLearner is a singleton.
	l1, err := s.EnsureLearner(ctx, "DIZAMU")
	if err != nil {
		t.Fatalf("ensure learner: %v", err)
	}
	l2, err := s.EnsureLearner(ctx, "someone-else")
	if err != nil {
		t.Fatalf("ensure learner again: %v", err)
	}
	if l1.ID != l2.ID {
		t.Errorf("EnsureLearner created a second learner: %d vs %d", l1.ID, l2.ID)
	}

	// SetLang round-trips.
	if err := s.SetLang(ctx, l1.ID, "no"); err != nil {
		t.Fatalf("set lang: %v", err)
	}
	l3, _ := s.EnsureLearner(ctx, "")
	if l3.Lang != "no" {
		t.Errorf("lang = %q, want no", l3.Lang)
	}

	// SyncExercises upserts.
	rows := []ExerciseRow{{ID: "svc-01-intranet-config", Version: 1, Topic: "services", Difficulty: 1}}
	if err := s.SyncExercises(ctx, rows); err != nil {
		t.Fatalf("sync: %v", err)
	}
	rows[0].Difficulty = 2 // re-sync with a change
	if err := s.SyncExercises(ctx, rows); err != nil {
		t.Fatalf("re-sync: %v", err)
	}
	got, err := s.Exercises(ctx)
	if err != nil {
		t.Fatalf("list exercises: %v", err)
	}
	found := false
	for _, r := range got {
		if r.ID == "svc-01-intranet-config" {
			found = true
			if r.Difficulty != 2 {
				t.Errorf("difficulty after re-sync = %d, want 2", r.Difficulty)
			}
		}
	}
	if !found {
		t.Error("synced exercise not found")
	}
}

func TestTestDSNRefusesRealDatabase(t *testing.T) {
	cases := map[string]string{
		"": "",
		"postgres://u:p@127.0.0.1:5432/rootcause_test?sslmode=disable": "postgres://u:p@127.0.0.1:5432/rootcause_test?sslmode=disable",
		"postgres://u:p@127.0.0.1:5432/rootcause_test":                 "postgres://u:p@127.0.0.1:5432/rootcause_test",
	}
	for dsn, want := range cases {
		t.Setenv("RC_TEST_DATABASE_URL", dsn)
		if got, err := TestDSN(); got != want || err != nil {
			t.Errorf("TestDSN(%q) = %q, %v; want %q, nil", dsn, got, err, want)
		}
	}
	t.Setenv("RC_TEST_DATABASE_URL", "postgres://u:p@127.0.0.1:5432/rootcause?sslmode=disable")
	if got, err := TestDSN(); got != "" || !errors.Is(err, ErrUnsafeTestDSN) {
		t.Errorf("TestDSN(real database) = %q, %v; want ErrUnsafeTestDSN", got, err)
	}
}

// TestWithStatsLock: two builds, here from two stores as from the CLI and the
// dashboard, never run at the same time, and a caller whose context ends
// while it waits does not run its build.
func TestWithStatsLock(t *testing.T) {
	a := openOrSkip(t)
	b := openOrSkip(t)
	ctx := context.Background()

	inA := make(chan struct{})
	releaseA := make(chan struct{})
	doneA := make(chan error, 1)
	go func() {
		doneA <- a.WithStatsLock(ctx, func() error {
			close(inA)
			<-releaseA
			return nil
		})
	}()
	<-inA

	waitCtx, cancel := context.WithTimeout(ctx, 300*time.Millisecond)
	defer cancel()
	ran := false
	if err := b.WithStatsLock(waitCtx, func() error { ran = true; return nil }); err == nil || ran {
		t.Errorf("second build ran while the first held the lock: err=%v ran=%v", err, ran)
	}

	close(releaseA)
	if err := <-doneA; err != nil {
		t.Fatal(err)
	}
	if err := b.WithStatsLock(ctx, func() error { ran = true; return nil }); err != nil || !ran {
		t.Errorf("build after release: err=%v ran=%v", err, ran)
	}
}
