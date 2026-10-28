package store

import (
	"context"
	"fmt"
)

// WipeLearnerData deletes everything the learner has done, in one
// transaction: every session and event, the statistics (the analytics schema
// dbt builds, so the marts read as never built), and the learner's settings
// (the language goes back to the default). The learner row keeps its id, so
// a running dashboard and the CLI keep working. The exercise catalog and the
// migrations stay. It only touches the database the store is connected to.
//
// It holds the learner and stats advisory locks, so it waits for a dbt build
// the CLI or the dashboard is running and cannot interleave with one.
func (s *Store) WipeLearnerData(ctx context.Context, learnerID int) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer tx.Rollback(ctx)
	for _, key := range []int64{learnerLockKey, statsLockKey} {
		if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock($1)`, key); err != nil {
			return fmt.Errorf("wipe: lock: %w", err)
		}
	}
	for _, stmt := range []string{
		`TRUNCATE event, session RESTART IDENTITY`,
		`DROP SCHEMA IF EXISTS analytics CASCADE`,
	} {
		if _, err := tx.Exec(ctx, stmt); err != nil {
			return fmt.Errorf("wipe: %w", err)
		}
	}
	tag, err := tx.Exec(ctx, `UPDATE learner SET lang = DEFAULT, created_at = now() WHERE id = $1`, learnerID)
	if err != nil {
		return fmt.Errorf("wipe: reset learner: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("wipe: no learner %d", learnerID)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}
