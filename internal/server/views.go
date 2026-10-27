package server

import (
	"context"
	"fmt"
	"net/http"
	"path/filepath"
	"time"

	"github.com/DIZAMU/RootCause/internal/exercise"
	"github.com/DIZAMU/RootCause/internal/i18n"
	"github.com/DIZAMU/RootCause/internal/play"
	"github.com/DIZAMU/RootCause/internal/store"
)

// ShellCommand opens a shell in the active sandbox (spec section 11: the
// dashboard shows it; v2 embeds a terminal instead).
const ShellCommand = "rootcause shell"

// sessionJSON is one session as the exercise view shows it. Exercise texts
// are filled server-side in the learner's language, as the CLI fills them.
// It holds only what the learner may see: revealed hints, and the
// walkthrough only once the session is solved or given up. Seeds, variant
// values and derived values are never sent, since derived values can spell
// out the answer.
type sessionJSON struct {
	ID           string          `json:"id"`
	Exercise     string          `json:"exercise"`
	Topic        string          `json:"topic"`
	Difficulty   int             `json:"difficulty"`
	Status       string          `json:"status"` // active | solved | gave_up | expired | voided
	Stars        *int            `json:"stars"`
	StartedAt    time.Time       `json:"started_at"`
	EndedAt      *time.Time      `json:"ended_at"`
	ExpiresAt    time.Time       `json:"expires_at"`
	Ticket       ticketJSON      `json:"ticket"`
	Hints        []hintJSON      `json:"hints"`
	HintsTotal   int             `json:"hints_total"`
	CanGiveUp    bool            `json:"can_give_up"`
	Checks       []checkRunJSON  `json:"checks"`
	Events       []eventJSON     `json:"events"`
	LevelChange  *levelChangeRow `json:"level_change"`
	Walkthrough  string          `json:"walkthrough"`
	ShellCommand string          `json:"shell_command"`
	Terminal     bool            `json:"terminal"`        // the dashboard has a web terminal (v2)
	Check        *checkJSON      `json:"check,omitempty"` // set on the response to a check
}

type ticketJSON struct {
	Title string `json:"title"`
	From  string `json:"from"`
	Body  string `json:"body"`
}

type hintJSON struct {
	Tier int    `json:"tier"`
	Text string `json:"text"`
}

type checkRunJSON struct {
	At       time.Time `json:"at"`
	Passed   bool      `json:"passed"`
	Feedback string    `json:"feedback"` // the symptom text, "" if none
}

// eventJSON is one line of the session's activity log. It carries the event
// type and time, plus the hint tier for hint_revealed; never the payload of
// session_started, which holds the seed and variant.
type eventJSON struct {
	Type string    `json:"type"`
	At   time.Time `json:"at"`
	Tier int       `json:"tier,omitempty"`
}

type levelChangeRow struct {
	Topic string `json:"topic"`
	From  int    `json:"from"`
	To    int    `json:"to"`
}

// sessionView builds the learner-visible view of sess.
func (s *Server) sessionView(ctx context.Context, lang string, sess store.Session) (sessionJSON, error) {
	texts, vars, err := play.Texts(s.cfg.ExercisesDir, sess, lang)
	if err != nil {
		return sessionJSON{}, err
	}
	events, err := s.cfg.Store.Events(ctx, sess.ID)
	if err != nil {
		return sessionJSON{}, err
	}
	ex, err := exercise.Load(filepath.Join(s.cfg.ExercisesDir, sess.ExerciseID))
	if err != nil {
		return sessionJSON{}, err
	}

	v := sessionJSON{
		ID: sess.ID, Exercise: sess.ExerciseID, Topic: ex.Topic, Difficulty: ex.Difficulty,
		Status: sess.Status, Stars: sess.Stars,
		StartedAt: sess.StartedAt, EndedAt: sess.EndedAt, ExpiresAt: sess.ExpiresAt,
		Ticket: ticketJSON{
			Title: i18n.Fill(texts.Ticket.Title, vars),
			From:  i18n.Fill(texts.Ticket.From, vars),
			Body:  i18n.Fill(texts.Ticket.Body, vars),
		},
		Hints:        []hintJSON{},
		HintsTotal:   len(texts.Hints),
		CanGiveUp:    sess.Status == "active" && sess.HintsRevealed >= 3,
		Checks:       []checkRunJSON{},
		Events:       []eventJSON{},
		ShellCommand: ShellCommand,
		Terminal:     s.cfg.Engine.HasTerminal(),
	}
	for tier := 1; tier <= sess.HintsRevealed && tier <= len(texts.Hints); tier++ {
		v.Hints = append(v.Hints, hintJSON{Tier: tier, Text: i18n.Fill(texts.Hints[tier-1], vars)})
	}
	if sess.Status == "solved" || sess.Status == "gave_up" {
		v.Walkthrough = i18n.Fill(texts.Walkthrough, vars)
	}
	for _, e := range events {
		ev := eventJSON{Type: e.Type, At: e.OccurredAt}
		switch e.Type {
		case "hint_revealed":
			ev.Tier = intField(e.Payload, "tier")
		case "check_run":
			passed, _ := e.Payload["passed"].(bool)
			key, _ := e.Payload["feedback"].(string)
			fb := ""
			if !passed {
				fb = play.FeedbackText(texts, key, vars)
			}
			v.Checks = append(v.Checks, checkRunJSON{At: e.OccurredAt, Passed: passed, Feedback: fb})
		case "level_changed":
			topic, _ := e.Payload["topic"].(string)
			v.LevelChange = &levelChangeRow{Topic: topic, From: intField(e.Payload, "from"), To: intField(e.Payload, "to")}
		}
		v.Events = append(v.Events, ev)
	}
	return v, nil
}

// intField reads a JSON number from an event payload.
func intField(payload map[string]any, key string) int {
	if f, ok := payload[key].(float64); ok {
		return int(f)
	}
	return 0
}

// writeSession writes the view of sess, with the check result if one is
// given.
func (s *Server) writeSession(w http.ResponseWriter, ctx context.Context, lang string, sess store.Session, check *checkJSON) {
	v, err := s.sessionView(ctx, lang, sess)
	if err != nil {
		s.failErr(w, lang, err)
		return
	}
	v.Check = check
	writeJSON(w, http.StatusOK, v)
}

// writeSessionByID reads the session again, after an action changed it, and
// writes its view.
func (s *Server) writeSessionByID(w http.ResponseWriter, ctx context.Context, lang, id string, check *checkJSON) {
	sess, err := s.cfg.Store.SessionByID(ctx, id)
	if err != nil {
		s.failErr(w, lang, fmt.Errorf("reload session: %w", err))
		return
	}
	s.writeSession(w, ctx, lang, sess, check)
}
