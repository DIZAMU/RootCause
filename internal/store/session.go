package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// ErrActiveSession means the learner already has an active session, so a new
// one cannot start (migration 0002).
var ErrActiveSession = errors.New("learner already has an active session")

// Session is one attempt at one variant (spec section 3).
type Session struct {
	ID              string
	LearnerID       int
	ExerciseID      string
	ExerciseVersion int
	Seed            int64
	VariantValues   map[string]string
	DerivedValues   map[string]any
	Status          string
	SandboxID       string
	HintsRevealed   int
	Stars           *int
	StartedAt       time.Time
	EndedAt         *time.Time
	ExpiresAt       time.Time
}

// NewSession is the data needed to create a session.
type NewSession struct {
	ID              string
	LearnerID       int
	ExerciseID      string
	ExerciseVersion int
	Difficulty      int // the exercise's difficulty now; kept for the level rule
	Seed            int64
	VariantValues   map[string]string
	TTL             time.Duration
}

// CreateSession inserts a new active session. It returns ErrActiveSession if
// the learner already has one.
func (s *Store) CreateSession(ctx context.Context, n NewSession) (Session, error) {
	variantJSON, err := json.Marshal(n.VariantValues)
	if err != nil {
		return Session{}, fmt.Errorf("marshal variants: %w", err)
	}
	expires := time.Now().Add(n.TTL)
	_, err = s.pool.Exec(ctx, `
		INSERT INTO session
		    (id, learner_id, exercise_id, exercise_version, exercise_difficulty,
		     seed, variant_values, status, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, 'active', $8)`,
		n.ID, n.LearnerID, n.ExerciseID, n.ExerciseVersion, n.Difficulty,
		n.Seed, variantJSON, expires)
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "one_active_session_per_learner" {
		return Session{}, ErrActiveSession
	}
	if err != nil {
		return Session{}, fmt.Errorf("insert session: %w", err)
	}
	return s.SessionByID(ctx, n.ID)
}

const sessionColumns = `id, learner_id, exercise_id, exercise_version, seed,
	variant_values, derived_values, status, COALESCE(sandbox_id, ''),
	hints_revealed, stars, started_at, ended_at, expires_at`

func scanSession(row pgx.Row) (Session, error) {
	var s Session
	var variantJSON, derivedJSON []byte
	if err := row.Scan(&s.ID, &s.LearnerID, &s.ExerciseID, &s.ExerciseVersion, &s.Seed,
		&variantJSON, &derivedJSON, &s.Status, &s.SandboxID,
		&s.HintsRevealed, &s.Stars, &s.StartedAt, &s.EndedAt, &s.ExpiresAt); err != nil {
		return Session{}, err
	}
	_ = json.Unmarshal(variantJSON, &s.VariantValues)
	_ = json.Unmarshal(derivedJSON, &s.DerivedValues)
	return s, nil
}

// SessionByID returns one session.
func (s *Store) SessionByID(ctx context.Context, id string) (Session, error) {
	row := s.pool.QueryRow(ctx, `SELECT `+sessionColumns+` FROM session WHERE id = $1`, id)
	sess, err := scanSession(row)
	if err != nil {
		return Session{}, fmt.Errorf("get session %s: %w", id, err)
	}
	return sess, nil
}

// DueSessions returns active sessions whose TTL has passed (spec section 8.4
// step 8), oldest expiry first.
func (s *Store) DueSessions(ctx context.Context) ([]Session, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+sessionColumns+`
		FROM session WHERE status = 'active' AND expires_at < now()
		ORDER BY expires_at`)
	if err != nil {
		return nil, fmt.Errorf("due sessions: %w", err)
	}
	defer rows.Close()
	var out []Session
	for rows.Next() {
		sess, err := scanSession(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, sess)
	}
	return out, rows.Err()
}

// ActiveSessionIDs returns the ids of every active session, for any learner.
func (s *Store) ActiveSessionIDs(ctx context.Context) (map[string]bool, error) {
	rows, err := s.pool.Query(ctx, `SELECT id FROM session WHERE status = 'active'`)
	if err != nil {
		return nil, fmt.Errorf("active session ids: %w", err)
	}
	defer rows.Close()
	ids := make(map[string]bool)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids[id] = true
	}
	return ids, rows.Err()
}

// ActiveSession returns the learner's single active session, or ErrNoRows.
func (s *Store) ActiveSession(ctx context.Context, learnerID int) (Session, error) {
	row := s.pool.QueryRow(ctx, `SELECT `+sessionColumns+`
		FROM session WHERE learner_id = $1 AND status = 'active'
		ORDER BY started_at DESC LIMIT 1`, learnerID)
	return scanSession(row)
}

// IsNoRows reports whether err is "no rows", so callers need not import pgx.
func IsNoRows(err error) bool { return errors.Is(err, pgx.ErrNoRows) }

// SetSandbox records the sandbox id for a session.
func (s *Store) SetSandbox(ctx context.Context, id, sandboxID string) error {
	_, err := s.pool.Exec(ctx, `UPDATE session SET sandbox_id = $1 WHERE id = $2`, sandboxID, id)
	return err
}

// SetDerived stores the derived values printed by break.sh.
func (s *Store) SetDerived(ctx context.Context, id string, derived map[string]any) error {
	j, err := json.Marshal(derived)
	if err != nil {
		return err
	}
	_, err = s.pool.Exec(ctx, `UPDATE session SET derived_values = $1 WHERE id = $2`, j, id)
	return err
}

// IncHints increments and returns the revealed-hint count.
func (s *Store) IncHints(ctx context.Context, id string) (int, error) {
	var n int
	err := s.pool.QueryRow(ctx,
		`UPDATE session SET hints_revealed = hints_revealed + 1 WHERE id = $1 RETURNING hints_revealed`,
		id).Scan(&n)
	return n, err
}

// Outcome is one finished session's contribution to a topic level.
type Outcome struct {
	Status     string
	Stars      *int
	HadCheck   bool
	Difficulty int // the exercise's difficulty, 1 to 3
}

// TopicOutcomes returns the learner's finished sessions in a topic, oldest
// first, with whether each ran at least one check (needed to score expiries)
// and the difficulty the exercise had when the session started (needed for
// the level rule; a later recalibration does not change it).
func (s *Store) TopicOutcomes(ctx context.Context, learnerID int, topic string) ([]Outcome, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT s.status, s.stars,
		       EXISTS (SELECT 1 FROM event e WHERE e.session_id = s.id AND e.type = 'check_run'),
		       s.exercise_difficulty
		FROM session s
		JOIN exercise x ON x.id = s.exercise_id
		WHERE s.learner_id = $1 AND x.topic = $2
		  AND s.status IN ('solved', 'gave_up', 'expired', 'voided')
		ORDER BY s.ended_at NULLS LAST, s.started_at`, learnerID, topic)
	if err != nil {
		return nil, fmt.Errorf("topic outcomes: %w", err)
	}
	defer rows.Close()
	var out []Outcome
	for rows.Next() {
		var o Outcome
		if err := rows.Scan(&o.Status, &o.Stars, &o.HadCheck, &o.Difficulty); err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

// BestStars returns the highest stars achieved per exercise (solved sessions).
func (s *Store) BestStars(ctx context.Context) (map[string]int, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT exercise_id, MAX(stars) FROM session WHERE stars IS NOT NULL GROUP BY exercise_id`)
	if err != nil {
		return nil, fmt.Errorf("best stars: %w", err)
	}
	defer rows.Close()
	best := make(map[string]int)
	for rows.Next() {
		var id string
		var stars int
		if err := rows.Scan(&id, &stars); err != nil {
			return nil, err
		}
		best[id] = stars
	}
	return best, rows.Err()
}

// EndSession marks a session finished with a terminal status and optional stars.
func (s *Store) EndSession(ctx context.Context, id, status string, stars *int) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE session SET status = $1, stars = $2, ended_at = now() WHERE id = $3`,
		status, stars, id)
	return err
}
