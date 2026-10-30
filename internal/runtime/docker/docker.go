// Package docker implements runtime.Runtime by shelling out to the docker CLI.
// Every sandbox is created locked down (ARCHITECTURE.md section 13.1): all Linux
// capabilities dropped, memory/CPU/process limits, no privileged mode, no
// Docker socket, an init process as PID 1 (docker's, or systemd for
// exercises that boot it), and network "none" or an internal-only network.
package docker

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/DIZAMU/RootCause/internal/runtime"
)

// labelSandbox marks every container this package creates, so List and the
// reaper can find RootCause sandboxes and nothing else.
const labelSandbox = "rootcause.sandbox"

const defaultPidsLimit = 256

// Runtime controls Docker sandboxes.
type Runtime struct {
	bin string // docker binary, default "docker"
}

// New returns a Docker runtime. It errors if the docker CLI is not on PATH.
func New() (*Runtime, error) {
	bin := "docker"
	if _, err := exec.LookPath(bin); err != nil {
		return nil, fmt.Errorf("docker CLI not found on PATH: %w", err)
	}
	return &Runtime{bin: bin}, nil
}

var _ runtime.Runtime = (*Runtime)(nil)
var _ runtime.Sheller = (*Runtime)(nil)
var _ runtime.Diagnoser = (*Runtime)(nil)
var _ runtime.SystemdChecker = (*Runtime)(nil)

// Create starts a locked-down sandbox and returns it.
func (r *Runtime) Create(ctx context.Context, spec runtime.SandboxSpec) (runtime.Sandbox, error) {
	if spec.Name == "" || spec.Image == "" {
		return runtime.Sandbox{}, fmt.Errorf("sandbox spec needs a name and an image")
	}
	pids := spec.Pids
	if pids == 0 {
		pids = defaultPidsLimit
	}
	if spec.Network == "internal" {
		if err := r.EnsureInternalNetwork(ctx); err != nil {
			return runtime.Sandbox{}, err
		}
	}

	args := []string{
		"run", "-d",
		"--name", spec.Name,
		"--cap-drop", "ALL",
		"--security-opt", "no-new-privileges",
		"--pids-limit", fmt.Sprintf("%d", pids),
		"--network", networkArg(spec.Network),
		"--label", labelSandbox + "=true",
	}
	if spec.Init == "systemd" {
		// systemd must be PID 1 and reaps stopped processes itself. It needs
		// writable in-memory /run and /run/lock.
		args = append(args, "--tmpfs", "/run", "--tmpfs", "/run/lock")
	} else {
		args = append(args, "--init") // a real PID 1 that reaps stopped processes (no zombies)
	}
	for _, t := range spec.Tmpfs {
		args = append(args, "--tmpfs", t.Path+":size="+dockerMemory(t.Size))
	}
	for _, c := range spec.Caps {
		args = append(args, "--cap-add", c)
	}
	if spec.Memory != "" {
		args = append(args, "--memory", dockerMemory(spec.Memory))
	}
	if spec.CPUs > 0 {
		args = append(args, "--cpus", fmt.Sprintf("%g", spec.CPUs))
	}
	for k, v := range spec.Labels {
		args = append(args, "--label", k+"="+v)
	}
	args = append(args, spec.Image)

	out, errOut, err := r.run(ctx, nil, args...)
	if err != nil {
		return runtime.Sandbox{}, fmt.Errorf("docker run: %w: %s", err, strings.TrimSpace(errOut))
	}
	id := strings.TrimSpace(string(out))
	return runtime.Sandbox{
		ID:      id,
		Name:    spec.Name,
		Image:   spec.Image,
		Created: time.Now(),
		Labels:  spec.Labels,
	}, nil
}

// CheckIsolation verifies that root inside a sandbox is not root on the host
// (ARCHITECTURE.md section 13.1): the daemon must run with user namespace
// remapping or in rootless mode. It returns an error otherwise.
func (r *Runtime) CheckIsolation(ctx context.Context) error {
	out, errOut, err := r.run(ctx, nil, "info", "--format", "{{json .SecurityOptions}}")
	if err != nil {
		return fmt.Errorf("docker info: %w: %s", err, strings.TrimSpace(errOut))
	}
	if !isolated(string(out)) {
		return ErrNotIsolated
	}
	return nil
}

// ErrNotIsolated means the Docker daemon maps container root to host root.
var ErrNotIsolated = errors.New("docker daemon runs without userns-remap or rootless mode")

// isolated reports whether docker info security options show user namespace
// remapping ("name=userns") or rootless mode ("name=rootless").
func isolated(securityOptions string) bool {
	return strings.Contains(securityOptions, "name=userns") ||
		strings.Contains(securityOptions, "name=rootless")
}

// CheckSystemd returns runtime.ErrNeedsCgroupV2 unless the daemon reports
// cgroup version 2. systemd 257 (Debian 13) does not boot on cgroup v1.
func (r *Runtime) CheckSystemd(ctx context.Context) error {
	out, errOut, err := r.run(ctx, nil, "info", "--format", "{{.CgroupVersion}}")
	if err != nil {
		return fmt.Errorf("docker info: %w: %s", err, strings.TrimSpace(errOut))
	}
	if v := strings.TrimSpace(string(out)); v != "2" {
		return fmt.Errorf("%w (docker reports cgroup version %q)", runtime.ErrNeedsCgroupV2, v)
	}
	return nil
}

// diagnoseLogLines is how many of a sandbox's last log lines Diagnose shows.
const diagnoseLogLines = 20

// Diagnose describes a sandbox's state for an error message: its status, its
// exit code and its last log lines. It never fails; what it cannot read, it
// says it cannot read.
func (r *Runtime) Diagnose(ctx context.Context, id string) string {
	var b strings.Builder
	out, errOut, err := r.run(ctx, nil, "inspect", "--format", "{{.State.Status}} {{.State.ExitCode}}", id)
	if err != nil {
		fmt.Fprintf(&b, "state unknown: %s", strings.TrimSpace(errOut))
	} else if status, code, ok := strings.Cut(strings.TrimSpace(string(out)), " "); ok {
		fmt.Fprintf(&b, "status %s, exit code %s", status, code)
	}
	// docker logs writes the container's stdout and stderr to its own.
	logs, logErr, err := r.run(ctx, nil, "logs", "--tail", fmt.Sprint(diagnoseLogLines), id)
	tail := strings.TrimSpace(string(logs) + logErr)
	switch {
	case err != nil:
		b.WriteString("; logs unavailable")
	case tail == "":
		b.WriteString("; no log output")
	default:
		fmt.Fprintf(&b, "; last log lines:\n%s", tail)
	}
	return b.String()
}

// Exec runs a command in the sandbox and captures its output and exit code.
// A non-zero exit from the command is returned in ExecResult.ExitCode, not as
// a Go error; a Go error means the exec itself could not run.
func (r *Runtime) Exec(ctx context.Context, id string, cmd []string, stdin io.Reader) (runtime.ExecResult, error) {
	args := []string{"exec"}
	if stdin != nil {
		args = append(args, "-i")
	}
	args = append(args, id)
	args = append(args, cmd...)

	out, errOut, err := r.run(ctx, stdin, args...)
	res := runtime.ExecResult{Stdout: out, Stderr: []byte(errOut)}
	if err == nil {
		res.ExitCode = 0
		return res, nil
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		res.ExitCode = ee.ExitCode()
		return res, nil
	}
	return res, fmt.Errorf("docker exec: %w", err)
}

// CopyIn writes data to a path inside the sandbox via a temporary file.
func (r *Runtime) CopyIn(ctx context.Context, id string, path string, data []byte) error {
	tmp, err := os.CreateTemp("", "rc-copyin-*")
	if err != nil {
		return fmt.Errorf("temp file: %w", err)
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("write temp: %w", err)
	}
	tmp.Close()
	// World-readable: under userns-remap the copied file is owned by a UID the
	// sandbox's root does not map to, so it must be readable by others to be
	// usable inside the sandbox.
	if err := os.Chmod(tmp.Name(), 0o644); err != nil {
		return fmt.Errorf("chmod temp: %w", err)
	}

	_, errOut, err := r.run(ctx, nil, "cp", tmp.Name(), id+":"+path)
	if err != nil {
		return fmt.Errorf("docker cp: %w: %s", err, strings.TrimSpace(errOut))
	}
	return nil
}

// Restart restarts the sandbox (re-runs its entrypoint), for exercises whose
// check.before is "restart".
func (r *Runtime) Restart(ctx context.Context, id string) error {
	_, errOut, err := r.run(ctx, nil, "restart", id)
	if err != nil {
		return fmt.Errorf("docker restart: %w: %s", err, strings.TrimSpace(errOut))
	}
	return nil
}

// Destroy force-removes the sandbox. It is not an error if it is already gone.
func (r *Runtime) Destroy(ctx context.Context, id string) error {
	_, errOut, err := r.run(ctx, nil, "rm", "-f", id)
	if err != nil {
		if strings.Contains(errOut, "No such container") {
			return nil
		}
		return fmt.Errorf("docker rm: %w: %s", err, strings.TrimSpace(errOut))
	}
	return nil
}

// List returns the running RootCause sandboxes, with their labels.
func (r *Runtime) List(ctx context.Context) ([]runtime.Sandbox, error) {
	const sep = "\x1f" // unit separator, safe against names/images
	format := strings.Join([]string{"{{.ID}}", "{{.Names}}", "{{.Image}}", "{{.Labels}}"}, sep)
	out, errOut, err := r.run(ctx, nil,
		"ps", "--no-trunc", "--filter", "label="+labelSandbox+"=true", "--format", format)
	if err != nil {
		return nil, fmt.Errorf("docker ps: %w: %s", err, strings.TrimSpace(errOut))
	}
	var boxes []runtime.Sandbox
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line == "" {
			continue
		}
		f := strings.Split(line, sep)
		if len(f) != 4 {
			continue
		}
		boxes = append(boxes, runtime.Sandbox{ID: f[0], Name: f[1], Image: f[2], Labels: parseLabels(f[3])})
	}
	return boxes, nil
}

// parseLabels reads docker ps's "key=value,key=value" label list. RootCause
// label values never contain a comma.
func parseLabels(s string) map[string]string {
	labels := make(map[string]string)
	for _, kv := range strings.Split(s, ",") {
		if k, v, ok := strings.Cut(kv, "="); ok {
			labels[k] = v
		}
	}
	return labels
}

// Shell opens an interactive shell wired to the caller's terminal (v1 only).
func (r *Runtime) Shell(ctx context.Context, id string, cmd []string) error {
	args := append([]string{"exec", "-it", id}, cmd...)
	c := exec.CommandContext(ctx, r.bin, args...)
	c.Stdin, c.Stdout, c.Stderr = os.Stdin, os.Stdout, os.Stderr
	return c.Run()
}

// run executes the docker CLI, capturing stdout and stderr separately.
func (r *Runtime) run(ctx context.Context, stdin io.Reader, args ...string) (stdout []byte, stderr string, err error) {
	c := exec.CommandContext(ctx, r.bin, args...)
	var outBuf, errBuf bytes.Buffer
	c.Stdout, c.Stderr, c.Stdin = &outBuf, &errBuf, stdin
	err = c.Run()
	return outBuf.Bytes(), errBuf.String(), err
}

// networkArg maps a spec network to a docker --network value.
func networkArg(network string) string {
	switch network {
	case "internal":
		return internalNetwork
	case "none", "":
		return "none"
	default:
		return "none"
	}
}

// dockerMemory converts a spec memory string ("256Mi", "1Gi") to a docker
// --memory value ("256m", "1g"); docker's m/g suffixes are 1024-based.
func dockerMemory(mem string) string {
	mem = strings.ReplaceAll(mem, "Mi", "m")
	mem = strings.ReplaceAll(mem, "Gi", "g")
	return mem
}
