package cli

import (
	"context"
	"fmt"
	"math/rand"
	"path/filepath"
	"strings"

	"github.com/DIZAMU/RootCause/internal/exercise"
	"github.com/DIZAMU/RootCause/internal/grading"
	"github.com/DIZAMU/RootCause/internal/i18n"
	"github.com/DIZAMU/RootCause/internal/session"
	"github.com/DIZAMU/RootCause/internal/store"
)

// topics is the fixed v1 topic set (spec section 4.2), in display order.
var topics = []string{"permissions", "services", "logs", "networking", "disk"}

// Start begins a specific exercise (spec section 10: rootcause start <id>).
func (a *App) Start(ctx context.Context, args []string) error {
	if len(args) < 1 {
		a.say("start.usage", nil)
		return nil
	}
	if cur, ok, err := a.active(ctx); err != nil {
		return err
	} else if ok {
		a.say("start.active_exists", map[string]string{"exercise": cur.ExerciseID})
		return nil
	}
	seed := rand.Int63()
	sess, err := a.eng.Start(ctx, a.learner.ID, args[0], seed)
	if err != nil {
		return err
	}
	a.say("start.started", map[string]string{"exercise": sess.ExerciseID})
	a.printTicket(sess)
	a.say("shell.hint", nil)
	return nil
}

// Ticket re-shows the active ticket.
func (a *App) Ticket(ctx context.Context) error {
	sess, ok, err := a.active(ctx)
	if err != nil {
		return err
	}
	if !ok {
		a.say("no_active_session", nil)
		return nil
	}
	a.printTicket(sess)
	return nil
}

// Shell opens a shell in the active sandbox.
func (a *App) Shell(ctx context.Context) error {
	sess, ok, err := a.active(ctx)
	if err != nil {
		return err
	}
	if !ok {
		a.say("shell.usage", nil)
		return nil
	}
	return a.eng.Shell(ctx, sess.ID)
}

// Hint reveals the next hint tier.
func (a *App) Hint(ctx context.Context) error {
	sess, ok, err := a.active(ctx)
	if err != nil {
		return err
	}
	if !ok {
		a.say("no_active_session", nil)
		return nil
	}
	texts, err := a.exerciseTexts(sess)
	if err != nil {
		return err
	}
	if sess.HintsRevealed >= len(texts.Hints) {
		a.say("hint.none_more", nil)
		return nil
	}
	tier, err := a.eng.Hint(ctx, sess.ID)
	if err != nil {
		return err
	}
	text := i18n.Fill(texts.Hints[tier-1], textVars(sess))
	a.say("hint.revealed", map[string]string{"tier": fmt.Sprint(tier), "text": text})
	return nil
}

// Check grades the active session.
func (a *App) Check(ctx context.Context) error {
	sess, ok, err := a.active(ctx)
	if err != nil {
		return err
	}
	if !ok {
		a.say("no_active_session", nil)
		return nil
	}
	res, err := a.eng.Check(ctx, sess.ID)
	if err != nil {
		return err
	}
	texts, err := a.exerciseTexts(sess)
	if err != nil {
		return err
	}
	vars := textVars(sess)
	switch {
	case res.Infra:
		a.say("check.infra", nil)
	case res.Passed:
		a.say("check.pass", nil)
		a.printWalkthrough(texts, vars)
		if res.Level != nil {
			a.announceLevel(res.Level)
		}
	default:
		a.say("check.fail", map[string]string{"feedback": i18n.Fill(texts.Feedback[res.Feedback], vars)})
	}
	return nil
}

// Reset gives a fresh copy of the same variant.
func (a *App) Reset(ctx context.Context) error {
	sess, ok, err := a.active(ctx)
	if err != nil {
		return err
	}
	if !ok {
		a.say("no_active_session", nil)
		return nil
	}
	if err := a.eng.Reset(ctx, sess.ID); err != nil {
		return err
	}
	a.say("reset.done", nil)
	return nil
}

// GiveUp ends the session and shows the walkthrough (after hint 3).
func (a *App) GiveUp(ctx context.Context) error {
	sess, ok, err := a.active(ctx)
	if err != nil {
		return err
	}
	if !ok {
		a.say("no_active_session", nil)
		return nil
	}
	if sess.HintsRevealed < 3 {
		a.say("giveup.locked", nil)
		return nil
	}
	lc, err := a.eng.GiveUp(ctx, sess.ID)
	if err != nil {
		return err
	}
	a.say("giveup.done", nil)
	texts, err := a.exerciseTexts(sess)
	if err != nil {
		return err
	}
	a.printWalkthrough(texts, textVars(sess))
	if lc != nil {
		a.announceLevel(lc)
	}
	return nil
}

// Status shows the active session and best stars per exercise. (Topic levels
// and the review queue arrive with grading and dbt in later phases.)
func (a *App) Status(ctx context.Context) error {
	if sess, ok, err := a.active(ctx); err != nil {
		return err
	} else if ok {
		a.say("status.active", map[string]string{"exercise": sess.ExerciseID, "status": sess.Status})
	} else {
		a.say("status.no_active", nil)
	}
	if err := a.printLevels(ctx); err != nil {
		return err
	}
	return a.List(ctx)
}

// printLevels shows the current level for each topic (spec section 7.1),
// computed from the finished outcomes.
func (a *App) printLevels(ctx context.Context) error {
	a.say("status.levels_header", nil)
	for _, topic := range topics {
		outs, err := a.st.TopicOutcomes(ctx, a.learner.ID, topic)
		if err != nil {
			return err
		}
		kinds := make([]grading.Kind, len(outs))
		for i, o := range outs {
			kinds[i] = grading.Classify(o.Status, o.Stars, o.HadCheck)
		}
		a.say("status.level_row", map[string]string{"topic": topic, "level": fmt.Sprint(grading.Level(kinds))})
	}
	return nil
}

// announceLevel reports a topic moving up or down a level.
func (a *App) announceLevel(lc *session.LevelChange) {
	key := "level.up"
	if lc.To < lc.From {
		key = "level.down"
	}
	a.say(key, map[string]string{"topic": lc.Topic, "from": fmt.Sprint(lc.From), "to": fmt.Sprint(lc.To)})
}

// List shows all exercises with best stars.
func (a *App) List(ctx context.Context) error {
	best, err := a.st.BestStars(ctx)
	if err != nil {
		return err
	}
	exs, err := a.st.Exercises(ctx)
	if err != nil {
		return err
	}
	a.say("list.header", nil)
	for _, ex := range exs {
		a.say("list.row", map[string]string{
			"stars":      starGlyphs(best, ex.ID),
			"id":         ex.ID,
			"topic":      ex.Topic,
			"difficulty": fmt.Sprint(ex.Difficulty),
		})
	}
	return nil
}

// Lang sets the interface language.
func (a *App) Lang(ctx context.Context, args []string) error {
	if len(args) < 1 || (args[0] != "en" && args[0] != "no") {
		a.say("lang.usage", nil)
		return nil
	}
	if err := a.st.SetLang(ctx, a.learner.ID, args[0]); err != nil {
		return err
	}
	a.learner.Lang = args[0]
	a.say("lang.set", map[string]string{"lang": args[0]})
	return nil
}

// --- helpers ---------------------------------------------------------------

func (a *App) active(ctx context.Context) (store.Session, bool, error) {
	s, err := a.st.ActiveSession(ctx, a.learner.ID)
	if store.IsNoRows(err) {
		return store.Session{}, false, nil
	}
	if err != nil {
		return store.Session{}, false, err
	}
	return s, true, nil
}

func (a *App) exerciseTexts(sess store.Session) (*exercise.I18n, error) {
	ex, err := exercise.Load(filepath.Join(a.root, "exercises", sess.ExerciseID))
	if err != nil {
		return nil, err
	}
	return ex.LoadI18n(a.lang())
}

func (a *App) printTicket(sess store.Session) {
	texts, err := a.exerciseTexts(sess)
	if err != nil {
		fmt.Fprintln(a.out, err)
		return
	}
	vars := textVars(sess)
	fmt.Fprintln(a.out)
	a.say("ticket.title", map[string]string{"title": i18n.Fill(texts.Ticket.Title, vars)})
	a.say("ticket.from", map[string]string{"from": i18n.Fill(texts.Ticket.From, vars)})
	fmt.Fprintln(a.out, i18n.Fill(texts.Ticket.Body, vars))
}

func (a *App) printWalkthrough(texts *exercise.I18n, vars map[string]string) {
	fmt.Fprintln(a.out)
	a.say("walkthrough.header", nil)
	fmt.Fprintln(a.out, i18n.Fill(texts.Walkthrough, vars))
}

// textVars collects variant and derived values for placeholder filling.
func textVars(sess store.Session) map[string]string {
	m := make(map[string]string)
	for k, v := range sess.VariantValues {
		m[k] = v
	}
	for k, v := range sess.DerivedValues {
		m[k] = fmt.Sprint(v)
	}
	return m
}

// starGlyphs renders best stars as filled/empty marks, or "---" if never solved.
func starGlyphs(best map[string]int, id string) string {
	n, ok := best[id]
	if !ok {
		return "---"
	}
	return strings.Repeat("*", n) + strings.Repeat(".", 3-n)
}
