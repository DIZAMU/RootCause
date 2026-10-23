package cli

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"time"

	"github.com/DIZAMU/RootCause/internal/play"
	"github.com/DIZAMU/RootCause/internal/selector"
	"github.com/DIZAMU/RootCause/internal/store"
)

// Next starts the recommended next exercise (spec sections 7.2 and 10).
func (a *App) Next(ctx context.Context) error {
	if cur, ok, err := a.active(ctx); err != nil {
		return err
	} else if ok {
		a.say("start.active_exists", map[string]string{"exercise": cur.ExerciseID})
		return nil
	}
	catalog, err := loadAllExercises(a.root)
	if err != nil {
		return err
	}
	rec, err := play.Recommend(ctx, selector.Deps{
		Store: a.st,
		Level: a.eng.TopicLevel,
		Refresh: func(ctx context.Context) error {
			a.say("stats.updating", nil)
			return a.buildStats(ctx)
		},
	}, a.learner.ID, catalog, time.Now())
	if rec.RefreshErr != nil {
		a.warnRefresh(rec.RefreshErr)
	}
	if rec.Source == selector.SourceFallback {
		a.say("next.fallback", nil)
	}
	if errors.Is(err, selector.ErrNoCandidate) {
		a.say("next.none", nil)
		return nil
	}
	if err != nil {
		return err
	}

	pick := rec.Pick
	seed, err := play.Seed(a.exercisesDir(), pick, rand.Int63)
	if err != nil {
		return err
	}
	if pick.Reason == selector.ReasonReview {
		a.say("next.review", map[string]string{"exercise": pick.Exercise.ID})
	} else {
		a.say("next.weakest", map[string]string{
			"exercise": pick.Exercise.ID, "topic": pick.Exercise.Topic, "level": fmt.Sprint(rec.Level),
		})
	}
	return a.startExercise(ctx, pick.Exercise.ID, seed)
}

// Stats shows the per-exercise statistics from the marts (spec section 7.3).
// With --refresh it rebuilds them first (spec section 9.3).
func (a *App) Stats(ctx context.Context, args []string) error {
	switch {
	case len(args) == 0:
	case len(args) == 1 && args[0] == "--refresh":
		a.say("stats.updating", nil)
		if err := a.buildStats(ctx); err != nil {
			return fail("stats.refresh_failed", err)
		}
	default:
		a.say("stats.usage", nil)
		return nil
	}

	stats, err := a.st.MartExerciseStats(ctx)
	if errors.Is(err, store.ErrNoMarts) {
		a.say("stats.none", nil)
		return nil
	}
	if err != nil {
		return err
	}
	if fresh, err := a.st.MartsFresh(ctx, a.learner.ID); err != nil {
		return err
	} else if !fresh {
		a.say("stats.stale", nil)
	}
	a.say("stats.header", nil)
	for _, s := range stats {
		a.say("stats.row", map[string]string{
			"id":         s.ExerciseID,
			"topic":      s.Topic,
			"difficulty": fmt.Sprint(s.Difficulty),
			"sessions":   fmt.Sprint(s.Sessions),
			"solves":     fmt.Sprint(s.Solves),
			"rate":       orDash(s.SolveRate, func(v float64) string { return fmt.Sprintf("%.0f%%", v*100) }),
			"hints":      orDash(s.AvgHints, func(v float64) string { return fmt.Sprintf("%.1f", v) }),
			"time":       orDash(s.AvgSolveSeconds, func(v float64) string { return (time.Duration(v) * time.Second).String() }),
			"flag":       a.flagText(s.CalibrationFlag),
		})
	}
	return nil
}

// refreshAfterSession rebuilds the marts after a finished session (spec
// section 9.3). The session's result is already printed and saved, so a
// failure here is only a warning: the next pick falls back to topic levels.
func (a *App) refreshAfterSession(ctx context.Context) {
	a.say("stats.updating", nil)
	if err := a.buildStats(ctx); err != nil {
		a.warnRefresh(err)
	}
}

// buildStats runs dbt against the app's database, under the stats lock.
func (a *App) buildStats(ctx context.Context) error {
	return play.BuildStats(ctx, a.st, a.dsn)
}

// warnRefresh prints a failed refresh as a warning, with its detail.
func (a *App) warnRefresh(err error) {
	fmt.Fprintln(a.out, Message(a.bundle, a.lang(), fail("stats.refresh_failed", err)))
}

func (a *App) flagText(flag *string) string {
	switch {
	case flag == nil:
		return ""
	case *flag == "too_hard":
		return a.t("stats.flag_too_hard", nil)
	case *flag == "too_easy":
		return a.t("stats.flag_too_easy", nil)
	default:
		return "  (" + *flag + ")"
	}
}

// orDash formats v, or returns "-" when there is no value.
func orDash(v *float64, format func(float64) string) string {
	if v == nil {
		return "-"
	}
	return format(*v)
}
