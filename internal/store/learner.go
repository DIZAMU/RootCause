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

// EnsureLearner returns the single learner, creating it with the given name and
// default language if none exists yet.
func (s *Store) EnsureLearner(ctx context.Context, name string) (Learner, error) {
	var l Learner
	err := s.pool.QueryRow(ctx,
		`SELECT id, name, lang FROM learner ORDER BY id LIMIT 1`).
		Scan(&l.ID, &l.Name, &l.Lang)
	if err == nil {
		return l, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return Learner{}, fmt.Errorf("read learner: %w", err)
	}
	err = s.pool.QueryRow(ctx,
		`INSERT INTO learner (name) VALUES ($1) RETURNING id, name, lang`, name).
		Scan(&l.ID, &l.Name, &l.Lang)
	if err != nil {
		return Learner{}, fmt.Errorf("create learner: %w", err)
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
