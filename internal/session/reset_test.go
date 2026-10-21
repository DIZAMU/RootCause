package session

import (
	"context"
	"io"
	"path/filepath"
	"testing"
	"time"

	"github.com/DIZAMU/RootCause/internal/runtime"
	"github.com/DIZAMU/RootCause/internal/store"
)

// brokenRuntime creates sandboxes but every command in them fails, as when
// the machine cannot plant its fault. It records what it destroys.
type brokenRuntime struct{ destroyed []string }

func (b *brokenRuntime) Create(_ context.Context, spec runtime.SandboxSpec) (runtime.Sandbox, error) {
	return runtime.Sandbox{ID: "fake-" + spec.Name, Name: spec.Name, Labels: spec.Labels}, nil
}
func (b *brokenRuntime) Exec(context.Context, string, []string, io.Reader) (runtime.ExecResult, error) {
	return runtime.ExecResult{ExitCode: 1, Stderr: []byte("planted failure")}, nil
}
func (b *brokenRuntime) CopyIn(context.Context, string, string, []byte) error { return nil }
func (b *brokenRuntime) Restart(context.Context, string) error                { return nil }
func (b *brokenRuntime) Destroy(_ context.Context, id string) error {
	b.destroyed = append(b.destroyed, id)
	return nil
}
func (b *brokenRuntime) List(context.Context) ([]runtime.Sandbox, error) { return nil, nil }

// TestResetVoidsOnFailure checks that a Reset whose rebuilt sandbox cannot be
// prepared voids the session, logs a voided event with the reason, and
// destroys the half-built sandbox. No Docker needed; it needs the test DB.
func TestResetVoidsOnFailure(t *testing.T) {
	dsn := store.TestDSN()
	if dsn == "" {
		t.Skip("RC_TEST_DATABASE_URL not set to a database ending in _test")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	st, err := store.Open(ctx, dsn)
	if err != nil {
		t.Skip("db not reachable:", err)
	}
	t.Cleanup(st.Close)
	if err := st.Migrate(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	rt := &brokenRuntime{}
	learnerID := testLearner(t, st, rt)

	exDir, _ := filepath.Abs("../../exercises")
	if err := st.SyncExercises(ctx, []store.ExerciseRow{
		{ID: "svc-01-intranet-config", Version: 1, Topic: "services", Difficulty: 1},
	}); err != nil {
		t.Fatalf("sync: %v", err)
	}
	id := newID()
	if _, err := st.CreateSession(ctx, store.NewSession{
		ID: id, LearnerID: learnerID, ExerciseID: "svc-01-intranet-config",
		ExerciseVersion: 1, Difficulty: 1, Seed: 1, TTL: time.Hour,
	}); err != nil {
		t.Fatalf("create session: %v", err)
	}
	if err := st.SetSandbox(ctx, id, "fake-old"); err != nil {
		t.Fatalf("set sandbox: %v", err)
	}

	eng := New(rt, nil, nil, st, exDir)
	if err := eng.Reset(ctx, id); err == nil {
		t.Fatal("Reset succeeded although every command fails")
	}

	sess, err := st.SessionByID(ctx, id)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if sess.Status != "voided" {
		t.Errorf("status = %q, want voided", sess.Status)
	}
	events, _ := st.Events(ctx, id)
	var reason any
	for _, e := range events {
		if e.Type == "voided" {
			reason = e.Payload["reason"]
		}
	}
	if reason != "reset_failed" {
		t.Errorf("voided event reason = %v, want reset_failed (events %v)", reason, events)
	}
	want := map[string]bool{"fake-old": false, "fake-rootcause-" + id: false}
	for _, d := range rt.destroyed {
		if _, ok := want[d]; ok {
			want[d] = true
		}
	}
	for sb, done := range want {
		if !done {
			t.Errorf("sandbox %s was not destroyed (destroyed: %v)", sb, rt.destroyed)
		}
	}
}
