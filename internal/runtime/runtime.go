// Package runtime abstracts the sandbox backend. All sandbox control goes
// through the Runtime interface (spec section 8.3), so v2 can swap the Docker
// implementation for a Kubernetes one without changing any caller.
package runtime

import (
	"context"
	"io"
	"time"
)

// Runtime creates and controls sandboxes. The v1 implementation is
// internal/runtime/docker; v2 will add internal/runtime/kubernetes.
type Runtime interface {
	Create(ctx context.Context, spec SandboxSpec) (Sandbox, error)
	Exec(ctx context.Context, id string, cmd []string, stdin io.Reader) (ExecResult, error)
	CopyIn(ctx context.Context, id string, path string, data []byte) error
	Restart(ctx context.Context, id string) error
	Destroy(ctx context.Context, id string) error
	List(ctx context.Context) ([]Sandbox, error)
}

// SandboxSpec describes a sandbox to create. Security defaults (spec section
// 13.1) are applied by the implementation: all capabilities dropped, limits
// enforced, no Docker socket, no host network, no privileged mode.
//
// There is deliberately no environment field: RC_SEED and RC_VAR_* values are
// passed only to the commands that need them (setup, break, check), so the
// learner cannot read them from the sandbox's environment.
type SandboxSpec struct {
	Name    string            // container name, unique per session
	Image   string            // exercise image
	Memory  string            // e.g. "256Mi"
	CPUs    float64           // e.g. 0.5
	Network string            // "none" | "internal"
	Caps    []string          // Linux capabilities to add back after dropping all
	Pids    int               // process-count limit; 0 means the implementation default
	Labels  map[string]string // bookkeeping labels, e.g. the session id
	Init    string            // PID 1: "" or "none" (an init the runtime adds), "systemd" (the image boots systemd)
	Tmpfs   []Tmpfs           // size-limited in-memory volumes
}

// Tmpfs is one size-limited in-memory volume in a sandbox.
type Tmpfs struct {
	Path string // absolute mount point
	Size string // e.g. "32Mi"
}

// Sandbox is a created sandbox.
type Sandbox struct {
	ID      string
	Name    string
	Image   string
	Created time.Time
	Labels  map[string]string
}

// ExecResult is the outcome of a non-interactive Exec.
type ExecResult struct {
	ExitCode int
	Stdout   []byte
	Stderr   []byte
}

// Sheller is an optional, runtime-specific capability: open an interactive
// shell wired to the caller's own terminal. It is not part of Runtime because
// v1 (docker exec -it) and v2 (ttyd in the browser) do it in unrelated ways.
type Sheller interface {
	Shell(ctx context.Context, id string, cmd []string) error
}
