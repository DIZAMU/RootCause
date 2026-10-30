// Package play holds the steps the two front ends share, the CLI and the
// dashboard server: choosing the next exercise and its seed, filling an
// exercise's texts for a session, and rebuilding the statistics. Both call
// these functions, so the CLI and the dashboard cannot drift apart.
package play

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/DIZAMU/RootCause/internal/dbt"
	"github.com/DIZAMU/RootCause/internal/exercise"
	"github.com/DIZAMU/RootCause/internal/i18n"
	"github.com/DIZAMU/RootCause/internal/selector"
	"github.com/DIZAMU/RootCause/internal/store"
)

// Topics is the fixed v1 topic set (ARCHITECTURE.md section 4.2), in display order.
var Topics = []string{"permissions", "services", "logs", "networking", "disk"}

// Texts loads the exercise texts of sess in lang, and the values that fill
// their placeholders.
func Texts(exercisesDir string, sess store.Session, lang string) (*exercise.I18n, map[string]string, error) {
	ex, err := exercise.Load(filepath.Join(exercisesDir, sess.ExerciseID))
	if err != nil {
		return nil, nil, err
	}
	texts, err := ex.LoadI18n(lang)
	if err != nil {
		return nil, nil, err
	}
	return texts, TextVars(sess), nil
}

// Ticket is an exercise's ticket header outside a session, for the inbox and
// the catalog.
type Ticket struct {
	Title string
	From  string
}

// CatalogTicket returns the ticket title and sender of an exercise in lang,
// before any session exists. Placeholders are filled from the first value of
// each variant, a fixed display choice that does not depend on any seed. No
// derived value exists yet, and the ticket header never uses one.
func CatalogTicket(exercisesDir, id, lang string) (Ticket, error) {
	ex, err := exercise.Load(filepath.Join(exercisesDir, id))
	if err != nil {
		return Ticket{}, err
	}
	texts, err := ex.LoadI18n(lang)
	if err != nil {
		return Ticket{}, err
	}
	vars := make(map[string]string, len(ex.Variants))
	for name, values := range ex.Variants {
		if len(values) > 0 {
			vars[name] = values[0]
		}
	}
	return Ticket{Title: i18n.Fill(texts.Ticket.Title, vars), From: i18n.Fill(texts.Ticket.From, vars)}, nil
}

// TextVars collects variant and derived values for placeholder filling.
func TextVars(sess store.Session) map[string]string {
	m := make(map[string]string)
	for k, v := range sess.VariantValues {
		m[k] = v
	}
	for k, v := range sess.DerivedValues {
		m[k] = fmt.Sprint(v)
	}
	return m
}

// FeedbackText returns the exercise's symptom text for a check's feedback
// key, with placeholders filled, or "" if the exercise has no text for that
// key.
func FeedbackText(texts *exercise.I18n, key string, vars map[string]string) string {
	return strings.TrimSpace(i18n.Fill(texts.Feedback[key], vars))
}

// Recommendation is the next exercise for a learner, and how it was chosen.
type Recommendation struct {
	Pick       selector.Pick
	Level      int // the level of the picked exercise's topic, 1 to 3
	Source     selector.Source
	RefreshErr error // set if the marts were stale and rebuilding them failed
}

// Recommend picks the learner's next exercise from catalog (ARCHITECTURE.md section
// 7.2), through selector.Load and selector.Next. deps.Refresh decides whether
// stale marts are rebuilt first; nil only previews the pick. It returns
// selector.ErrNoCandidate when there is nothing to pick.
func Recommend(ctx context.Context, deps selector.Deps, learnerID int, catalog []store.ExerciseRow, now time.Time) (Recommendation, error) {
	exs := make([]selector.Exercise, len(catalog))
	for i, r := range catalog {
		exs[i] = selector.Exercise{ID: r.ID, Topic: r.Topic, Difficulty: r.Difficulty}
	}
	loaded, err := selector.Load(ctx, deps, learnerID, exs)
	if err != nil {
		return Recommendation{}, err
	}
	rec := Recommendation{Source: loaded.Source, RefreshErr: loaded.RefreshErr}
	if rec.Pick, err = selector.Next(loaded.Input, now); err != nil {
		return rec, err
	}
	rec.Level = loaded.Input.Topics[rec.Pick.Exercise.Topic].Level
	if rec.Level == 0 {
		rec.Level = 1
	}
	return rec, nil
}

// Seed returns the seed to start pick with: for a review pick, one that gives
// a new variant (selector.ReviewSeed); otherwise a random one.
func Seed(exercisesDir string, pick selector.Pick, random func() int64) (int64, error) {
	if pick.Reason != selector.ReasonReview {
		return random(), nil
	}
	ex, err := exercise.Load(filepath.Join(exercisesDir, pick.Exercise.ID))
	if err != nil {
		return 0, err
	}
	return selector.ReviewSeed(ex.SelectVariant, *pick.Due, random), nil
}

// BuildStats rebuilds the marts with dbt (ARCHITECTURE.md section 9.3) while holding the
// store's stats lock, so the CLI and the dashboard never build at once.
func BuildStats(ctx context.Context, st *store.Store, dsn string) error {
	return st.WithStatsLock(ctx, func() error { return dbt.Build(ctx, dsn) })
}

// ErrNoExercises means the exercises folder cannot be read.
var ErrNoExercises = errors.New("cannot read the exercises folder")

// Catalog loads every exercise in exercisesDir and returns its catalog rows
// (ARCHITECTURE.md section 9.1). Any exercise that fails to load is an error.
func Catalog(exercisesDir string) ([]store.ExerciseRow, error) {
	entries, err := os.ReadDir(exercisesDir)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrNoExercises, err)
	}
	var rows []store.ExerciseRow
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		ex, err := exercise.Load(filepath.Join(exercisesDir, e.Name()))
		if err != nil {
			return nil, err
		}
		rows = append(rows, store.ExerciseRow{
			ID: ex.ID, Version: ex.Version, Topic: ex.Topic, Difficulty: ex.Difficulty,
		})
	}
	return rows, nil
}
