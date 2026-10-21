package session

import (
	"context"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/DIZAMU/RootCause/internal/runtime/docker"
	"github.com/DIZAMU/RootCause/internal/store"
)

// TestReaperExpiresDueSession starts a real session, backdates its TTL, and
// checks the reaper expires it: status expired, sandbox gone, expired event.
func TestReaperExpiresDueSession(t *testing.T) {
	dsn := store.TestDSN()
	if dsn == "" {
		t.Skip("RC_TEST_DATABASE_URL not set to a database ending in _test")
	}
	if err := exec.Command("docker", "info").Run(); err != nil {
		t.Skip("docker not reachable:", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	st, err := store.Open(ctx, dsn)
	if err != nil {
		t.Skip("db not reachable:", err)
	}
	defer st.Close()
	if err := st.Migrate(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	learner, _ := st.EnsureLearner(ctx, "DIZAMU")

	rt, err := docker.New()
	if err != nil {
		t.Skip("docker runtime:", err)
	}
	exDir, _ := filepath.Abs("../../exercises")
	eng := New(rt, rt, rt, st, exDir)

	sess, err := eng.Start(ctx, learner.ID, "svc-01-intranet-config", 1)
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	t.Cleanup(func() { _ = rt.Destroy(context.Background(), sess.SandboxID) })

	// Backdate the TTL so the session is due now.
	if _, err := st.Pool().Exec(ctx,
		`UPDATE session SET expires_at = now() - interval '1 hour' WHERE id = $1`, sess.ID); err != nil {
		t.Fatalf("backdate expiry: %v", err)
	}

	n, err := NewReaper(eng, st, 0).ReapOnce(ctx)
	if err != nil {
		t.Fatalf("reap: %v", err)
	}
	if n < 1 {
		t.Fatalf("reaper expired %d sessions, want at least 1", n)
	}

	after, err := st.SessionByID(ctx, sess.ID)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if after.Status != "expired" {
		t.Errorf("status = %q, want expired", after.Status)
	}

	// The sandbox must be gone.
	boxes, _ := rt.List(ctx)
	for _, b := range boxes {
		if b.ID == sess.SandboxID {
			t.Errorf("sandbox %s still running after reap", sess.SandboxID)
		}
	}

	// An expired event must be logged.
	events, _ := st.Events(ctx, sess.ID)
	hasExpired := false
	for _, e := range events {
		if e.Type == "expired" {
			hasExpired = true
		}
	}
	if !hasExpired {
		t.Error("no expired event logged")
	}
}
