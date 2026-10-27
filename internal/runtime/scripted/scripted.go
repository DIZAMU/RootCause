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
}

// SetCheck makes later checks pass, or fail with the feedback key.
func (r *Runtime) SetCheck(passes bool, feedback string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.checkPasses, r.feedback = passes, feedback
}

func (r *Runtime) Create(_ context.Context, spec runtime.SandboxSpec) (runtime.Sandbox, error) {
	return runtime.Sandbox{ID: "fake-" + spec.Name, Name: spec.Name, Labels: spec.Labels}, nil
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
func (r *Runtime) Destroy(context.Context, string) error                { return nil }
func (r *Runtime) List(context.Context) ([]runtime.Sandbox, error)      { return nil, nil }
func (r *Runtime) BuildImage(context.Context, string, string) error     { return nil }

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
