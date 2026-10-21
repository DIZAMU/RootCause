package session

import (
	"context"
	"testing"

	"github.com/DIZAMU/RootCause/internal/runtime"
	"github.com/DIZAMU/RootCause/internal/store"
)

// testLearner creates a learner used only by the calling test, so the test
// does not depend on, or disturb, the profile learner or data that other tests
// leave in the test database. Afterwards it voids the learner's active
// sessions, destroys their sandboxes and deletes the learner's rows.
//
// Register t.Cleanup(st.Close) before calling it: cleanups run in reverse
// order, so the store is then still open when this cleanup runs.
func testLearner(t *testing.T, st *store.Store, rt runtime.Runtime) int {
	t.Helper()
	ctx := context.Background()

	// Make sure the profile learner exists first. It then keeps the lowest
	// id, so EnsureLearner never returns a test learner.
	if _, err := st.EnsureLearner(ctx, "DIZAMU"); err != nil {
		t.Fatalf("ensure profile learner: %v", err)
	}
	var id int
	if err := st.Pool().QueryRow(ctx,
		`INSERT INTO learner (name) VALUES ($1) RETURNING id`, "session-test-"+t.Name()).Scan(&id); err != nil {
		t.Fatalf("create test learner: %v", err)
	}

	t.Cleanup(func() {
		ctx := context.Background()
		rows, err := st.Pool().Query(ctx, `
			UPDATE session SET status = 'voided', ended_at = now()
			WHERE learner_id = $1 AND status = 'active'
			RETURNING COALESCE(sandbox_id, '')`, id)
		if err != nil {
			t.Errorf("void test sessions: %v", err)
		} else {
			var sandboxes []string
			for rows.Next() {
				var sb string
				if err := rows.Scan(&sb); err != nil {
					t.Errorf("read sandbox id: %v", err)
				}
				if sb != "" {
					sandboxes = append(sandboxes, sb)
				}
			}
			rows.Close()
			for _, sb := range sandboxes {
				if err := rt.Destroy(ctx, sb); err != nil {
					t.Errorf("destroy sandbox %s: %v", sb, err)
				}
			}
		}
		for _, sql := range []string{
			`DELETE FROM event WHERE session_id IN (SELECT id FROM session WHERE learner_id = $1)`,
			`DELETE FROM session WHERE learner_id = $1`,
			`DELETE FROM learner WHERE id = $1`,
		} {
			if _, err := st.Pool().Exec(ctx, sql, id); err != nil {
				t.Errorf("cleanup %q: %v", sql, err)
			}
		}
	})
	return id
}
