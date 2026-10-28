// Package scripted is a stand-in runtime for tests. It lets the engine's real
// Start, Hint, Check, Reset, GiveUp and Expire run without Docker: every
// command succeeds, check.sh passes or fails as the test sets, and systemd
// reports a finished boot. Only tests import it.
package scripted

import (
	"context"
	"io"
	"strings"
	"sync"

	"github.com/DIZAMU/RootCause/internal/runtime"
)

// Runtime implements runtime.Runtime and session.Builder.
type Runtime struct {
	mu          sync.Mutex
	checkPasses bool
	feedback    string
	buildHook   func()
	live        map[string]runtime.Sandbox // created and not yet destroyed, by id
}

// SetBuildHook makes BuildImage call hook first, so a test can hold a start
// at its first step.
func (r *Runtime) SetBuildHook(hook func()) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.buildHook = hook
}

// SetCheck makes later checks pass, or fail with the feedback key.
func (r *Runtime) SetCheck(passes bool, feedback string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.checkPasses, r.feedback = passes, feedback
}

// Create records the sandbox as live until Destroy.
func (r *Runtime) Create(_ context.Context, spec runtime.SandboxSpec) (runtime.Sandbox, error) {
	b := runtime.Sandbox{ID: "fake-" + spec.Name, Name: spec.Name, Image: spec.Image, Labels: spec.Labels}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.live == nil {
		r.live = make(map[string]runtime.Sandbox)
	}
	r.live[b.ID] = b
	return b, nil
}

func (r *Runtime) Exec(_ context.Context, _ string, cmd []string, _ io.Reader) (runtime.ExecResult, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	joined := strings.Join(cmd, " ")
	switch {
	case strings.Contains(joined, "is-system-running"):
		return runtime.ExecResult{Stdout: []byte("running\n")}, nil
	case strings.HasSuffix(joined, "/root/check.sh") && r.checkPasses:
		return runtime.ExecResult{Stdout: []byte(`{"passed": true}` + "\n")}, nil
	case strings.HasSuffix(joined, "/root/check.sh"):
		return runtime.ExecResult{ExitCode: 1, Stdout: []byte(`{"passed": false, "feedback": "` + r.feedback + `"}` + "\n")}, nil
	}
	return runtime.ExecResult{Stdout: []byte("{}\n")}, nil
}

func (r *Runtime) CopyIn(context.Context, string, string, []byte) error { return nil }
func (r *Runtime) Restart(context.Context, string) error                { return nil }

// Destroy forgets the sandbox. An unknown id is not an error, as in Docker's
// forced removal.
func (r *Runtime) Destroy(_ context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.live, id)
	return nil
}

// List returns the live sandboxes.
func (r *Runtime) List(context.Context) ([]runtime.Sandbox, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]runtime.Sandbox, 0, len(r.live))
	for _, b := range r.live {
		out = append(out, b)
	}
	return out, nil
}

// BuildImage calls the build hook, if set, and succeeds.
func (r *Runtime) BuildImage(context.Context, string, string) error {
	r.mu.Lock()
	hook := r.buildHook
	r.mu.Unlock()
	if hook != nil {
		hook()
	}
	return nil
}

// Terminal echoes stdin to stdout, like a shell printing what is typed, until
// stdin ends or ctx does.
func (r *Runtime) Terminal(ctx context.Context, _ string, _ []string, stdin io.Reader, stdout io.Writer, _ <-chan runtime.TermSize) error {
	done := make(chan error, 1)
	go func() {
		_, err := io.Copy(stdout, stdin)
		done <- err
	}()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-done:
		return err
	}
}
