package session

import (
	"strings"
	"testing"

	"github.com/DIZAMU/RootCause/internal/exercise"
)

func loadSvc01(t *testing.T) *exercise.Exercise {
	t.Helper()
	ex, err := exercise.Load("../../exercises/svc-01-intranet-config")
	if err != nil {
		t.Fatalf("load svc-01: %v", err)
	}
	return ex
}

func TestAssertTextsFillable(t *testing.T) {
	ex := loadSvc01(t)

	// break.sh derives "line"; with it every placeholder is filled.
	if err := assertTextsFillable(ex, map[string]any{"line": 5}); err != nil {
		t.Errorf("with the derived line: %v", err)
	}

	// Without it, hint 3 and the walkthrough reference a value nothing fills.
	err := assertTextsFillable(ex, nil)
	if err == nil || !strings.Contains(err.Error(), "line") {
		t.Errorf("without derived values: err = %v, want it to name {line}", err)
	}
}

func TestAssertFeedbackKnown(t *testing.T) {
	ex := loadSvc01(t)
	if err := assertFeedbackKnown(ex, "config_invalid"); err != nil {
		t.Errorf("known key: %v", err)
	}
	for _, key := range []string{"no_such_key", ""} {
		if err := assertFeedbackKnown(ex, key); err == nil {
			t.Errorf("key %q: want an error, got nil", key)
		}
	}
}
