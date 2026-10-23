package selector_test

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"math/rand"
	"os/exec"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/DIZAMU/RootCause/internal/dbt"
	"github.com/DIZAMU/RootCause/internal/exercise"
	"github.com/DIZAMU/RootCause/internal/play"
	"github.com/DIZAMU/RootCause/internal/runtime/scripted"
	"github.com/DIZAMU/RootCause/internal/selector"
	"github.com/DIZAMU/RootCause/internal/session"
	"github.com/DIZAMU/RootCause/internal/store"
)

// result is how a scripted session ends.
type result int

const (
	solve3      result = iota // solved with no hints: 3 stars
	solve1                    // solved after hint 3: 1 star
	giveUp                    // gave up after hint 3
	expiredFail               // a failed check, then expired
)

// world is the test's engine, store and learner.
type world struct {
	t        *testing.T
	ctx      context.Context
	st       *store.Store
	eng      *session.Engine
	rt       *scripted.Runtime
	learner  int
	exDir    string
	project  string
	dsn      string
	catalog  []selector.Exercise
	lastSeed map[string]int64
}

// play runs one session of exerciseID through the real engine, ending as res,
// then moves all its timestamps ageHours into the past. Sessions are played
// oldest first, so the order the engine saw matches the backdated order.
func (w *world) play(exerciseID string, res result, ageHours int) {
	w.t.Helper()
	seed := int64(len(w.lastSeed) + 1)
	sess, err := w.eng.Start(w.ctx, w.learner, exerciseID, seed)
	if err != nil {
		w.t.Fatalf("start %s: %v", exerciseID, err)
	}
	w.lastSeed[exerciseID] = seed
	if res != solve3 && res != expiredFail {
		for i := 0; i < 3; i++ {
			if _, err := w.eng.Hint(w.ctx, sess.ID); err != nil {
				w.t.Fatalf("hint: %v", err)
			}
		}
	}
	switch res {
	case solve3, solve1:
		w.rt.SetCheck(true, "")
		if r, err := w.eng.Check(w.ctx, sess.ID); err != nil || !r.Passed {
			w.t.Fatalf("check %s: passed=%v err=%v", exerciseID, r.Passed, err)
		}
	case giveUp:
		if _, err := w.eng.GiveUp(w.ctx, sess.ID); err != nil {
			w.t.Fatalf("give up %s: %v", exerciseID, err)
		}
	case expiredFail:
		w.rt.SetCheck(false, "x")
		if r, err := w.eng.Check(w.ctx, sess.ID); err != nil || r.Passed {
			w.t.Fatalf("check %s: passed=%v err=%v", exerciseID, r.Passed, err)
		}
		cur, err := w.st.SessionByID(w.ctx, sess.ID)
		if err != nil {
			w.t.Fatal(err)
		}
		if _, err := w.eng.Expire(w.ctx, cur); err != nil {
			w.t.Fatalf("expire %s: %v", exerciseID, err)
		}
	}
	age := fmt.Sprintf("%d hours", ageHours)
	for _, sql := range []string{
		`UPDATE session SET started_at = started_at - $2::interval, ended_at = ended_at - $2::interval,
		        expires_at = expires_at - $2::interval WHERE id = $1`,
		`UPDATE event SET occurred_at = occurred_at - $2::interval WHERE session_id = $1`,
	} {
		if _, err := w.st.Pool().Exec(w.ctx, sql, sess.ID, age); err != nil {
			w.t.Fatalf("backdate: %v", err)
		}
	}
}

// pick runs the production selection path: Load (refreshing the marts with
// the real dbt container when they are stale), then Next.
func (w *world) pick(refresh bool) (selector.Pick, selector.Loaded) {
	w.t.Helper()
	deps := selector.Deps{Store: w.st, Level: w.eng.TopicLevel}
	if refresh {
		deps.Refresh = func(ctx context.Context) error { return dbt.Build(ctx, w.project, w.dsn) }
	}
	loaded, err := selector.Load(w.ctx, deps, w.learner, w.catalog)
	if err != nil {
		w.t.Fatalf("load: %v", err)
	}
	if loaded.RefreshErr != nil {
		w.t.Fatalf("dbt refresh: %v", loaded.RefreshErr)
	}
	p, err := selector.Next(loaded.Input, time.Now())
	if err != nil {
		w.t.Fatalf("next: %v", err)
	}
	return p, loaded
}

// expect picks with fresh marts and checks the exercise and the reason.
func (w *world) expect(step, wantID string, wantReason selector.Reason) selector.Pick {
	w.t.Helper()
	p, loaded := w.pick(true)
	if loaded.Source != selector.SourceMarts {
		w.t.Fatalf("%s: source = %s, want marts", step, loaded.Source)
	}
	if p.Exercise.ID != wantID || p.Reason != wantReason {
		w.t.Fatalf("%s: picked %s (%s), want %s (%s)", step, p.Exercise.ID, p.Reason, wantID, wantReason)
	}
	return p
}

// TestScriptedSelection is the Phase 4 acceptance test (spec section 18):
// after a scripted series of sessions, the selector picks the expected
// exercises. Sessions run through the real engine and store, the marts are
// built by the real dbt container, and picks go through the real Load and
// Next. It needs Docker and RC_TEST_DATABASE_URL, and uses its own learner.
func TestScriptedSelection(t *testing.T) {
	dsn := store.TestDSN()
	if dsn == "" {
		t.Skip("RC_TEST_DATABASE_URL not set to a database ending in _test")
	}
	if err := exec.Command("docker", "info").Run(); err != nil {
		t.Skip("docker daemon not reachable:", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	st, err := store.Open(ctx, dsn)
	if err != nil {
		t.Skip("db not reachable:", err)
	}
	t.Cleanup(st.Close)
	if err := st.Migrate(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	project, _ := filepath.Abs("../../dbt")
	if err := dbt.Build(ctx, project, dsn); errors.Is(err, dbt.ErrUnsupportedDatabase) {
		t.Skip("dbt needs the compose Postgres:", err)
	} else if err != nil {
		t.Fatalf("first dbt build: %v", err)
	}

	exDir, _ := filepath.Abs("../../exercises")
	rt := &scripted.Runtime{}
	w := &world{
		t: t, ctx: ctx, st: st, rt: rt, exDir: exDir, project: project, dsn: dsn,
		eng:      session.New(rt, rt, nil, st, exDir),
		learner:  testLearner(t, st),
		catalog:  loadCatalog(t, exDir),
		lastSeed: make(map[string]int64),
	}

	// A new learner: every topic is level 1 and never practiced, so the topic
	// name decides (disk), and disk-01 is its only exercise.
	w.expect("new learner", "disk-01-stopped-saving", selector.ReasonWeakest)
	w.play("disk-01-stopped-saving", solve3, 288)

	// Never-practiced topics come before practiced ones: logs.
	w.expect("never practiced first", "log-01-crash-on-start", selector.ReasonWeakest)
	w.play("log-01-crash-on-start", solve3, 264)

	// networking: difficulty 1 (net-02) is closest to level 1.
	w.expect("closest difficulty", "net-02-outside-only", selector.ReasonWeakest)
	w.play("net-02-outside-only", solve3, 240)
	w.play("net-01-no-names", solve3, 216) // two strong solves: networking level 2
	w.play("perm-01-shared-folder", solve3, 192)
	w.play("svc-01-intranet-config", solve3, 168)

	// Level 1 topics: disk, logs, permissions, services. disk was practiced
	// longest ago. networking is level 2, so it waits.
	w.expect("least recently practiced", "disk-01-stopped-saving", selector.ReasonWeakest)

	w.play("log-01-crash-on-start", giveUp, 120) // failed 5 days ago...
	w.play("perm-02-backup-denied", solve1, 96)  // 1 star 4 days ago: due 1 day ago
	w.play("svc-02-dies-on-reboot", solve3, 84)  // services level 2
	w.play("log-01-crash-on-start", solve3, 48)  // ...solved with 3 stars 2 days ago: not due
	w.play("disk-01-stopped-saving", solve1, 24) // 1 star 1 day ago: due in 2 days

	p := w.expect("review first", "perm-02-backup-denied", selector.ReasonReview)
	queue, err := st.MartReviewQueue(ctx, w.learner)
	if err != nil {
		t.Fatal(err)
	}
	inQueue := map[string]time.Time{}
	for _, r := range queue {
		inQueue[r.ExerciseID] = r.DueAt
	}
	if _, ok := inQueue["log-01-crash-on-start"]; ok {
		t.Error("log-01 is in the review queue, but its fail was followed by a 3-star solve")
	}
	if due, ok := inQueue["disk-01-stopped-saving"]; !ok || !due.After(time.Now()) {
		t.Errorf("disk-01 (1 star a day ago) should be queued and not yet due; queued=%v due=%v", ok, due)
	}
	// The review returns as a new variant.
	ex, err := exercise.Load(filepath.Join(exDir, "perm-02-backup-denied"))
	if err != nil {
		t.Fatal(err)
	}
	if p.Due.LastSeed != w.lastSeed["perm-02-backup-denied"] {
		t.Errorf("queue's last seed = %d, want %d", p.Due.LastSeed, w.lastSeed["perm-02-backup-denied"])
	}
	seed, err := play.Seed(exDir, p, rand.Int63)
	if err != nil {
		t.Fatal(err)
	}
	if maps.Equal(ex.SelectVariant(seed), p.Due.LastVariant) {
		t.Errorf("review seed %d repeats the last variant %v", seed, p.Due.LastVariant)
	}

	w.play("net-02-outside-only", giveUp, 20)
	w.play("net-01-no-names", expiredFail, 16)  // two fails: networking back to level 1
	w.play("perm-02-backup-denied", solve3, 10) // the review, solved: leaves the queue

	// Nothing is due (disk-01 is due in 2 days). services is level 2 although
	// practiced longest ago; among the level 1 topics, logs was practiced
	// longest ago.
	w.expect("lowest level beats least recently practiced", "log-01-crash-on-start", selector.ReasonWeakest)

	// dbt and the engine agree on every level.
	levels, err := st.MartTopicLevels(ctx, w.learner)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]int{"disk": 1, "logs": 1, "networking": 1, "permissions": 1, "services": 2}
	for topic, lv := range want {
		engine, err := w.eng.TopicLevel(ctx, w.learner, topic)
		if err != nil {
			t.Fatal(err)
		}
		if levels[topic].Level != lv || engine != lv {
			t.Errorf("%s: mart level %d, engine level %d, want %d", topic, levels[topic].Level, engine, lv)
		}
	}
	var changes int
	if err := st.Pool().QueryRow(ctx, `SELECT count(*) FROM event e JOIN session s ON s.id = e.session_id
		WHERE s.learner_id = $1 AND e.type = 'level_changed'`, w.learner).Scan(&changes); err != nil {
		t.Fatal(err)
	}
	if changes != 3 { // networking up and down, services up
		t.Errorf("%d level_changed events, want 3", changes)
	}

	// Without the marts, the fallback (engine levels, no review) makes the
	// same pick.
	if _, err := st.Pool().Exec(ctx, `DROP SCHEMA analytics CASCADE`); err != nil {
		t.Fatalf("drop analytics: %v", err)
	}
	fp, loaded := w.pick(false)
	if loaded.Source != selector.SourceFallback || fp.Exercise.ID != "log-01-crash-on-start" {
		t.Errorf("fallback: source %s, picked %s; want fallback, log-01", loaded.Source, fp.Exercise.ID)
	}
	if loaded.Input.Review != nil {
		t.Errorf("fallback has a review queue: %v", loaded.Input.Review)
	}
	// Leave the marts built for whoever runs next.
	if err := dbt.Build(ctx, project, dsn); err != nil {
		t.Fatalf("rebuild marts: %v", err)
	}
}

// loadCatalog loads every exercise through the CLI's catalog loader.
func loadCatalog(t *testing.T, exDir string) []selector.Exercise {
	t.Helper()
	rows, err := play.Catalog(exDir)
	if err != nil {
		t.Fatal(err)
	}
	var out []selector.Exercise
	for _, r := range rows {
		out = append(out, selector.Exercise{ID: r.ID, Topic: r.Topic, Difficulty: r.Difficulty})
	}
	if len(out) != 8 || !slices.ContainsFunc(out, func(e selector.Exercise) bool { return e.ID == "svc-02-dies-on-reboot" }) {
		t.Fatalf("catalog = %v, want the 8 v1 exercises", out)
	}
	return out
}

// testLearner creates a learner used only by this test and deletes its rows
// afterwards, so the test does not depend on what other runs left behind.
func testLearner(t *testing.T, st *store.Store) int {
	t.Helper()
	ctx := context.Background()
	if _, err := st.EnsureLearner(ctx, "DIZAMU"); err != nil {
		t.Fatalf("ensure profile learner: %v", err)
	}
	var id int
	if err := st.Pool().QueryRow(ctx,
		`INSERT INTO learner (name) VALUES ($1) RETURNING id`, "selector-test-"+t.Name()).Scan(&id); err != nil {
		t.Fatalf("create test learner: %v", err)
	}
	t.Cleanup(func() {
		for _, sql := range []string{
			`DELETE FROM event WHERE session_id IN (SELECT id FROM session WHERE learner_id = $1)`,
			`DELETE FROM session WHERE learner_id = $1`,
			`DELETE FROM learner WHERE id = $1`,
		} {
			if _, err := st.Pool().Exec(context.Background(), sql, id); err != nil {
				t.Errorf("cleanup %q: %v", sql, err)
			}
		}
	})
	return id
}
