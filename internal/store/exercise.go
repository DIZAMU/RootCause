package store

import (
	"context"
	"fmt"
)

// ExerciseRow is the catalog data synced from an exercise.yaml (spec section
// 9.1). The store takes plain rows so it does not depend on the exercise loader.
type ExerciseRow struct {
	ID         string
	Version    int
	Topic      string
	Difficulty int
}

// SyncExercises upserts the catalog: new exercises are inserted, existing ones
// have their version, topic and difficulty refreshed. It is called on startup.
func (s *Store) SyncExercises(ctx context.Context, rows []ExerciseRow) error {
	batchSQL := `
		INSERT INTO exercise (id, version, topic, difficulty, synced_at)
		VALUES ($1, $2, $3, $4, now())
		ON CONFLICT (id) DO UPDATE
		SET version = EXCLUDED.version,
		    topic = EXCLUDED.topic,
		    difficulty = EXCLUDED.difficulty,
		    synced_at = now()`
	for _, r := range rows {
		if _, err := s.pool.Exec(ctx, batchSQL, r.ID, r.Version, r.Topic, r.Difficulty); err != nil {
			return fmt.Errorf("sync exercise %s: %w", r.ID, err)
		}
	}
	return nil
}

// Exercises returns the synced catalog, ordered by id.
func (s *Store) Exercises(ctx context.Context) ([]ExerciseRow, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, version, topic, difficulty FROM exercise ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("list exercises: %w", err)
	}
	defer rows.Close()

	var out []ExerciseRow
	for rows.Next() {
		var r ExerciseRow
		if err := rows.Scan(&r.ID, &r.Version, &r.Topic, &r.Difficulty); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
