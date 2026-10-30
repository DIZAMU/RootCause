package cli

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"math/rand"
	"path/filepath"
	"strings"
	"time"

	"github.com/DIZAMU/RootCause/internal/exercise"
	"github.com/DIZAMU/RootCause/internal/i18n"
	"github.com/DIZAMU/RootCause/internal/play"
	"github.com/DIZAMU/RootCause/internal/session"
	"github.com/DIZAMU/RootCause/internal/store"
)

// Start begins a specific exercise (ARCHITECTURE.md section 10: rootcause start <id>).
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
	if !fileExists(filepath.Join(a.root, "exercises", args[0], "exercise.yaml")) {
		return failWith("start.unknown_exercise", map[string]string{"id": args[0]}, nil)
	}
	return a.startExercise(ctx, args[0], rand.Int63())
}

// startExercise starts exerciseID with seed and shows its ticket.
func (a *App) startExercise(ctx context.Context, exerciseID string, seed int64) error {
	sess, err := a.eng.Start(ctx, a.learner.ID, exerciseID, seed)
	if errors.Is(err, store.ErrActiveSession) {
		// Another start won the race (migration 0002 enforces one active
		// session); report it the same way as the check above.
		if cur, ok, aerr := a.active(ctx); aerr == nil && ok {
			a.say("start.active_exists", map[string]string{"exercise": cur.ExerciseID})
			return nil
		}
	}
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
	text := i18n.Fill(texts.Hints[tier-1], play.TextVars(sess))
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
	vars := play.TextVars(sess)
	switch {
	case res.Infra:
		a.say("check.infra", nil)
	case res.Passed:
		a.say("check.pass", nil)
		a.printWalkthrough(texts, vars)
		if res.Level != nil {
			a.announceLevel(res.Level)
		}
		a.refreshAfterSession(ctx)
	default:
		if fb := play.FeedbackText(texts, res.Feedback, vars); fb != "" {
			a.say("check.fail", map[string]string{"feedback": fb})
		} else {
			a.say("check.fail_generic", nil)
		}
	}
	return nil
}

// Reset gives a fresh copy of the same variant. With --all it deletes all of
// the learner's data instead (see WipeAll).
func (a *App) Reset(ctx context.Context, args []string) error {
	all, yes := false, false
	for _, arg := range args {
		switch arg {
		case "--all":
			all = true
		case "--yes":
			yes = true
		default:
			a.say("reset.usage", nil)
			return nil
		}
	}
	if all {
		return a.WipeAll(ctx, yes)
	}
	if yes {
		a.say("reset.usage", nil)
		return nil
	}
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

// WipeAll deletes all of the learner's data, the same way the dashboard's
// "Reset all data" does (session.Engine.WipeAll): the active session and its
// sandbox, the history, the levels, the statistics and the settings. Unless
// yes is set, the learner must type the confirmation word first. The
// statistics are rebuilt afterwards.
func (a *App) WipeAll(ctx context.Context, yes bool) error {
	if !yes {
		word := a.t("wipe.word", nil)
		a.say("wipe.warning", nil)
		fmt.Fprint(a.out, a.t("wipe.prompt", map[string]string{"word": word}))
		line, _ := bufio.NewReader(a.in).ReadString('\n')
		if strings.TrimSpace(line) != word {
			a.say("wipe.cancelled", nil)
			return nil
		}
	}
	n, err := a.eng.WipeAll(ctx, a.learner.ID)
	if err != nil {
		return err
	}
	a.say("wipe.done", map[string]string{"sandboxes": fmt.Sprint(n)})
	a.learner.Lang = "en" // the wipe resets the language to the default
	a.refreshAfterSession(ctx)
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
	a.printWalkthrough(texts, play.TextVars(sess))
	if lc != nil {
		a.announceLevel(lc)
	}
	a.refreshAfterSession(ctx)
	return nil
}

// Status shows the active session, the level per topic, the review queue and
// the best stars per exercise (ARCHITECTURE.md section 10).
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
	if err := a.printReviewQueue(ctx); err != nil {
		return err
	}
	return a.List(ctx)
}

// printReviewQueue shows the exercises waiting for review, from the marts, as
// of their last build.
func (a *App) printReviewQueue(ctx context.Context) error {
	a.say("status.review_header", nil)
	queue, err := a.st.MartReviewQueue(ctx, a.learner.ID)
	if errors.Is(err, store.ErrNoMarts) {
		a.say("status.review_unavailable", nil)
		return nil
	}
	if err != nil {
		return err
	}
	if len(queue) == 0 {
		a.say("status.review_empty", nil)
	}
	now := time.Now()
	for _, r := range queue {
		if !r.DueAt.After(now) {
			a.say("status.review_due_now", map[string]string{"exercise": r.ExerciseID})
			continue
		}
		a.say("status.review_due", map[string]string{
			"exercise": r.ExerciseID, "due": r.DueAt.Local().Format("2006-01-02 15:04"),
		})
	}
	return nil
}

// printLevels shows the current level for each topic (ARCHITECTURE.md section 7.1),
// computed from the finished outcomes.
func (a *App) printLevels(ctx context.Context) error {
	a.say("status.levels_header", nil)
	for _, topic := range play.Topics {
		level, err := a.eng.TopicLevel(ctx, a.learner.ID, topic)
		if err != nil {
			return err
		}
		a.say("status.level_row", map[string]string{"topic": topic, "level": fmt.Sprint(level)})
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
	texts, _, err := play.Texts(a.exercisesDir(), sess, a.lang())
	return texts, err
}

// exercisesDir is the folder holding the exercise folders.
func (a *App) exercisesDir() string { return filepath.Join(a.root, "exercises") }

func (a *App) printTicket(sess store.Session) {
	texts, err := a.exerciseTexts(sess)
	if err != nil {
		fmt.Fprintln(a.out, a.Describe(err))
		return
	}
	vars := play.TextVars(sess)
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

// starGlyphs renders best stars as filled/empty marks, or "---" if never solved.
func starGlyphs(best map[string]int, id string) string {
	n, ok := best[id]
	if !ok {
		return "---"
	}
	return strings.Repeat("*", n) + strings.Repeat(".", 3-n)
}
