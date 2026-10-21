package session

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/DIZAMU/RootCause/internal/runtime/docker"
	"github.com/DIZAMU/RootCause/internal/store"
)

// TestPlaySvc01 drives svc-01 through the lifecycle end to end: start (fault
// planted), check fails, a simulated learner fix, check passes, solved. It
// asserts the right events are logged. It needs both Docker and Postgres; it
// skips otherwise.
func TestPlaySvc01(t *testing.T) {
	dsn := store.DSNFromEnv()
	if dsn == "" {
		t.Skip("RC_DATABASE_URL not set")
	}
	if err := exec.Command("docker", "info").Run(); err != nil {
		t.Skip("docker not reachable:", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	st, err := store.Open(ctx, dsn)
	if err != nil {
		t.Skip("db not reachable:", err)
	}
	defer st.Close()
	if err := st.Migrate(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	learner, err := st.EnsureLearner(ctx, "DIZAMU")
	if err != nil {
		t.Fatalf("learner: %v", err)
	}

	rt, err := docker.New()
	if err != nil {
		t.Skip("docker runtime:", err)
	}
	exDir, _ := filepath.Abs("../../exercises")
	eng := New(rt, rt, rt, st, exDir)

	// Start: fault planted, sandbox running.
	sess, err := eng.Start(ctx, learner.ID, "svc-01-intranet-config", 1)
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	t.Cleanup(func() { _ = rt.Destroy(context.Background(), sess.SandboxID) })
	if sess.Status != "active" || sess.SandboxID == "" {
		t.Fatalf("session not active with a sandbox: %+v", sess)
	}
	if sess.DerivedValues["line"] == nil {
		t.Errorf("expected a derived 'line' value from break.sh, got %v", sess.DerivedValues)
	}

	// Check while broken: must fail.
	res, err := eng.Check(ctx, sess.ID)
	if err != nil {
		t.Fatalf("check (broken): %v", err)
	}
	if res.Passed {
		t.Fatalf("check passed on a broken machine")
	}

	// Simulate the learner fixing it: run the reference solution inside.
	solution, err := os.ReadFile(filepath.Join(exDir, "svc-01-intranet-config", "solution.sh"))
	if err != nil {
		t.Fatalf("read solution: %v", err)
	}
	if err := rt.CopyIn(ctx, sess.SandboxID, "/root/solution.sh", solution); err != nil {
		t.Fatalf("copy solution: %v", err)
	}
	if r, err := rt.Exec(ctx, sess.SandboxID,
		[]string{"bash", "-c", "bash /root/solution.sh"}, nil); err != nil || r.ExitCode != 0 {
		t.Fatalf("run solution: err=%v exit=%d stderr=%s", err, r.ExitCode, r.Stderr)
	}

	// Check after fix: must pass and mark solved.
	res, err = eng.Check(ctx, sess.ID)
	if err != nil {
		t.Fatalf("check (fixed): %v", err)
	}
	if !res.Passed {
		t.Fatalf("check did not pass after the fix: feedback=%q", res.Feedback)
	}
	after, err := st.SessionByID(ctx, sess.ID)
	if err != nil {
		t.Fatalf("reload session: %v", err)
	}
	if after.Status != "solved" {
		t.Errorf("status = %q, want solved", after.Status)
	}

	// Every action appears as an event (the phase's acceptance criterion).
	events, err := st.Events(ctx, sess.ID)
	if err != nil {
		t.Fatalf("events: %v", err)
	}
	var types []string
	for _, e := range events {
		types = append(types, e.Type)
	}
	want := []string{"session_started", "check_run", "check_run", "solved"}
	if len(types) != len(want) {
		t.Fatalf("event types = %v, want %v", types, want)
	}
	for i := range want {
		if types[i] != want[i] {
			t.Errorf("event[%d] = %q, want %q (all: %v)", i, types[i], want[i], types)
		}
	}
}
