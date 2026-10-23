package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
)

// Readers for the dbt models in schema analytics (spec section 9.2), and the
// source-table readers the selector falls back to when those models are
// missing or stale (spec section 9.3).

// missingRelation reports whether err means a schema or table does not exist,
// which is how a database where dbt never ran looks.
func missingRelation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && (pgErr.Code == "42P01" || pgErr.Code == "3F000")
}

// MartsFresh reports whether the marts hold the learner's latest finished
// session: the newest ended_at in analytics.int_session_outcomes equals the
// newest in the session table. It is false, with no error, if dbt never ran.
func (s *Store) MartsFresh(ctx context.Context, learnerID int) (bool, error) {
	var fresh bool
	err := s.pool.QueryRow(ctx, `
		SELECT (SELECT max(ended_at) FROM analytics.int_session_outcomes WHERE learner_id = $1)
		       IS NOT DISTINCT FROM
		       (SELECT max(ended_at) FROM session WHERE learner_id = $1 AND status <> 'active')`,
		learnerID).Scan(&fresh)
	if missingRelation(err) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("marts freshness: %w", err)
	}
	return fresh, nil
}

// TopicLevel is one topic's row in analytics.mart_topic_levels.
type TopicLevel struct {
	Level         int
	LastPracticed *time.Time
}

// MartTopicLevels returns the learner's level and last practice per topic.
func (s *Store) MartTopicLevels(ctx context.Context, learnerID int) (map[string]TopicLevel, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT topic, level, last_practiced_at
		FROM analytics.mart_topic_levels WHERE learner_id = $1`, learnerID)
	if err != nil {
		return nil, fmt.Errorf("mart_topic_levels: %w", err)
	}
	defer rows.Close()
	out := make(map[string]TopicLevel)
	for rows.Next() {
		var topic string
		var tl TopicLevel
		if err := rows.Scan(&topic, &tl.Level, &tl.LastPracticed); err != nil {
			return nil, err
		}
		out[topic] = tl
	}
	return out, rows.Err()
}

// ReviewItem is one row of analytics.mart_review_queue.
type ReviewItem struct {
	ExerciseID  string
	DueAt       time.Time
	LastSeed    int64
	LastVariant map[string]string
}

// MartReviewQueue returns the learner's review candidates, due or not,
// oldest due first. It returns ErrNoMarts if dbt never ran.
func (s *Store) MartReviewQueue(ctx context.Context, learnerID int) ([]ReviewItem, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT exercise_id, due_at, last_seed, last_variant_values
		FROM analytics.mart_review_queue WHERE learner_id = $1
		ORDER BY due_at, exercise_id`, learnerID)
	if missingRelation(err) {
		return nil, ErrNoMarts
	}
	if err != nil {
		return nil, fmt.Errorf("mart_review_queue: %w", err)
	}
	defer rows.Close()
	var out []ReviewItem
	for rows.Next() {
		var r ReviewItem
		var variant []byte
		if err := rows.Scan(&r.ExerciseID, &r.DueAt, &r.LastSeed, &variant); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(variant, &r.LastVariant); err != nil {
			return nil, fmt.Errorf("review variant for %s: %w", r.ExerciseID, err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// ExerciseStat is one row of analytics.mart_exercise_stats.
type ExerciseStat struct {
	ExerciseID      string
	Topic           string
	Difficulty      int
	Sessions        int
	Solves          int
	SolveRate       *float64
	AvgHints        *float64
	AvgSolveSeconds *float64
	CalibrationFlag *string
}

// MartExerciseStats returns the per-exercise statistics, ordered by id. It
// returns ErrNoMarts if dbt never ran.
func (s *Store) MartExerciseStats(ctx context.Context) ([]ExerciseStat, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT exercise_id, topic, difficulty, sessions, solves,
		       solve_rate::float8, avg_hints::float8, avg_solve_seconds::float8, calibration_flag
		FROM analytics.mart_exercise_stats ORDER BY exercise_id`)
	if missingRelation(err) {
		return nil, ErrNoMarts
	}
	if err != nil {
		return nil, fmt.Errorf("mart_exercise_stats: %w", err)
	}
	defer rows.Close()
	var out []ExerciseStat
	for rows.Next() {
		var r ExerciseStat
		if err := rows.Scan(&r.ExerciseID, &r.Topic, &r.Difficulty, &r.Sessions, &r.Solves,
			&r.SolveRate, &r.AvgHints, &r.AvgSolveSeconds, &r.CalibrationFlag); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// ErrNoMarts means the dbt models have never been built in this database.
var ErrNoMarts = errors.New("the dbt models have not been built yet")

// LastPracticed returns, per topic, when the learner last finished a session
// that was not voided. It is the source-table fallback for the same column
// in mart_topic_levels.
func (s *Store) LastPracticed(ctx context.Context, learnerID int) (map[string]time.Time, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT x.topic, max(s.ended_at)
		FROM session s JOIN exercise x ON x.id = s.exercise_id
		WHERE s.learner_id = $1 AND s.status NOT IN ('active', 'voided') AND s.ended_at IS NOT NULL
		GROUP BY x.topic`, learnerID)
	if err != nil {
		return nil, fmt.Errorf("last practiced: %w", err)
	}
	defer rows.Close()
	out := make(map[string]time.Time)
	for rows.Next() {
		var topic string
		var at time.Time
		if err := rows.Scan(&topic, &at); err != nil {
			return nil, err
		}
		out[topic] = at
	}
	return out, rows.Err()
}

// AttemptHistory returns, per exercise, the start of the learner's latest
// session that was not voided, and the exercise of the learner's most recent
// session of any status ("" if none).
func (s *Store) AttemptHistory(ctx context.Context, learnerID int) (map[string]time.Time, string, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT exercise_id, max(started_at)
		FROM session WHERE learner_id = $1 AND status <> 'voided'
		GROUP BY exercise_id`, learnerID)
	if err != nil {
		return nil, "", fmt.Errorf("attempt history: %w", err)
	}
	defer rows.Close()
	last := make(map[string]time.Time)
	for rows.Next() {
		var id string
		var at time.Time
		if err := rows.Scan(&id, &at); err != nil {
			return nil, "", err
		}
		last[id] = at
	}
	if err := rows.Err(); err != nil {
		return nil, "", err
	}
	var previous string
	err = s.pool.QueryRow(ctx, `
		SELECT exercise_id FROM session WHERE learner_id = $1
		ORDER BY started_at DESC, id DESC LIMIT 1`, learnerID).Scan(&previous)
	if err != nil && !IsNoRows(err) {
		return nil, "", fmt.Errorf("previous exercise: %w", err)
	}
	return last, previous, nil
}

// statsLockKey names the Postgres advisory lock held while dbt rebuilds the
// marts, so the CLI and the dashboard server never run two builds at once.
const statsLockKey int64 = 7300_0003

// WithStatsLock runs build while holding the stats advisory lock. A second
// caller, in this process or another, waits until the first build is done.
// If ctx ends while it waits, it returns ctx's error and build does not run.
func (s *Store) WithStatsLock(ctx context.Context, build func() error) error {
	// An advisory lock belongs to one database connection, so the lock and
	// the unlock run on the same connection taken from the pool.
	conn, err := s.pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("acquire connection: %w", err)
	}
	defer conn.Release()
	if _, err := conn.Exec(ctx, `SELECT pg_advisory_lock($1)`, statsLockKey); err != nil {
		return fmt.Errorf("lock stats: %w", err)
	}
	defer func() {
		// If the unlock fails, close the connection: that releases the lock,
		// and the pool will not hand out a connection that still holds it.
		if _, err := conn.Exec(context.Background(), `SELECT pg_advisory_unlock($1)`, statsLockKey); err != nil {
			_ = conn.Conn().Close(context.Background())
		}
	}()
	return build()
}
