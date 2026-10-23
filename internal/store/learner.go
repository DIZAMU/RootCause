package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// Learner is the local profile. v1 has exactly one.
type Learner struct {
	ID   int
	Name string
	Lang string
}

// learnerLockKey names the Postgres advisory lock that serializes creating the
// profile, so two first-time commands cannot each create a learner.
const learnerLockKey int64 = 7300_0002

// EnsureLearner returns the single learner, creating it with the given name and
// default language if none exists yet. The check and the create run in one
// transaction under an advisory lock, which Postgres releases at commit.
func (s *Store) EnsureLearner(ctx context.Context, name string) (Learner, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Learner{}, fmt.Errorf("begin: %w", err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock($1)`, learnerLockKey); err != nil {
		return Learner{}, fmt.Errorf("lock learner: %w", err)
	}

	var l Learner
	err = tx.QueryRow(ctx,
		`SELECT id, name, lang FROM learner ORDER BY id LIMIT 1`).
		Scan(&l.ID, &l.Name, &l.Lang)
	if errors.Is(err, pgx.ErrNoRows) {
		err = tx.QueryRow(ctx,
			`INSERT INTO learner (name) VALUES ($1) RETURNING id, name, lang`, name).
			Scan(&l.ID, &l.Name, &l.Lang)
		if err != nil {
			return Learner{}, fmt.Errorf("create learner: %w", err)
		}
	} else if err != nil {
		return Learner{}, fmt.Errorf("read learner: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return Learner{}, fmt.Errorf("commit: %w", err)
	}
	return l, nil
}

// SetLang updates the learner's interface language ("en" or "no").
func (s *Store) SetLang(ctx context.Context, learnerID int, lang string) error {
	if lang != "en" && lang != "no" {
		return fmt.Errorf("lang must be en or no, got %q", lang)
	}
	_, err := s.pool.Exec(ctx, `UPDATE learner SET lang = $1 WHERE id = $2`, lang, learnerID)
	if err != nil {
		return fmt.Errorf("set lang: %w", err)
	}
	return nil
}

// LearnerByID returns one learner. The dashboard server reads the learner on
// every request, so a language set from the CLI shows up without a restart.
func (s *Store) LearnerByID(ctx context.Context, id int) (Learner, error) {
	var l Learner
	err := s.pool.QueryRow(ctx, `SELECT id, name, lang FROM learner WHERE id = $1`, id).
		Scan(&l.ID, &l.Name, &l.Lang)
	if err != nil {
		return Learner{}, fmt.Errorf("read learner %d: %w", id, err)
	}
	return l, nil
}
