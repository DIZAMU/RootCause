package server_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/DIZAMU/RootCause/internal/cli"
	"github.com/DIZAMU/RootCause/internal/exercise"
	"github.com/DIZAMU/RootCause/internal/i18n"
	"github.com/DIZAMU/RootCause/internal/play"
	"github.com/DIZAMU/RootCause/internal/runtime/scripted"
	"github.com/DIZAMU/RootCause/internal/server"
	"github.com/DIZAMU/RootCause/internal/session"
	"github.com/DIZAMU/RootCause/internal/store"
	"github.com/DIZAMU/RootCause/internal/testenv"
	"github.com/DIZAMU/RootCause/locales"
)

// indexHTML stands in for the built web/dist/index.html: it holds the token
// placeholder in the same meta tag as web/index.html.
const indexHTML = `<!doctype html><html><head><meta name="rootcause-token" content="__RC_TOKEN__"></head><body></body></html>`

var tokenMeta = regexp.MustCompile(`<meta name="rootcause-token" content="([0-9a-f]+)">`)

// running is a dashboard served by the real Server.Serve on a real
// 127.0.0.1 listener.
type running struct {
	t     *testing.T
	base  string // http://127.0.0.1:PORT
	port  string
	token string
	srv   *server.Server
	stop  func() // cancels Serve's context and waits for Serve to return
}

// serve starts cfg on a free port. The token is read from the served
// index.html, the way the browser gets it.
func serve(t *testing.T, cfg server.Config) *running {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	srv, err := server.New(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	ln, err := server.Listen(0)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- srv.Serve(ctx, ln) }()
	stopped := false
	r := &running{
		t: t, srv: srv,
		base: "http://" + ln.Addr().String(),
		port: strconv.Itoa(ln.Addr().(*net.TCPAddr).Port),
	}
	r.stop = func() {
		if stopped {
			return
		}
		stopped = true
		cancel()
		select {
		case err := <-done:
			if err != nil {
				t.Errorf("serve: %v", err)
			}
		case <-time.After(10 * time.Second):
			t.Error("serve did not stop within 10s")
		}
	}
	t.Cleanup(r.stop)
	if cfg.Assets != nil {
		res := r.do("GET", "/", "", nil, "")
		m := tokenMeta.FindStringSubmatch(string(res.body))
		if res.status != 200 || m == nil {
			t.Fatalf("index: status %d, no token in %q", res.status, res.body)
		}
		r.token = m[1]
	}
	return r
}

type response struct {
	status int
	header http.Header
	body   []byte
}

// do sends a request with the given Host ("" = the listener address) and
// token header ("" = none).
func (r *running) do(method, path, host string, body any, token string) response {
	r.t.Helper()
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			r.t.Fatal(err)
		}
		rd = strings.NewReader(string(b))
	}
	req, err := http.NewRequest(method, r.base+path, rd)
	if err != nil {
		r.t.Fatal(err)
	}
	if host != "" {
		req.Host = host
	}
	if token != "" {
		req.Header.Set(server.TokenHeader, token)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	// A cross-site page would send its own Origin; the server must not answer
	// with any CORS header.
	req.Header.Set("Origin", "http://evil.example")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		r.t.Fatal(err)
	}
	defer res.Body.Close()
	b, err := io.ReadAll(res.Body)
	if err != nil {
		r.t.Fatal(err)
	}
	for k := range res.Header {
		if strings.HasPrefix(strings.ToLower(k), "access-control-") {
			r.t.Errorf("%s %s sent CORS header %s", method, path, k)
		}
	}
	return response{status: res.StatusCode, header: res.Header, body: b}
}

// api calls an /api endpoint with the token, checks the status and decodes
// the body into out (if not nil).
func (r *running) api(method, path string, body any, want int, out any) []byte {
	r.t.Helper()
	res := r.do(method, path, "", body, r.token)
	if res.status != want {
		r.t.Fatalf("%s %s: status %d, want %d: %s", method, path, res.status, want, res.body)
	}
	if out != nil {
		if err := json.Unmarshal(res.body, out); err != nil {
			r.t.Fatalf("%s %s: decode: %v: %s", method, path, err, res.body)
		}
	}
	return res.body
}

func TestListenBindsLoopbackOnly(t *testing.T) {
	ln, err := server.Listen(0)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	addr := ln.Addr().(*net.TCPAddr)
	if !addr.IP.Equal(net.IPv4(127, 0, 0, 1)) {
		t.Errorf("listening on %s, want 127.0.0.1 only", addr)
	}
	if server.BindAddress != "127.0.0.1" {
		t.Errorf("BindAddress = %q", server.BindAddress)
	}
}

// TestGuard covers the localhost hardening without a database: every
// request below is refused before a handler runs, or never reaches the
// store.
func TestGuard(t *testing.T) {
	bundle, err := i18n.Load(locales.FS)
	if err != nil {
		t.Fatal(err)
	}
	r := serve(t, server.Config{Bundle: bundle, Assets: fstest.MapFS{"index.html": {Data: []byte(indexHTML)}}})

	cases := []struct {
		name, method, path, host, token string
		want                            int
	}{
		{"index, rebinding host", "GET", "/", "evil.example:" + r.port, "", 403},
		{"index, right host, wrong port", "GET", "/", "localhost:1", "", 403},
		{"api, rebinding host with token", "GET", "/api/i18n/en", "evil.example:" + r.port, r.token, 403},
		{"api, no token", "GET", "/api/i18n/en", "", "", 403},
		{"api, wrong token", "GET", "/api/i18n/en", "", strings.Repeat("0", 64), 403},
		{"api post, no token", "POST", "/api/start", "", "", 403},
		{"api preflight", "OPTIONS", "/api/start", "", "", 403},
		{"stats, no token", "GET", "/api/stats", "", "", 403},
		{"rebuild, no token", "POST", "/api/stats/rebuild", "", "", 403},
		{"rebuild, rebinding host with token", "POST", "/api/stats/rebuild", "evil.example:" + r.port, r.token, 403},
		{"api, token", "GET", "/api/i18n/en", "", r.token, 200},
		{"api, localhost host", "GET", "/api/i18n/no", "localhost:" + r.port, r.token, 200},
		{"api, upper-case host", "GET", "/api/i18n/no", "LOCALHOST:" + r.port, r.token, 200},
		{"index, localhost host", "GET", "/", "localhost:" + r.port, "", 200},
		{"unknown language", "GET", "/api/i18n/de", "", r.token, 404},
	}
	for _, c := range cases {
		res := r.do(c.method, c.path, c.host, nil, c.token)
		if res.status != c.want {
			t.Errorf("%s: status %d, want %d", c.name, res.status, c.want)
		}
		if got := res.header.Get("X-Frame-Options"); got != "DENY" {
			t.Errorf("%s: X-Frame-Options %q", c.name, got)
		}
		if got := res.header.Get("Content-Security-Policy"); got != server.ContentSecurityPolicy {
			t.Errorf("%s: Content-Security-Policy %q", c.name, got)
		}
	}

	var strs map[string]string
	r.api("GET", "/api/i18n/no", nil, 200, &strs)
	if strs["web.nav.home"] != bundle.T("no", "web.nav.home", nil) || strs["check.pass"] != bundle.T("no", "check.pass", nil) {
		t.Errorf("i18n/no does not serve the Go bundle: %q, %q", strs["web.nav.home"], strs["check.pass"])
	}
}

// world is a dashboard over the real engine, store and selector, with the
// scripted runtime in place of Docker. It needs RC_TEST_DATABASE_URL.
type world struct {
	*running
	ctx     context.Context
	st      *store.Store
	eng     *session.Engine
	rt      *scripted.Runtime
	bundle  *i18n.Bundle
	learner int
	exDir   string
	builds  chan struct{} // one value per BuildStats call
}

func newWorld(t *testing.T, assets bool) *world {
	t.Helper()
	return newWorldWith(t, assets, nil)
}

// newWorldWith is newWorld with a change to the server's config before it
// starts.
func newWorldWith(t *testing.T, assets bool, change func(*server.Config)) *world {
	t.Helper()
	dsn := testenv.DSN(t)
	ctx := context.Background()
	st := testenv.Store(t, dsn)
	bundle, err := i18n.Load(locales.FS)
	if err != nil {
		t.Fatal(err)
	}
	exDir, _ := filepath.Abs("../../exercises")
	rt := &scripted.Runtime{}
	w := &world{
		ctx: ctx, st: st, rt: rt, bundle: bundle, exDir: exDir,
		eng:     session.New(rt, rt, nil, st, exDir),
		learner: testLearner(t, st),
		builds:  make(chan struct{}, 100),
	}
	cfg := server.Config{
		Store: st, Engine: w.eng, Bundle: bundle, LearnerID: w.learner, ExercisesDir: exDir,
		Catalog: func() ([]store.ExerciseRow, error) { return play.Catalog(exDir) },
		BuildStats: func(context.Context) error {
			w.builds <- struct{}{}
			return nil
		},
		Describe: func(lang string, err error) string { return cli.Message(bundle, lang, err) },
	}
	if assets {
		cfg.Assets = fstest.MapFS{"index.html": {Data: []byte(indexHTML)}}
	}
	if change != nil {
		change(&cfg)
	}
	w.running = serve(t, cfg)
	return w
}

// sessionJSON is the part of the session view the tests read.
type sessionJSON struct {
	ID       string `json:"id"`
	Exercise string `json:"exercise"`
	Topic    string `json:"topic"`
	Status   string `json:"status"`
	Stars    *int   `json:"stars"`
	Ticket   struct{ Title, From, Body string }
	Hints    []struct {
		Tier int
		Text string
	}
	HintsTotal int  `json:"hints_total"`
	CanGiveUp  bool `json:"can_give_up"`
	Checks     []struct {
		Passed   bool
		Feedback string
	}
	Events      []struct{ Type string }
	LevelChange *struct {
		Topic    string
		From, To int
	} `json:"level_change"`
	Walkthrough string `json:"walkthrough"`
	Shell       string `json:"shell_command"`
	Check       *struct {
		Result, Feedback string
		Stars            int
	}
}

func (s sessionJSON) eventTypes() string {
	var types []string
	for _, e := range s.Events {
		types = append(types, e.Type)
	}
	return strings.Join(types, ",")
}

// TestSessionOverAPI is the Phase 5 done-when at the API level: a session is
// played from the dashboard while the CLI's engine acts on the same session,
// and each side sees the other's actions, in both languages.
func TestSessionOverAPI(t *testing.T) {
	w := newWorld(t, true)
	const svc01 = "svc-01-intranet-config"
	en, err := loadTexts(w.exDir, svc01, "en")
	if err != nil {
		t.Fatal(err)
	}

	w.api("GET", "/api/session", nil, 204, nil)
	w.api("POST", "/api/start", map[string]string{"exercise": "no-such-exercise"}, 404, nil)
	var ov struct {
		Active *struct{ ID string }
		Next   *struct{ Exercise, Reason string }
		Levels []struct {
			Topic string
			Level int
		}
	}
	w.api("GET", "/api/overview", nil, 200, &ov)
	if ov.Active != nil || ov.Next == nil || len(ov.Levels) != 5 {
		t.Fatalf("overview before start: %+v", ov)
	}

	// Start from the dashboard.
	var s sessionJSON
	raw := w.api("POST", "/api/start", map[string]string{"exercise": svc01}, 200, &s)
	if s.Status != "active" || s.Exercise != svc01 || s.Topic != "services" || s.Ticket.Title != en.Ticket.Title {
		t.Fatalf("started session: %+v", s)
	}
	if len(s.Hints) != 0 || s.Walkthrough != "" || s.HintsTotal != 3 || s.Shell != server.ShellCommand {
		t.Errorf("new session shows hints %v, walkthrough %q, total %d, shell %q", s.Hints, s.Walkthrough, s.HintsTotal, s.Shell)
	}
	for _, secret := range []string{`"seed"`, `"variant`, `"derived`} {
		if strings.Contains(string(raw), secret) {
			t.Errorf("session view contains %s: %s", secret, raw)
		}
	}
	w.api("POST", "/api/start", map[string]string{"exercise": svc01}, 409, nil)

	// Derived values (break.sh output) can spell out the answer. They show
	// only inside hint 3.
	if err := w.st.SetDerived(w.ctx, s.ID, map[string]any{"line": 4242}); err != nil {
		t.Fatal(err)
	}
	if body := w.api("GET", "/api/session", nil, 200, nil); strings.Contains(string(body), "4242") {
		t.Errorf("derived value visible before hint 3: %s", body)
	}

	// The CLI reveals a hint: the dashboard sees it.
	if _, err := w.eng.Hint(w.ctx, s.ID); err != nil {
		t.Fatal(err)
	}
	w.api("GET", "/api/session", nil, 200, &s)
	if len(s.Hints) != 1 || s.Hints[0].Text != en.Hints[0] {
		t.Errorf("after CLI hint: hints %+v", s.Hints)
	}

	// The dashboard reveals hint 2, then a failed check with its symptom text.
	w.api("POST", "/api/hint", map[string]string{"session": s.ID}, 200, &s)
	if len(s.Hints) != 2 {
		t.Errorf("after dashboard hint: %d hints", len(s.Hints))
	}
	w.api("POST", "/api/hint", map[string]string{"session": "s-0000000000000000"}, 409, nil)
	w.api("POST", "/api/giveup", map[string]string{"session": s.ID}, 409, nil) // locked until hint 3
	w.rt.SetCheck(false, "site_unreachable")
	w.api("POST", "/api/check", map[string]string{"session": s.ID}, 200, &s)
	if s.Check == nil || s.Check.Result != "fail" || s.Check.Feedback != en.Feedback["site_unreachable"] {
		t.Errorf("failed check: %+v", s.Check)
	}

	// Switch to Norwegian: the ticket, hints and feedback follow.
	no, err := loadTexts(w.exDir, svc01, "no")
	if err != nil {
		t.Fatal(err)
	}
	w.api("PUT", "/api/lang", map[string]string{"lang": "no"}, 204, nil)
	w.api("GET", "/api/sessions/"+s.ID, nil, 200, &s)
	if s.Ticket.Title != no.Ticket.Title || s.Hints[0].Text != no.Hints[0] || s.Checks[0].Feedback != no.Feedback["site_unreachable"] {
		t.Errorf("Norwegian view: title %q, hint %q, feedback %q", s.Ticket.Title, s.Hints[0].Text, s.Checks[0].Feedback)
	}
	var ovLang struct{ Lang string }
	w.api("GET", "/api/overview", nil, 200, &ovLang)
	if ovLang.Lang != "no" {
		t.Errorf("overview lang %q, want no", ovLang.Lang)
	}
	var e struct{ Error string }
	w.api("POST", "/api/giveup", map[string]string{"session": s.ID}, 409, &e)
	if e.Error != w.bundle.T("no", "giveup.locked", nil) {
		t.Errorf("give up error %q, want the Norwegian giveup.locked", e.Error)
	}
	w.api("PUT", "/api/lang", map[string]string{"lang": "en"}, 204, nil)

	// The dashboard solves it: 2 stars, the walkthrough, a stats rebuild.
	for len(w.builds) > 0 {
		<-w.builds // rebuilds the overview started for this learner's stale marts
	}
	w.rt.SetCheck(true, "")
	w.api("POST", "/api/check", map[string]string{"session": s.ID}, 200, &s)
	if s.Check == nil || s.Check.Result != "pass" || s.Check.Stars != 2 || s.Status != "solved" || s.Walkthrough == "" {
		t.Fatalf("passing check: status %s, check %+v, walkthrough %d bytes", s.Status, s.Check, len(s.Walkthrough))
	}
	if !strings.Contains(s.Walkthrough, "4242") {
		t.Errorf("walkthrough does not fill {line}: %q", s.Walkthrough)
	}
	select {
	case <-w.builds:
	case <-time.After(5 * time.Second):
		t.Error("no statistics rebuild after the solve")
	}
	if got := s.eventTypes(); got != "session_started,hint_revealed,hint_revealed,check_run,check_run,solved" {
		t.Errorf("events = %s", got)
	}
	w.api("GET", "/api/session", nil, 204, nil)
	w.api("POST", "/api/check", map[string]string{"session": s.ID}, 409, nil)

	var hist []struct {
		Session, Exercise, Status string
		Stars                     *int
		Hints, Checks             int
	}
	w.api("GET", "/api/history", nil, 200, &hist)
	if len(hist) != 1 || hist[0].Session != s.ID || hist[0].Status != "solved" || *hist[0].Stars != 2 || hist[0].Hints != 2 || hist[0].Checks != 2 {
		t.Errorf("history = %+v", hist)
	}
	var exs []struct {
		ID        string
		BestStars *int `json:"best_stars"`
	}
	w.api("GET", "/api/exercises", nil, 200, &exs)
	if len(exs) != 8 {
		t.Errorf("%d exercises, want 8", len(exs))
	}
	for _, ex := range exs {
		if ex.ID == svc01 && (ex.BestStars == nil || *ex.BestStars < 2) {
			t.Errorf("svc-01 best stars %v", ex.BestStars)
		}
	}

	// Start the recommended next exercise. The selector never repeats svc-01.
	w.api("POST", "/api/start", map[string]string{"exercise": ""}, 200, &s)
	if s.Status != "active" || s.Exercise == svc01 {
		t.Fatalf("next: %+v", s)
	}
	w.api("POST", "/api/reset", map[string]string{"session": s.ID}, 200, &s)
	if !strings.Contains(s.eventTypes(), "reset") {
		t.Errorf("no reset event: %s", s.eventTypes())
	}

	// The CLI gives up after hint 3. The dashboard sees the result and the
	// walkthrough, and its own actions on that session are refused.
	for i := 0; i < 3; i++ {
		if _, err := w.eng.Hint(w.ctx, s.ID); err != nil {
			t.Fatal(err)
		}
	}
	w.api("GET", "/api/sessions/"+s.ID, nil, 200, &s)
	if !s.CanGiveUp {
		t.Error("give up not offered after hint 3")
	}
	if _, err := w.eng.GiveUp(w.ctx, s.ID); err != nil {
		t.Fatal(err)
	}
	w.api("GET", "/api/sessions/"+s.ID, nil, 200, &s)
	if s.Status != "gave_up" || s.Walkthrough == "" || s.CanGiveUp {
		t.Errorf("after CLI give up: status %s, walkthrough %d bytes, can give up %v", s.Status, len(s.Walkthrough), s.CanGiveUp)
	}
	w.api("POST", "/api/giveup", map[string]string{"session": s.ID}, 409, nil)
	w.api("GET", "/api/sessions/s-0000000000000000", nil, 404, nil)
}

// TestNotBuiltPage: a binary without the webdist tag serves a translated page
// at / that says how to build the dashboard.
func TestNotBuiltPage(t *testing.T) {
	w := newWorld(t, false)
	for _, lang := range []string{"en", "no"} {
		if err := w.st.SetLang(w.ctx, w.learner, lang); err != nil {
			t.Fatal(err)
		}
		res := w.do("GET", "/", "", nil, "")
		if res.status != 200 || !strings.Contains(string(res.body), w.bundle.T(lang, "web.not_built", nil)) {
			t.Errorf("%s: status %d, body %q", lang, res.status, res.body)
		}
	}
}

// TestActionsNeedTheShownSession: with a session the CLI started, the
// dashboard cannot start another, and an action naming any other session is
// refused, so it never lands on a session the learner is not looking at.
func TestActionsNeedTheShownSession(t *testing.T) {
	w := newWorld(t, true)
	sess, err := w.eng.Start(w.ctx, w.learner, "perm-01-shared-folder", 1)
	if err != nil {
		t.Fatal(err)
	}
	var e struct{ Error string }
	w.api("POST", "/api/start", map[string]string{"exercise": "disk-01-stopped-saving"}, 409, &e)
	if !strings.Contains(e.Error, "perm-01-shared-folder") {
		t.Errorf("start error %q does not name the active exercise", e.Error)
	}
	w.api("POST", "/api/start", map[string]string{"exercise": "no-such-exercise"}, 409, nil)
	for _, path := range []string{"/api/hint", "/api/check", "/api/reset", "/api/giveup"} {
		w.api("POST", path, map[string]string{"session": "s-0000000000000000"}, 409, nil)
		w.api("POST", path, map[string]string{}, 400, nil)
	}
	var ov struct {
		Active *struct{ ID, Exercise string }
	}
	w.api("GET", "/api/overview", nil, 200, &ov)
	if ov.Active == nil || ov.Active.ID != sess.ID {
		t.Errorf("overview active = %+v, want %s", ov.Active, sess.ID)
	}
}

// testLearner creates a learner used only by the calling test and deletes
// its rows afterwards, so the test does not depend on what other runs left.
func testLearner(t *testing.T, st *store.Store) int {
	t.Helper()
	ctx := context.Background()
	if _, err := st.EnsureLearner(ctx, "DIZAMU"); err != nil {
		t.Fatalf("ensure profile learner: %v", err)
	}
	var id int
	if err := st.Pool().QueryRow(ctx,
		`INSERT INTO learner (name) VALUES ($1) RETURNING id`, "server-test-"+t.Name()).Scan(&id); err != nil {
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

// loadTexts reads an exercise's texts the way the server does.
func loadTexts(exDir, id, lang string) (*exercise.I18n, error) {
	texts, _, err := play.Texts(exDir, store.Session{ExerciseID: id}, lang)
	if err != nil {
		return nil, err
	}
	if texts == nil {
		return nil, errors.New("no texts")
	}
	return texts, nil
}

// The policy keeps scripts to the dashboard's own files; only styles may be
// inline (the web terminal writes <style> elements).
func TestContentSecurityPolicy(t *testing.T) {
	csp := server.ContentSecurityPolicy
	for _, want := range []string{"default-src 'self'", "frame-ancestors 'none'", "base-uri 'none'", "form-action 'none'"} {
		if !strings.Contains(csp, want) {
			t.Errorf("policy %q lacks %q", csp, want)
		}
	}
	if strings.Contains(csp, "script-src") || strings.Contains(csp, "unsafe-eval") ||
		strings.Count(csp, "'unsafe-inline'") != 1 || !strings.Contains(csp, "style-src 'self' 'unsafe-inline'") {
		t.Errorf("policy %q: only style-src may allow inline, scripts must fall back to default-src 'self'", csp)
	}
}

// TestRebuildOverAPI: "rebuild now" starts a build even when the marts are
// fresh, answers 409 while one runs, and 404 where the server cannot build.
func TestRebuildOverAPI(t *testing.T) {
	release := make(chan struct{})
	started := make(chan struct{}, 10)
	w := newWorldWith(t, true, func(c *server.Config) {
		c.CanRebuild = true
		c.BuildStats = func(context.Context) error {
			started <- struct{}{}
			<-release
			return nil
		}
	})
	var page struct {
		CanRebuild bool `json:"can_rebuild"`
		Built      bool `json:"built"`
	}
	w.api("GET", "/api/stats", nil, 200, &page)
	if !page.CanRebuild {
		t.Error("can_rebuild is false with CanRebuild set")
	}
	w.api("POST", "/api/stats/rebuild", nil, 202, nil)
	select {
	case <-started:
	case <-time.After(10 * time.Second):
		t.Fatal("the rebuild did not start")
	}
	w.api("POST", "/api/stats/rebuild", nil, 409, nil)
	close(release)

	off := newWorldWith(t, true, func(c *server.Config) { c.CanRebuild = false })
	off.api("GET", "/api/stats", nil, 200, &page)
	if page.CanRebuild {
		t.Error("can_rebuild is true without CanRebuild")
	}
	off.api("POST", "/api/stats/rebuild", nil, 404, nil)
}

// TestStartProgressInOverview: while a start runs, the overview reports its
// exercise and step, and nothing from the exercise's variant.
func TestStartProgressInOverview(t *testing.T) {
	w := newWorld(t, true)
	hold, entered := make(chan struct{}), make(chan struct{})
	w.rt.SetBuildHook(func() {
		close(entered)
		<-hold
	})
	done := make(chan struct{})
	go func() {
		defer close(done)
		// Not w.api: it calls Fatalf, which may not run outside the test's
		// own goroutine.
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

	raw := w.api("GET", "/api/overview", nil, 200, nil)
	var ov struct {
		Starting *struct {
			Exercise    string
			Step, Steps int
		}
	}
	if err := json.Unmarshal(raw, &ov); err != nil {
		t.Fatal(err)
	}
	if ov.Starting == nil || ov.Starting.Exercise != "perm-01-shared-folder" || ov.Starting.Step != 0 || ov.Starting.Steps != session.StartSteps {
		t.Errorf("starting = %+v, want perm-01 at step 0 of %d", ov.Starting, session.StartSteps)
	}
	ex, err := exercise.Load(filepath.Join(w.exDir, "perm-01-shared-folder"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "RC_") {
		t.Error("the overview contains an RC_ variable name")
	}
	// The start itself carries exactly three fields, none from the variant.
	// (The overview's next pick may show another ticket header, filled for
	// display; see play.CatalogTicket.)
	var fields struct {
		Starting map[string]json.RawMessage `json:"starting"`
	}
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatal(err)
	}
	if len(fields.Starting) != 3 || fields.Starting["exercise"] == nil || fields.Starting["step"] == nil || fields.Starting["steps"] == nil {
		t.Errorf("starting has fields %v, want exactly exercise, step, steps", fields.Starting)
	}
	startingRaw, _ := json.Marshal(fields.Starting)
	for name, values := range ex.Variants {
		for _, v := range values {
			if strings.Contains(string(startingRaw), v) || strings.Contains(string(startingRaw), name) {
				t.Errorf("starting contains variant %s or its value %q: %s", name, v, startingRaw)
			}
		}
	}
	close(hold)
	<-done
	w.api("GET", "/api/overview", nil, 200, &ov)
	if ov.Starting != nil {
		t.Errorf("starting is still set after the start: %+v", ov.Starting)
	}
}
