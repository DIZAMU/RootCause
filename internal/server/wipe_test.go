package server_test

import (
	"testing"
	"time"

	"github.com/DIZAMU/RootCause/internal/play"
	"github.com/DIZAMU/RootCause/internal/server"
	"github.com/DIZAMU/RootCause/internal/testenv"
)

// TestWipeOverAPI: "Reset all data" needs the confirmation, is refused while
// a session starts, and then deletes the sessions, events, statistics and
// settings, destroys the sandboxes, and leaves a fresh learner. The wipe
// empties whole tables, so the test runs in a database of its own.
func TestWipeOverAPI(t *testing.T) {
	dsn := testenv.OwnDatabase(t, "wipe")
	w := newWorldOn(t, dsn, true, func(c *server.Config) { c.CanRebuild = true })
	rows, err := play.Catalog(w.exDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := w.st.SyncExercises(w.ctx, rows); err != nil {
		t.Fatal(err)
	}
	pool := w.st.Pool()
	count := func(sql string) int {
		t.Helper()
		var n int
		if err := pool.QueryRow(w.ctx, sql).Scan(&n); err != nil {
			t.Fatalf("%s: %v", sql, err)
		}
		return n
	}

	// A start in progress holds the wipe off.
	hold, entered := make(chan struct{}), make(chan struct{})
	w.rt.SetBuildHook(func() {
		close(entered)
		<-hold
	})
	started := make(chan struct{})
	go func() {
		defer close(started)
		res := w.do("POST", "/api/start", "", map[string]string{"exercise": "perm-01-shared-folder"}, w.token)
		if res.status != 200 {
			t.Errorf("POST /api/start: status %d: %s", res.status, res.body)
		}
	}()
	select {
	case <-entered:
	case <-time.After(10 * time.Second):
		t.Fatal("the start did not reach the image build")
	}
	w.api("POST", "/api/data/reset", map[string]string{"confirm": server.WipeConfirm}, 409, nil)
	close(hold)
	<-started
	w.rt.SetBuildHook(nil)

	var ov struct {
		Active *struct{ ID string }
	}
	w.api("GET", "/api/overview", nil, 200, &ov)
	if ov.Active == nil {
		t.Fatal("no active session after the start")
	}
	w.api("POST", "/api/hint", map[string]string{"session": ov.Active.ID}, 200, nil)
	w.api("PUT", "/api/lang", map[string]string{"lang": "no"}, 204, nil)
	for _, sql := range []string{
		`CREATE SCHEMA analytics`,
		`CREATE TABLE analytics.mart_build_info (built_at timestamptz)`,
		`INSERT INTO analytics.mart_build_info VALUES (now())`,
	} {
		if _, err := pool.Exec(w.ctx, sql); err != nil {
			t.Fatalf("%s: %v", sql, err)
		}
	}
	boxes, _ := w.rt.List(w.ctx)
	if len(boxes) == 0 {
		t.Fatal("the session has no live sandbox")
	}

	// Without the confirmation nothing happens.
	w.api("POST", "/api/data/reset", nil, 400, nil)
	w.api("POST", "/api/data/reset", map[string]string{}, 400, nil)
	w.api("POST", "/api/data/reset", map[string]string{"confirm": "reset"}, 400, nil)
	if n := count(`SELECT count(*) FROM session`); n != 1 {
		t.Fatalf("a refused wipe changed the sessions: %d", n)
	}

	drain := func() {
		for {
			select {
			case <-w.builds:
			default:
				return
			}
		}
	}
	drain()
	var out struct{ Sandboxes int }
	w.api("POST", "/api/data/reset", map[string]string{"confirm": server.WipeConfirm}, 200, &out)
	if out.Sandboxes != len(boxes) {
		t.Errorf("sandboxes destroyed = %d, want %d", out.Sandboxes, len(boxes))
	}
	if left, _ := w.rt.List(w.ctx); len(left) != 0 {
		t.Errorf("sandboxes left after the wipe: %v", left)
	}
	for sql, want := range map[string]int{
		`SELECT count(*) FROM session`: 0,
		`SELECT count(*) FROM event`:   0,
		`SELECT count(*) FROM information_schema.schemata WHERE schema_name = 'analytics'`: 0,
		`SELECT count(*) FROM exercise`: len(rows),
	} {
		if n := count(sql); n != want {
			t.Errorf("%s = %d, want %d", sql, n, want)
		}
	}
	var lang string
	if err := pool.QueryRow(w.ctx, `SELECT lang FROM learner WHERE id = $1`, w.learner).Scan(&lang); err != nil {
		t.Fatalf("the learner is gone: %v", err)
	}
	if lang != "en" {
		t.Errorf("lang = %q after the wipe, want the default en", lang)
	}
	select {
	case <-w.builds:
	case <-time.After(10 * time.Second):
		t.Error("the statistics were not rebuilt after the wipe")
	}

	// The fresh state: no session, statistics never built, and a new session
	// can start.
	var fresh struct {
		Lang   string
		Active *struct{ ID string }
		Stats  struct {
			BuiltAt *time.Time `json:"built_at"`
		}
	}
	w.api("GET", "/api/overview", nil, 200, &fresh)
	if fresh.Active != nil || fresh.Stats.BuiltAt != nil || fresh.Lang != "en" {
		t.Errorf("overview after the wipe = %+v, want no session, never built, en", fresh)
	}
	w.api("GET", "/api/history", nil, 200, nil)
	w.api("POST", "/api/start", map[string]string{"exercise": "disk-01-stopped-saving"}, 200, nil)
}
