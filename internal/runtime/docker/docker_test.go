package docker

import (
	"bytes"
	"context"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/DIZAMU/RootCause/internal/runtime"
)

// newOrSkip returns a Docker runtime, or skips the test if docker is not
// usable here (e.g. CI without a daemon). These are integration tests.
func newOrSkip(t *testing.T) *Runtime {
	t.Helper()
	r, err := New()
	if err != nil {
		t.Skip("docker CLI not available:", err)
	}
	if err := exec.Command("docker", "info").Run(); err != nil {
		t.Skip("docker daemon not reachable:", err)
	}
	return r
}

const testImage = "alpine:latest"

func TestSandboxLifecycle(t *testing.T) {
	r := newOrSkip(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	name := "rc-test-lifecycle-" + sanitize(t.Name())
	// keep PID 1 alive so we can exec into it
	_ = r.Destroy(ctx, name)
	spec := runtime.SandboxSpec{
		Name:    name,
		Image:   testImage,
		Memory:  "256Mi",
		CPUs:    0.5,
		Network: "none",
		Env:     map[string]string{"RC_SEED": "1"},
		Labels:  map[string]string{"rootcause.session": "test-session"},
	}
	// alpine's default command exits immediately; override by creating with a
	// long-running PID 1 via a spec image that sleeps. We use `sleep` through Exec
	// after starting a kept-alive container.
	sb, err := r.createSleeping(ctx, spec)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	t.Cleanup(func() { _ = r.Destroy(context.Background(), name) })
	if sb.ID == "" {
		t.Fatal("create returned empty id")
	}

	// Exec: capture output and exit code.
	res, err := r.Exec(ctx, name, []string{"id", "-u"}, nil)
	if err != nil {
		t.Fatalf("exec id: %v", err)
	}
	if res.ExitCode != 0 {
		t.Fatalf("exec id exit = %d, stderr=%s", res.ExitCode, res.Stderr)
	}
	if got := strings.TrimSpace(string(res.Stdout)); got != "0" {
		t.Errorf("uid inside sandbox = %q, want 0 (root inside)", got)
	}

	// Non-zero exit is reported, not errored.
	res, err = r.Exec(ctx, name, []string{"sh", "-c", "exit 3"}, nil)
	if err != nil {
		t.Fatalf("exec false: %v", err)
	}
	if res.ExitCode != 3 {
		t.Errorf("exit code = %d, want 3", res.ExitCode)
	}

	// CopyIn then read it back.
	payload := []byte("hello from the engine\n")
	if err := r.CopyIn(ctx, name, "/tmp/payload", payload); err != nil {
		t.Fatalf("copyin: %v", err)
	}
	res, err = r.Exec(ctx, name, []string{"cat", "/tmp/payload"}, nil)
	if err != nil || res.ExitCode != 0 {
		t.Fatalf("cat payload: err=%v exit=%d stderr=%s", err, res.ExitCode, res.Stderr)
	}
	if !bytes.Equal(res.Stdout, payload) {
		t.Errorf("payload = %q, want %q", res.Stdout, payload)
	}

	// Stdin is piped through.
	res, err = r.Exec(ctx, name, []string{"cat"}, strings.NewReader("piped"))
	if err != nil || res.ExitCode != 0 {
		t.Fatalf("cat stdin: err=%v exit=%d", err, res.ExitCode)
	}
	if got := string(res.Stdout); got != "piped" {
		t.Errorf("stdin echo = %q, want %q", got, "piped")
	}

	// List finds our sandbox by label.
	boxes, err := r.List(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	found := false
	for _, b := range boxes {
		if b.Name == name {
			found = true
		}
	}
	if !found {
		t.Errorf("List did not include %q; got %+v", name, boxes)
	}

	// Destroy, then List no longer shows it.
	if err := r.Destroy(ctx, name); err != nil {
		t.Fatalf("destroy: %v", err)
	}
	if err := r.Destroy(ctx, name); err != nil {
		t.Errorf("destroy of already-gone sandbox should be a no-op, got %v", err)
	}
}

// createSleeping starts a container whose PID 1 stays alive, so the test can
// exec into it. It mirrors Create but appends a long-running command.
func (r *Runtime) createSleeping(ctx context.Context, spec runtime.SandboxSpec) (runtime.Sandbox, error) {
	args := []string{
		"run", "-d",
		"--name", spec.Name,
		"--cap-drop", "ALL",
		"--security-opt", "no-new-privileges",
		"--pids-limit", "256",
		"--network", networkArg(spec.Network),
		"--label", labelSandbox + "=true",
		"--memory", dockerMemory(spec.Memory),
		"--label", "rootcause.session=test-session",
		"--entrypoint", "sleep",
		spec.Image, "infinity",
	}
	out, errOut, err := r.run(ctx, nil, args...)
	if err != nil {
		return runtime.Sandbox{}, errorf(err, errOut)
	}
	return runtime.Sandbox{ID: strings.TrimSpace(string(out)), Name: spec.Name, Image: spec.Image}, nil
}

func errorf(err error, errOut string) error {
	return &runErr{err: err, out: strings.TrimSpace(errOut)}
}

type runErr struct {
	err error
	out string
}

func (e *runErr) Error() string { return e.err.Error() + ": " + e.out }

func sanitize(s string) string {
	return strings.NewReplacer("/", "-", " ", "-").Replace(s)
}
