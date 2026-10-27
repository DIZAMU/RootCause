package server

import (
	"context"
	"encoding/json"
	"errors"
	"math/rand"
	"net/http"
	"regexp"
	"slices"
	"time"

	"github.com/DIZAMU/RootCause/internal/i18n"
	"github.com/DIZAMU/RootCause/internal/play"
	"github.com/DIZAMU/RootCause/internal/selector"
	"github.com/DIZAMU/RootCause/internal/session"
	"github.com/DIZAMU/RootCause/internal/store"
)

// historyLimit is how many finished sessions the history returns.
const historyLimit = 200

// ansiEscape matches the terminal color codes in dbt's output, which a
// browser would show as garbage.
var ansiEscape = regexp.MustCompile(`\x1b\[[0-9;]*[A-Za-z]`)

// plainText removes terminal color codes from s.
func plainText(s string) string { return ansiEscape.ReplaceAllString(s, "") }

// maxBody bounds a request body. Every request body is a tiny JSON object.
const maxBody = 4 << 10

func (s *Server) routes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/overview", s.handleOverview)
	mux.HandleFunc("GET /api/exercises", s.handleExercises)
	mux.HandleFunc("GET /api/history", s.handleHistory)
	mux.HandleFunc("GET /api/session", s.handleActive)
	mux.HandleFunc("GET /api/sessions/{id}", s.handleSession)
	mux.HandleFunc("POST /api/start", s.handleStart)
	mux.HandleFunc("POST /api/hint", s.handleHint)
	mux.HandleFunc("POST /api/check", s.handleCheck)
	mux.HandleFunc("POST /api/reset", s.handleReset)
	mux.HandleFunc("POST /api/giveup", s.handleGiveUp)
	mux.HandleFunc("PUT /api/lang", s.handleSetLang)
	mux.HandleFunc("GET /api/i18n/{lang}", s.handleStrings)
	mux.HandleFunc("GET "+TerminalPath, s.handleTerminal)
}

// --- responses ---------------------------------------------------------------

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// errorBody is every error response: a message already in the learner's
// language.
type errorBody struct {
	Error string `json:"error"`
}

// fail writes a translated locale key as an error.
func (s *Server) fail(w http.ResponseWriter, status int, lang, key string, vars map[string]string) {
	writeJSON(w, status, errorBody{Error: s.cfg.Bundle.T(lang, key, vars)})
}

// failErr writes err as a 500, rendered the way the CLI renders it.
func (s *Server) failErr(w http.ResponseWriter, lang string, err error) {
	status := http.StatusInternalServerError
	if errors.Is(err, session.ErrGiveUpTooEarly) {
		status = http.StatusConflict
	}
	writeJSON(w, status, errorBody{Error: s.cfg.Describe(lang, err)})
}

// decode reads a small JSON request body into v.
func decode(r *http.Request, v any) error {
	return json.NewDecoder(http.MaxBytesReader(nil, r.Body, maxBody)).Decode(v)
}

// --- read endpoints ----------------------------------------------------------

type overviewJSON struct {
	Lang   string        `json:"lang"`
	Active *activeJSON   `json:"active"`
	Next   *nextJSON     `json:"next"`
	Source string        `json:"source"` // marts | fallback
	Levels []levelJSON   `json:"levels"`
	Review *[]reviewJSON `json:"review"` // null until the marts are built
	Stats  statsJSON     `json:"stats"`
}

type activeJSON struct {
	ID       string `json:"id"`
	Exercise string `json:"exercise"`
}

type nextJSON struct {
	Exercise   string `json:"exercise"`
	Topic      string `json:"topic"`
	Difficulty int    `json:"difficulty"`
	Reason     string `json:"reason"` // review | weakest_topic
	Level      int    `json:"level"`
}

type levelJSON struct {
	Topic string `json:"topic"`
	Level int    `json:"level"`
}

type reviewJSON struct {
	Exercise string    `json:"exercise"`
	DueAt    time.Time `json:"due_at"`
	DueNow   bool      `json:"due_now"`
}

type statsJSON struct {
	Fresh    bool       `json:"fresh"`
	Updating bool       `json:"updating"`
	Error    string     `json:"error"`    // the last rebuild's failure, translated; "" if none
	BuiltAt  *time.Time `json:"built_at"` // when dbt last built the marts; null if never
}

// handleOverview is the home view: the active session, a preview of the next
// pick, topic levels, the review queue and the state of the statistics. The
// preview never rebuilds the marts itself; if they are stale and the last
// rebuild did not fail, it starts one in the background, and the next poll
// shows its result.
func (s *Server) handleOverview(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := s.lang(ctx)
	out := overviewJSON{Lang: lang, Levels: []levelJSON{}}

	if sess, ok, err := s.active(ctx); err != nil {
		s.failErr(w, lang, err)
		return
	} else if ok {
		out.Active = &activeJSON{ID: sess.ID, Exercise: sess.ExerciseID}
	}

	fresh, err := s.cfg.Store.MartsFresh(ctx, s.cfg.LearnerID)
	if err != nil {
		s.failErr(w, lang, err)
		return
	}
	if !fresh && !s.stats.busy() && s.stats.err() == nil {
		s.stats.trigger()
	}

	catalog, err := s.cfg.Catalog()
	if err != nil {
		s.failErr(w, lang, err)
		return
	}
	rec, err := play.Recommend(ctx, s.deps(nil), s.cfg.LearnerID, catalog, time.Now())
	switch {
	case errors.Is(err, selector.ErrNoCandidate):
	case err != nil:
		s.failErr(w, lang, err)
		return
	default:
		p := rec.Pick.Exercise
		out.Next = &nextJSON{Exercise: p.ID, Topic: p.Topic, Difficulty: p.Difficulty, Reason: string(rec.Pick.Reason), Level: rec.Level}
	}
	out.Source = string(rec.Source)

	for _, topic := range play.Topics {
		level, err := s.cfg.Engine.TopicLevel(ctx, s.cfg.LearnerID, topic)
		if err != nil {
			s.failErr(w, lang, err)
			return
		}
		out.Levels = append(out.Levels, levelJSON{Topic: topic, Level: level})
	}

	queue, err := s.cfg.Store.MartReviewQueue(ctx, s.cfg.LearnerID)
	switch {
	case errors.Is(err, store.ErrNoMarts):
	case err != nil:
		s.failErr(w, lang, err)
		return
	default:
		now := time.Now()
		review := []reviewJSON{}
		for _, q := range queue {
			review = append(review, reviewJSON{Exercise: q.ExerciseID, DueAt: q.DueAt, DueNow: !q.DueAt.After(now)})
		}
		out.Review = &review
	}

	out.Stats = statsJSON{Fresh: fresh, Updating: s.stats.busy()}
	if out.Stats.BuiltAt, err = s.cfg.Store.StatsBuiltAt(ctx); err != nil {
		s.failErr(w, lang, err)
		return
	}
	if err := s.stats.err(); err != nil {
		out.Stats.Error = plainText(s.cfg.Describe(lang, err))
	}
	writeJSON(w, http.StatusOK, out)
}

type exerciseJSON struct {
	ID         string `json:"id"`
	Topic      string `json:"topic"`
	Difficulty int    `json:"difficulty"`
	BestStars  *int   `json:"best_stars"` // null if never solved
}

// handleExercises lists the catalog with the best stars per exercise, as
// rootcause list does.
func (s *Server) handleExercises(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := s.lang(ctx)
	catalog, err := s.cfg.Catalog()
	if err != nil {
		s.failErr(w, lang, err)
		return
	}
	best, err := s.cfg.Store.BestStars(ctx)
	if err != nil {
		s.failErr(w, lang, err)
		return
	}
	slices.SortFunc(catalog, func(a, b store.ExerciseRow) int {
		switch {
		case a.ID < b.ID:
			return -1
		case a.ID > b.ID:
			return 1
		}
		return 0
	})
	out := []exerciseJSON{}
	for _, ex := range catalog {
		e := exerciseJSON{ID: ex.ID, Topic: ex.Topic, Difficulty: ex.Difficulty}
		if n, ok := best[ex.ID]; ok {
			e.BestStars = &n
		}
		out = append(out, e)
	}
	writeJSON(w, http.StatusOK, out)
}

type historyJSON struct {
	Session   string     `json:"session"`
	Exercise  string     `json:"exercise"`
	Status    string     `json:"status"`
	Stars     *int       `json:"stars"`
	Hints     int        `json:"hints"`
	Checks    int        `json:"checks"`
	StartedAt time.Time  `json:"started_at"`
	EndedAt   *time.Time `json:"ended_at"`
}

func (s *Server) handleHistory(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	items, err := s.cfg.Store.History(ctx, s.cfg.LearnerID, historyLimit)
	if err != nil {
		s.failErr(w, s.lang(ctx), err)
		return
	}
	out := []historyJSON{}
	for _, h := range items {
		out = append(out, historyJSON{
			Session: h.SessionID, Exercise: h.ExerciseID, Status: h.Status, Stars: h.Stars,
			Hints: h.HintsRevealed, Checks: h.Checks, StartedAt: h.StartedAt, EndedAt: h.EndedAt,
		})
	}
	writeJSON(w, http.StatusOK, out)
}

// handleActive returns the active session, or 204 if there is none.
func (s *Server) handleActive(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := s.lang(ctx)
	sess, ok, err := s.active(ctx)
	if err != nil {
		s.failErr(w, lang, err)
		return
	}
	if !ok {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	s.writeSession(w, ctx, lang, sess, nil)
}

// handleSession returns one of the learner's sessions, in any status, so the
// dashboard can show the result of a session the CLI just finished.
func (s *Server) handleSession(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := s.lang(ctx)
	sess, err := s.cfg.Store.SessionByID(ctx, r.PathValue("id"))
	if store.IsNoRows(err) || (err == nil && sess.LearnerID != s.cfg.LearnerID) {
		s.fail(w, http.StatusNotFound, lang, "web.error.no_session", nil)
		return
	}
	if err != nil {
		s.failErr(w, lang, err)
		return
	}
	s.writeSession(w, ctx, lang, sess, nil)
}

func (s *Server) handleStrings(w http.ResponseWriter, r *http.Request) {
	lang := r.PathValue("lang")
	if !slices.Contains(i18n.Languages, lang) {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusOK, s.cfg.Bundle.Strings(lang))
}

func (s *Server) handleSetLang(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var body struct {
		Lang string `json:"lang"`
	}
	if err := decode(r, &body); err != nil || !slices.Contains(i18n.Languages, body.Lang) {
		s.fail(w, http.StatusBadRequest, s.lang(ctx), "web.error.bad_request", nil)
		return
	}
	if err := s.cfg.Store.SetLang(ctx, s.cfg.LearnerID, body.Lang); err != nil {
		s.failErr(w, s.lang(ctx), err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- session actions ---------------------------------------------------------

// handleStart starts an exercise: the one named, or with "" the recommended
// next one, exactly as rootcause next picks it (rebuilding stale marts
// first).
func (s *Server) handleStart(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := s.lang(ctx)
	var body struct {
		Exercise string `json:"exercise"`
	}
	if err := decode(r, &body); err != nil {
		s.fail(w, http.StatusBadRequest, lang, "web.error.bad_request", nil)
		return
	}
	s.actions.Lock()
	defer s.actions.Unlock()

	if cur, ok, err := s.active(ctx); err != nil {
		s.failErr(w, lang, err)
		return
	} else if ok {
		s.fail(w, http.StatusConflict, lang, "start.active_exists", map[string]string{"exercise": cur.ExerciseID})
		return
	}
	catalog, err := s.cfg.Catalog()
	if err != nil {
		s.failErr(w, lang, err)
		return
	}

	random := s.cfg.Random
	if random == nil {
		random = rand.Int63
	}
	var exerciseID string
	var seed int64
	if body.Exercise != "" {
		if !slices.ContainsFunc(catalog, func(e store.ExerciseRow) bool { return e.ID == body.Exercise }) {
			s.fail(w, http.StatusNotFound, lang, "web.error.unknown_exercise", map[string]string{"id": body.Exercise})
			return
		}
		exerciseID, seed = body.Exercise, random()
	} else {
		rec, err := play.Recommend(ctx, s.deps(s.stats.run), s.cfg.LearnerID, catalog, time.Now())
		if errors.Is(err, selector.ErrNoCandidate) {
			s.fail(w, http.StatusConflict, lang, "web.next.none", nil)
			return
		}
		if err != nil {
			s.failErr(w, lang, err)
			return
		}
		if seed, err = play.Seed(s.cfg.ExercisesDir, rec.Pick, random); err != nil {
			s.failErr(w, lang, err)
			return
		}
		exerciseID = rec.Pick.Exercise.ID
	}

	sess, err := s.cfg.Engine.Start(ctx, s.cfg.LearnerID, exerciseID, seed)
	if errors.Is(err, store.ErrActiveSession) {
		// The CLI started one first (migration 0002 keeps one active session).
		s.fail(w, http.StatusConflict, lang, "web.error.active_exists", nil)
		return
	}
	if err != nil {
		s.failErr(w, lang, err)
		return
	}
	s.writeSession(w, ctx, lang, sess, nil)
}

// actionBody names the session the dashboard shows, so an action never lands
// on a different session that the CLI started in the meantime.
type actionBody struct {
	Session string `json:"session"`
}

// target reads the action body and returns the active session if it is the
// one named. Otherwise it writes the error response and returns false.
func (s *Server) target(w http.ResponseWriter, r *http.Request, lang string) (store.Session, bool) {
	var body actionBody
	if err := decode(r, &body); err != nil || body.Session == "" {
		s.fail(w, http.StatusBadRequest, lang, "web.error.bad_request", nil)
		return store.Session{}, false
	}
	sess, ok, err := s.active(r.Context())
	if err != nil {
		s.failErr(w, lang, err)
		return store.Session{}, false
	}
	if !ok || sess.ID != body.Session {
		s.fail(w, http.StatusConflict, lang, "web.error.not_active", nil)
		return store.Session{}, false
	}
	return sess, true
}

func (s *Server) handleHint(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := s.lang(ctx)
	s.actions.Lock()
	defer s.actions.Unlock()
	sess, ok := s.target(w, r, lang)
	if !ok {
		return
	}
	tier, err := s.cfg.Engine.Hint(ctx, sess.ID)
	if err != nil {
		s.failErr(w, lang, err)
		return
	}
	if tier == sess.HintsRevealed {
		s.fail(w, http.StatusConflict, lang, "web.hint.none_more", nil)
		return
	}
	s.writeSessionByID(w, ctx, lang, sess.ID, nil)
}

// checkJSON is the result of one check, for the dashboard to announce.
type checkJSON struct {
	Result   string `json:"result"`   // pass | fail | infra
	Feedback string `json:"feedback"` // the symptom text for a fail, "" if none
	Stars    int    `json:"stars"`
}

func (s *Server) handleCheck(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := s.lang(ctx)
	s.actions.Lock()
	defer s.actions.Unlock()
	sess, ok := s.target(w, r, lang)
	if !ok {
		return
	}
	res, err := s.cfg.Engine.Check(ctx, sess.ID)
	if err != nil {
		s.failErr(w, lang, err)
		return
	}
	out := &checkJSON{Result: "fail"}
	switch {
	case res.Infra:
		out.Result = "infra"
	case res.Passed:
		out.Result, out.Stars = "pass", res.Stars
		s.stats.trigger()
	default:
		texts, vars, err := play.Texts(s.cfg.ExercisesDir, sess, lang)
		if err != nil {
			s.failErr(w, lang, err)
			return
		}
		out.Feedback = play.FeedbackText(texts, res.Feedback, vars)
	}
	s.writeSessionByID(w, ctx, lang, sess.ID, out)
}

func (s *Server) handleReset(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := s.lang(ctx)
	s.actions.Lock()
	defer s.actions.Unlock()
	sess, ok := s.target(w, r, lang)
	if !ok {
		return
	}
	if err := s.cfg.Engine.Reset(ctx, sess.ID); err != nil {
		s.failErr(w, lang, err)
		return
	}
	s.writeSessionByID(w, ctx, lang, sess.ID, nil)
}

func (s *Server) handleGiveUp(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := s.lang(ctx)
	s.actions.Lock()
	defer s.actions.Unlock()
	sess, ok := s.target(w, r, lang)
	if !ok {
		return
	}
	if _, err := s.cfg.Engine.GiveUp(ctx, sess.ID); err != nil {
		s.failErr(w, lang, err)
		return
	}
	s.stats.trigger()
	s.writeSessionByID(w, ctx, lang, sess.ID, nil)
}

// --- helpers -----------------------------------------------------------------

// deps is the selector's input, the same as rootcause next uses. refresh nil
// previews without rebuilding.
func (s *Server) deps(refresh func(context.Context) error) selector.Deps {
	return selector.Deps{Store: s.cfg.Store, Level: s.cfg.Engine.TopicLevel, Refresh: refresh}
}

func (s *Server) active(ctx context.Context) (store.Session, bool, error) {
	sess, err := s.cfg.Store.ActiveSession(ctx, s.cfg.LearnerID)
	if store.IsNoRows(err) {
		return store.Session{}, false, nil
	}
	if err != nil {
		return store.Session{}, false, err
	}
	return sess, true, nil
}
