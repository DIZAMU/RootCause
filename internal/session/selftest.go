package session

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"

	"github.com/DIZAMU/RootCause/internal/exercise"
	"github.com/DIZAMU/RootCause/internal/i18n"
	"github.com/DIZAMU/RootCause/internal/runtime"
)

// SelfTest proves one exercise works end to end for one seed (spec section 18,
// Phase 1), using exactly the sandbox flags and script handling a real session
// uses, so validation cannot drift from the engine:
//
//	A. unsolved: setup, break, then check must exit 1 (the fault is detected)
//	B. solved:   setup, break, solution, then check must exit 0
//
// In scenario A it also asserts that no exercise script is left in the sandbox
// and that no RC_ value is visible in its environment. It needs Docker, not
// the database. Progress is written to log.
func SelfTest(ctx context.Context, rt runtime.Runtime, builder Builder, exDir string, seed int64, log io.Writer) error {
	ex, err := exercise.Load(exDir)
	if err != nil {
		return err
	}
	e := &Engine{rt: rt, builder: builder}

	fmt.Fprintf(log, "== building image %s\n", imageTag(ex))
	if err := builder.BuildImage(ctx, imageTag(ex), ex.Dir()); err != nil {
		return err
	}
	env := scriptEnv(seed, ex.SelectVariant(seed))

	fmt.Fprintln(log, "== scenario A (unsolved): setup, break, then check must FAIL")
	if err := e.selfTestScenario(ctx, ex, seed, "a", env, false, log); err != nil {
		return fmt.Errorf("scenario A: %w", err)
	}
	fmt.Fprintln(log, "== scenario B (solved): setup, break, solution, then check must PASS")
	if err := e.selfTestScenario(ctx, ex, seed, "b", env, true, log); err != nil {
		return fmt.Errorf("scenario B: %w", err)
	}
	fmt.Fprintf(log, "== PASS: %s seed %d\n", ex.ID, seed)
	return nil
}

// selfTestScenario runs one scenario in its own fresh sandbox.
func (e *Engine) selfTestScenario(ctx context.Context, ex *exercise.Exercise, seed int64, tag string, env map[string]string, solve bool, log io.Writer) error {
	name := fmt.Sprintf("selftest-%s-%d-%s", ex.ID, seed, tag)
	_ = e.rt.Destroy(ctx, "rootcause-"+name)
	sb, err := e.rt.Create(ctx, e.specFor(ex, name))
	if err != nil {
		return err
	}
	defer func() { _ = e.rt.Destroy(context.Background(), sb.ID) }()

	derived, err := e.prepare(ctx, sb.ID, ex, env)
	if err != nil {
		return err
	}
	fmt.Fprintf(log, "break:    derived %v\n", derived)

	if !solve {
		if err := e.assertNoTrace(ctx, sb.ID); err != nil {
			return err
		}
		if err := assertTextsFillable(ex, derived); err != nil {
			return err
		}
		fmt.Fprintln(log, "texts:    every placeholder is filled in en and no")
	} else {
		res, err := e.execScript(ctx, sb.ID, ex.Dir(), "solution.sh", env)
		if err != nil {
			return err
		}
		if res.ExitCode != 0 {
			return fmt.Errorf("solution.sh exited %d: %s", res.ExitCode, strings.TrimSpace(string(res.Stderr)))
		}
	}

	if ex.Check.Before == "restart" {
		if err := e.rt.Restart(ctx, sb.ID); err != nil {
			return err
		}
		time.Sleep(time.Second)
	}
	res, err := e.execScript(ctx, sb.ID, ex.Dir(), "check.sh", env)
	if err != nil {
		return err
	}
	out := strings.TrimSpace(string(res.Stdout))
	fmt.Fprintf(log, "check:    %s (exit %d)\n", out, res.ExitCode)

	var parsed checkOut
	if err := json.Unmarshal([]byte(out), &parsed); err != nil || strings.Contains(out, "\n") {
		return fmt.Errorf("check.sh must print exactly one line of JSON, got %q", out)
	}
	want := 1
	if solve {
		want = 0
	}
	if res.ExitCode != want || parsed.Passed != solve {
		return fmt.Errorf("check exit %d passed=%v, want exit %d passed=%v", res.ExitCode, parsed.Passed, want, solve)
	}
	if !solve {
		if err := assertFeedbackKnown(ex, parsed.Feedback); err != nil {
			return err
		}
		fmt.Fprintf(log, "feedback: %q has text in en and no\n", parsed.Feedback)
	}
	return nil
}

// assertTextsFillable fails if a {placeholder} in either language file is not
// filled by a variant value or by a value break.sh derived.
func assertTextsFillable(ex *exercise.Exercise, derived map[string]any) error {
	available := make(map[string]bool)
	for name := range ex.Variants {
		available[name] = true
	}
	for name := range derived {
		available[name] = true
	}
	for _, lang := range i18n.Languages {
		t, err := ex.LoadI18n(lang)
		if err != nil {
			return err
		}
		if missing := exercise.UnfilledPlaceholders(t, available); len(missing) > 0 {
			return fmt.Errorf("%s texts use placeholders that nothing fills: %v", lang, missing)
		}
	}
	return nil
}

// assertFeedbackKnown fails if a failing check's feedback key has no text in
// either language file, so the learner would see no symptom description.
func assertFeedbackKnown(ex *exercise.Exercise, key string) error {
	for _, lang := range i18n.Languages {
		t, err := ex.LoadI18n(lang)
		if err != nil {
			return err
		}
		if strings.TrimSpace(t.Feedback[key]) == "" {
			return fmt.Errorf("check feedback key %q has no text in %s", key, lang)
		}
	}
	return nil
}

// assertNoTrace fails if an exercise script is still in the sandbox, or if the
// seed or a variant value is visible in its environment.
func (e *Engine) assertNoTrace(ctx context.Context, id string) error {
	var paths []string
	for _, name := range []string{"setup.sh", "break.sh", "check.sh", "solution.sh"} {
		paths = append(paths, filepath.Join("/root", name))
	}
	res, err := e.rt.Exec(ctx, id, append([]string{"ls"}, paths...), nil)
	if err != nil {
		return err
	}
	if res.ExitCode == 0 || len(strings.TrimSpace(string(res.Stdout))) > 0 {
		return fmt.Errorf("exercise script left in the sandbox: %s", strings.TrimSpace(string(res.Stdout)))
	}
	res, err = e.rt.Exec(ctx, id, []string{"env"}, nil)
	if err != nil {
		return err
	}
	for _, line := range strings.Split(string(res.Stdout), "\n") {
		if strings.HasPrefix(line, "RC_") {
			return fmt.Errorf("sandbox environment exposes %s", line)
		}
	}
	return nil
}
