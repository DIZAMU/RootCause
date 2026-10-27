package session

import (
	"context"
	"errors"
	"io"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/DIZAMU/RootCause/internal/exercise"
	"github.com/DIZAMU/RootCause/internal/runtime"
)

// scriptedRuntime is a fake runtime. exec answers every Exec; it records the
// sandboxes it creates and destroys. No Docker needed.
type scriptedRuntime struct {
	exec      func(id string, cmd []string) (runtime.ExecResult, error)
	created   []runtime.SandboxSpec
	destroyed []string
}

func (s *scriptedRuntime) Create(_ context.Context, spec runtime.SandboxSpec) (runtime.Sandbox, error) {
	s.created = append(s.created, spec)
	return runtime.Sandbox{ID: "id-" + spec.Name, Name: spec.Name, Labels: spec.Labels}, nil
}
func (s *scriptedRuntime) Exec(_ context.Context, id string, cmd []string, _ io.Reader) (runtime.ExecResult, error) {
	return s.exec(id, cmd)
}
func (s *scriptedRuntime) CopyIn(context.Context, string, string, []byte) error { return nil }
func (s *scriptedRuntime) Restart(context.Context, string) error                { return nil }
func (s *scriptedRuntime) Destroy(_ context.Context, id string) error {
	s.destroyed = append(s.destroyed, id)
	return nil
}
func (s *scriptedRuntime) List(context.Context) ([]runtime.Sandbox, error) { return nil, nil }

// fixture loads the exercise loader's valid test fixture and lets the test
// adjust its settings, so these tests do not depend on a real exercise.
func fixture(t *testing.T, adjust func(*exercise.Exercise)) *exercise.Exercise {
	t.Helper()
	ex, err := exercise.Load(filepath.Join("..", "exercise", "testdata", "valid"))
	if err != nil {
		t.Fatalf("load fixture: %v", err)
	}
	adjust(ex)
	return ex
}

func TestRunnerSpec(t *testing.T) {
	sandbox := runtime.SandboxSpec{
		Name: "rootcause-s-1", Image: "img", Memory: "256Mi", CPUs: 0.5, Network: "internal",
		Caps:   []string{"SETUID"},
		Labels: map[string]string{labelSession: "s-1", labelDatabase: "rootcause_test"},
	}
	got := RunnerSpec(sandbox)
	if got.Name != "rootcause-s-1-runner" || got.Image != "img" || got.Network != "internal" {
		t.Errorf("runner name/image/network = %q/%q/%q", got.Name, got.Image, got.Network)
	}
	if len(got.Caps) != 0 {
		t.Errorf("runner caps = %v, want none", got.Caps)
	}
	for k, v := range map[string]string{labelSession: "s-1", labelDatabase: "rootcause_test", labelRunner: "true"} {
		if got.Labels[k] != v {
			t.Errorf("runner label %s = %q, want %q", k, got.Labels[k], v)
		}
	}
	if _, ok := sandbox.Labels[labelRunner]; ok {
		t.Error("RunnerSpec changed the sandbox's own labels")
	}
}

// TestRunCheckRunnerDestroyedOnError checks that a runner check runs check.sh
// in the runner with RC_SANDBOX_HOST, and destroys the runner even when the
// exec itself fails.
func TestRunCheckRunnerDestroyedOnError(t *testing.T) {
	ex := fixture(t, func(ex *exercise.Exercise) {
		ex.Network, ex.Check.RunsFrom = "internal", "runner"
	})
	var runnerCmd []string
	rt := &scriptedRuntime{exec: func(id string, cmd []string) (runtime.ExecResult, error) {
		runnerCmd = cmd
		return runtime.ExecResult{}, errors.New("exec broke")
	}}
	e := &Engine{rt: rt}
	spec := e.specFor(ex, "s-1")

	if _, err := e.runCheck(context.Background(), ex, spec, "id-sandbox", map[string]string{"RC_VAR_PORT": "8080"}); err == nil {
		t.Fatal("runCheck returned no error although the exec failed")
	}
	if len(rt.created) != 1 || rt.created[0].Labels[labelRunner] != "true" {
		t.Fatalf("created %+v, want one runner", rt.created)
	}
	runnerID := "id-" + rt.created[0].Name
	found := false
	for _, d := range rt.destroyed {
		found = found || d == runnerID
	}
	if !found {
		t.Errorf("runner %s not destroyed; destroyed %v", runnerID, rt.destroyed)
	}
	if !strings.Contains(strings.Join(runnerCmd, " "), "RC_SANDBOX_HOST="+spec.Name) {
		t.Errorf("runner command %v lacks RC_SANDBOX_HOST=%s", runnerCmd, spec.Name)
	}
}

// TestWaitBooted checks that a degraded boot fails a new systemd sandbox,
// naming the failed units, but counts as booted after the check's restart.
func TestWaitBooted(t *testing.T) {
	ex := fixture(t, func(ex *exercise.Exercise) { ex.Init = "systemd" })
	rt := &scriptedRuntime{exec: func(_ string, cmd []string) (runtime.ExecResult, error) {
		if cmd[1] == "is-system-running" {
			return runtime.ExecResult{ExitCode: 1, Stdout: []byte("degraded\n")}, nil
		}
		return runtime.ExecResult{Stdout: []byte("broken.service loaded failed failed\n")}, nil
	}}
	e := &Engine{rt: rt}

	err := e.waitReady(context.Background(), ex, "id", true)
	if err == nil || !strings.Contains(err.Error(), "broken.service") {
		t.Errorf("strict wait on a degraded boot: err = %v, want one naming broken.service", err)
	}
	if err := e.waitReady(context.Background(), ex, "id", false); err != nil {
		t.Errorf("wait after restart on a degraded boot: %v, want booted", err)
	}
}

// TestAssertNoTrace checks that a script found anywhere in the sandbox, or an
// RC_ value in its environment, fails the selftest.
func TestAssertNoTrace(t *testing.T) {
	for _, c := range []struct {
		name, find, env, want string
	}{
		{"clean", "", "PATH=/usr/bin\n", ""},
		{"script in image", "/opt/app/check.sh\n", "PATH=/usr/bin\n", "/opt/app/check.sh"},
		{"seed in env", "", "RC_SEED=1\n", "RC_SEED"},
	} {
		t.Run(c.name, func(t *testing.T) {
			rt := &scriptedRuntime{exec: func(_ string, cmd []string) (runtime.ExecResult, error) {
				if cmd[0] == "find" {
					return runtime.ExecResult{Stdout: []byte(c.find)}, nil
				}
				return runtime.ExecResult{Stdout: []byte(c.env)}, nil
			}}
			err := (&Engine{rt: rt}).assertNoTrace(context.Background(), "id")
			if c.want == "" && err != nil {
				t.Errorf("clean sandbox: %v", err)
			}
			if c.want != "" && (err == nil || !strings.Contains(err.Error(), c.want)) {
				t.Errorf("err = %v, want one mentioning %s", err, c.want)
			}
		})
	}
}

// cgroupV1Runtime is a scripted runtime on a host that cannot boot systemd.
type cgroupV1Runtime struct{ scriptedRuntime }

func (cgroupV1Runtime) CheckSystemd(context.Context) error { return runtime.ErrNeedsCgroupV2 }

// TestSystemdNeedsCgroupV2 checks that an init: systemd exercise is refused
// with ErrNeedsCgroupV2 on a cgroup v1 host, by Start and by the selftest,
// before anything is built, stored or created.
func TestSystemdNeedsCgroupV2(t *testing.T) {
	rt := &cgroupV1Runtime{}
	exDir, _ := filepath.Abs("../../exercises")
	e := &Engine{rt: rt, exDir: exDir} // no store and no builder: neither may be reached

	if _, err := e.Start(context.Background(), 1, "svc-02-dies-on-reboot", 1); !errors.Is(err, runtime.ErrNeedsCgroupV2) {
		t.Errorf("Start: err = %v, want ErrNeedsCgroupV2", err)
	}
	err := SelfTest(context.Background(), rt, nil, filepath.Join(exDir, "svc-02-dies-on-reboot"), 1, io.Discard)
	if !errors.Is(err, runtime.ErrNeedsCgroupV2) {
		t.Errorf("SelfTest: err = %v, want ErrNeedsCgroupV2", err)
	}
	if len(rt.created) != 0 {
		t.Errorf("created %d sandboxes, want none", len(rt.created))
	}
}

// diagnosingRuntime is a scripted runtime that can describe a sandbox.
type diagnosingRuntime struct{ scriptedRuntime }

func (diagnosingRuntime) Diagnose(context.Context, string) string {
	return "status exited, exit code 255; no log output"
}

// TestNotReadyDiagnoses checks that a readiness timeout names the sandbox's
// state, for plain and systemd sandboxes.
func TestNotReadyDiagnoses(t *testing.T) {
	rt := &diagnosingRuntime{scriptedRuntime{exec: func(string, []string) (runtime.ExecResult, error) {
		return runtime.ExecResult{}, errors.New("container is not running")
	}}}
	e := &Engine{rt: rt}
	for _, init := range []string{"none", "systemd"} {
		ex := fixture(t, func(ex *exercise.Exercise) { ex.Init = init })
		ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
		err := e.waitReady(ctx, ex, "id", true)
		cancel()
		if err == nil || !strings.Contains(err.Error(), "exit code 255") {
			t.Errorf("init %s: err = %v, want one with the diagnosis", init, err)
		}
	}
}
