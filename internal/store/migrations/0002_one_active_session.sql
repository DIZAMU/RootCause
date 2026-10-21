-- 0002: one active session per learner, and each session keeps the
-- difficulty its exercise had when it started.

-- A learner has at most one active session. The CLI checks this before it
-- starts one; the index makes it hold even if two starts race.
CREATE UNIQUE INDEX one_active_session_per_learner
    ON session (learner_id)
    WHERE status = 'active';

-- Levels are replayed from this column, not from the exercise table, so
-- recalibrating an exercise later does not rewrite past levels.
ALTER TABLE session ADD COLUMN exercise_difficulty INT;

UPDATE session s
SET exercise_difficulty = x.difficulty
FROM exercise x
WHERE x.id = s.exercise_id;

ALTER TABLE session ALTER COLUMN exercise_difficulty SET NOT NULL;
ALTER TABLE session ADD CONSTRAINT session_exercise_difficulty_range
    CHECK (exercise_difficulty BETWEEN 1 AND 3);
