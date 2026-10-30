package store

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"
)

// fixture creates a learner and an exercise (topic "disk") used only by the
// calling test, so it neither depends on nor disturbs data other tests leave
// in the test database. Cleanup removes both, with their sessions and events.
func fixture(t *testing.T, s *Store, difficulty int) (learnerID int, exerciseID string) {
	t.Helper()
	ctx := context.Background()
	if err := s.Migrate(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	// Make sure the profile learner exists first. It then keeps the lowest id,
	// so EnsureLearner never returns a test learner.
	if _, err := s.EnsureLearner(ctx, "DIZAMU"); err != nil {
		t.Fatalf("ensure profile learner: %v", err)
	}
	exerciseID = fmt.Sprintf("store-test-%d", time.Now().UnixNano())
	if err := s.SyncExercises(ctx, []ExerciseRow{
		{ID: exerciseID, Version: 1, Topic: "disk", Difficulty: difficulty},
	}); err != nil {
		t.Fatalf("sync exercise: %v", err)
	}
	if err := s.pool.QueryRow(ctx,
		`INSERT INTO learner (name) VALUES ($1) RETURNING id`, exerciseID).Scan(&learnerID); err != nil {
		t.Fatalf("create learner: %v", err)
	}
	t.Cleanup(func() {
		ctx := context.Background()
		for _, step := range []struct {
			sql string
			arg any
		}{
			{`DELETE FROM event WHERE session_id IN (SELECT id FROM session WHERE learner_id = $1)`, learnerID},
			{`DELETE FROM session WHERE learner_id = $1`, learnerID},
			{`DELETE FROM learner WHERE id = $1`, learnerID},
			{`DELETE FROM exercise WHERE id = $1`, exerciseID},
		} {
			if _, err := s.pool.Exec(ctx, step.sql, step.arg); err != nil {
				t.Errorf("cleanup %q: %v", step.sql, err)
			}
		}
	})
	return learnerID, exerciseID
}

func newSession(learnerID int, exerciseID, id string, difficulty int) NewSession {
	return NewSession{
		ID: id, LearnerID: learnerID, ExerciseID: exerciseID,
		ExerciseVersion: 1, Difficulty: difficulty, Seed: 1, TTL: time.Hour,
	}
}

func TestOneActiveSession(t *testing.T) {
	s := openOrSkip(t)
	ctx := context.Background()
	lid, xid := fixture(t, s, 1)

	if _, err := s.CreateSession(ctx, newSession(lid, xid, xid+"-a", 1)); err != nil {
		t.Fatalf("first session: %v", err)
	}
	if _, err := s.CreateSession(ctx, newSession(lid, xid, xid+"-b", 1)); !errors.Is(err, ErrActiveSession) {
		t.Fatalf("second active session: err = %v, want ErrActiveSession", err)
	}

	// Once the first one has ended, a new one may start.
	three := 3
	if err := s.EndSession(ctx, xid+"-a", "solved", &three); err != nil {
		t.Fatalf("end first session: %v", err)
	}
	if _, err := s.CreateSession(ctx, newSession(lid, xid, xid+"-c", 1)); err != nil {
		t.Errorf("session after the first ended: %v", err)
	}
}

func TestDifficultyFrozenAtStart(t *testing.T) {
	s := openOrSkip(t)
	ctx := context.Background()
	lid, xid := fixture(t, s, 1)

	if _, err := s.CreateSession(ctx, newSession(lid, xid, xid+"-a", 1)); err != nil {
		t.Fatalf("create session: %v", err)
	}
	three := 3
	if err := s.EndSession(ctx, xid+"-a", "solved", &three); err != nil {
		t.Fatalf("end session: %v", err)
	}

	// Recalibrate the exercise, as calibration may do.
	if err := s.SyncExercises(ctx, []ExerciseRow{
		{ID: xid, Version: 1, Topic: "disk", Difficulty: 2},
	}); err != nil {
		t.Fatalf("re-sync: %v", err)
	}

	outs, err := s.TopicOutcomes(ctx, lid, "disk")
	if err != nil {
		t.Fatalf("topic outcomes: %v", err)
	}
	if len(outs) != 1 {
		t.Fatalf("got %d outcomes, want 1", len(outs))
	}
	if outs[0].Difficulty != 1 {
		t.Errorf("difficulty after recalibration = %d, want 1 (the value at start)", outs[0].Difficulty)
	}
}
