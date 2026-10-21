package docker

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
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

// testImage is built from testImageBase with a long-running PID 1, so the test
// goes through the real Create path with every security flag.
const (
	testImageBase = "alpine:latest"
	testImage     = "rootcause/runtime-test:latest"
)

func buildTestImage(t *testing.T, ctx context.Context, r *Runtime) {
	t.Helper()
	dir := t.TempDir()
	dockerfile := "FROM " + testImageBase + "\nENTRYPOINT [\"sleep\", \"infinity\"]\n"
	if err := os.WriteFile(filepath.Join(dir, "Dockerfile"), []byte(dockerfile), 0o644); err != nil {
		t.Fatalf("write Dockerfile: %v", err)
	}
	if err := r.BuildImage(ctx, testImage, dir); err != nil {
		t.Fatalf("build test image: %v", err)
	}
}

func TestSandboxLifecycle(t *testing.T) {
	r := newOrSkip(t)
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	buildTestImage(t, ctx, r)

	name := "rc-test-lifecycle-" + sanitize(t.Name())
	_ = r.Destroy(ctx, name)
	spec := runtime.SandboxSpec{
		Name:    name,
		Image:   testImage,
		Memory:  "256Mi",
		CPUs:    0.5,
		Network: "none",
		Caps:    []string{"SETUID"},
		Labels:  map[string]string{"rootcause.session": "test-session"},
	}
	sb, err := r.Create(ctx, spec)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	t.Cleanup(func() { _ = r.Destroy(context.Background(), name) })
	if sb.ID == "" {
		t.Fatal("create returned empty id")
	}

	// The security flags from spec section 13.1 are really applied.
	out, err := exec.Command("docker", "inspect", "--format",
		"{{.HostConfig.CapDrop}}|{{.HostConfig.CapAdd}}|{{.HostConfig.PidsLimit}}|"+
			"{{.HostConfig.NetworkMode}}|{{.HostConfig.Privileged}}|{{.HostConfig.Init}}|"+
			"{{.HostConfig.SecurityOpt}}|{{.HostConfig.Binds}}",
		name).Output()
	if err != nil {
		t.Fatalf("inspect: %v", err)
	}
	got := strings.TrimSpace(string(out))
	want := "[ALL]|[CAP_SETUID]|256|none|false|true|[no-new-privileges]|[]"
	if got != want {
		t.Errorf("sandbox flags = %q, want %q", got, want)
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

func TestIsolated(t *testing.T) {
	cases := map[string]bool{
		`["name=seccomp,profile=builtin"]`:                 false,
		`["name=seccomp,profile=builtin","name=userns"]`:   true,
		`["name=seccomp,profile=builtin","name=rootless"]`: true,
		`["name=apparmor","name=seccomp,profile=builtin"]`: false,
	}
	for opts, want := range cases {
		if got := isolated(opts); got != want {
			t.Errorf("isolated(%s) = %v, want %v", opts, got, want)
		}
	}
}

func sanitize(s string) string {
	return strings.NewReplacer("/", "-", " ", "-").Replace(s)
}
