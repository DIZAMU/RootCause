package session

import (
	"context"
	"path/filepath"
	"slices"
	"testing"

	"github.com/DIZAMU/RootCause/internal/runtime/scripted"
	"github.com/DIZAMU/RootCause/internal/testenv"
)

// TestStartReportsSteps: Start reports the four generic steps, in order,
// to a context from WithStartProgress.
func TestStartReportsSteps(t *testing.T) {
	st := testenv.Store(t, testenv.DSN(t))
	rt := &scripted.Runtime{}
	exDir, _ := filepath.Abs("../../exercises")
	eng := New(rt, rt, nil, st, exDir)
	learner := testLearner(t, st, rt)

	var steps []StartStep
	ctx := WithStartProgress(context.Background(), func(s StartStep) { steps = append(steps, s) })
	if _, err := eng.Start(ctx, learner, "perm-01-shared-folder", 1); err != nil {
		t.Fatal(err)
	}
	want := []StartStep{StepBuildImage, StepStartMachine, StepSetUp, StepPlantFault}
	if !slices.Equal(steps, want) {
		t.Errorf("steps %v, want %v", steps, want)
	}
	if len(want) != StartSteps {
		t.Errorf("StartSteps is %d, the engine reports %d steps", StartSteps, len(want))
	}
}
