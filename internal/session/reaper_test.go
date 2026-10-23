package session

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/DIZAMU/RootCause/internal/exercise"
	"github.com/DIZAMU/RootCause/internal/runtime/docker"
	"github.com/DIZAMU/RootCause/internal/testenv"
)

// TestReaperExpiresDueSession starts a real session, backdates its TTL, and
// checks the reaper expires it: status expired, sandbox gone, expired event.
func TestReaperExpiresDueSession(t *testing.T) {
	dsn := testenv.DSN(t)
	testenv.Docker(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	st := testenv.Store(t, dsn)

	rt, err := docker.New()
	if err != nil {
		testenv.NoDocker(t, "docker runtime:", err)
	}
	learnerID := testLearner(t, st, rt)
	exDir, _ := filepath.Abs("../../exercises")
	eng := New(rt, rt, rt, st, exDir)

	sess, err := eng.Start(ctx, learnerID, "svc-01-intranet-config", 1)
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

// TestReaperReclaimsOrphans starts four sandboxes from the svc-01 image and
// checks that ReclaimOrphans destroys only the one that is really an orphan of
// this database: the selftest sandbox, the sandbox owned by another database
// and the sandbox of an active session all survive.
func TestReaperReclaimsOrphans(t *testing.T) {
	dsn := testenv.DSN(t)
	testenv.Docker(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	st := testenv.Store(t, dsn)
	rt, err := docker.New()
	if err != nil {
		testenv.NoDocker(t, "docker runtime:", err)
	}
	learnerID := testLearner(t, st, rt)
	exDir, _ := filepath.Abs("../../exercises")
	eng := New(rt, rt, rt, st, exDir)

	// An active session whose sandbox must survive.
	active, err := eng.Start(ctx, learnerID, "svc-01-intranet-config", 1)
	if err != nil {
		t.Fatalf("start: %v", err)
	}

	ex, err := exercise.Load(filepath.Join(exDir, "svc-01-intranet-config"))
	if err != nil {
		t.Fatal(err)
	}
	orphanID := newID() // a well-formed id with no session row
	cases := map[string]map[string]string{
		"orphan":         {labelSession: orphanID, labelDatabase: st.Database()},
		"selftest":       {labelSession: newID(), labelDatabase: st.Database(), labelSelftest: "true"},
		"other database": {labelSession: newID(), labelDatabase: "some_other_db"},
	}
	ids := make(map[string]string)
	for name, labels := range cases {
		spec := eng.specFor(ex, "reaper-test-"+newID())
		spec.Labels = labels
		sb, err := rt.Create(ctx, spec)
		if err != nil {
			t.Fatalf("create %s sandbox: %v", name, err)
		}
		ids[name] = sb.ID
		t.Cleanup(func() { _ = rt.Destroy(context.Background(), sb.ID) })
	}

	n, err := NewReaper(eng, st, 0).ReclaimOrphans(ctx)
	if err != nil {
		t.Fatalf("reclaim: %v", err)
	}
	if n != 1 {
		t.Errorf("reclaimed %d sandboxes, want 1 (the orphan)", n)
	}

	running := make(map[string]bool)
	boxes, _ := rt.List(ctx)
	for _, b := range boxes {
		running[b.ID] = true
	}
	if running[ids["orphan"]] {
		t.Error("the orphan sandbox is still running")
	}
	for _, name := range []string{"selftest", "other database"} {
		if !running[ids[name]] {
			t.Errorf("the %s sandbox was destroyed; it must be left alone", name)
		}
	}
	if !running[active.SandboxID] {
		t.Error("the active session's sandbox was destroyed")
	}
}
