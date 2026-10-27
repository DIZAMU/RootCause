package cli

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestParseServe(t *testing.T) {
	cases := []struct {
		args      []string
		port      int
		listenAll bool
		ok        bool
	}{
		{nil, 7300, false, true},
		{[]string{"--port", "8080"}, 8080, false, true},
		{[]string{"--listen-all"}, 7300, true, true},
		{[]string{"--listen-all", "--port", "8080"}, 8080, true, true},
		{[]string{"--port", "0"}, 0, false, false},
		{[]string{"--port"}, 0, false, false},
		{[]string{"--bind", "0.0.0.0"}, 0, false, false},
	}
	for _, c := range cases {
		port, all, ok := parseServe(c.args)
		if ok != c.ok || (ok && (port != c.port || all != c.listenAll)) {
			t.Errorf("parseServe(%q) = %d, %v, %v; want %d, %v, %v", c.args, port, all, ok, c.port, c.listenAll, c.ok)
		}
	}
}

func TestListenAllNeedsKubernetes(t *testing.T) {
	var ue *userError
	if err := checkListenAll(true, "docker"); !errors.As(err, &ue) || ue.key != "serve.listen_all_refused" {
		t.Errorf("--listen-all with docker: %v, want serve.listen_all_refused", err)
	}
	if err := checkListenAll(true, "kubernetes"); err != nil {
		t.Errorf("--listen-all with kubernetes: %v", err)
	}
	if err := checkListenAll(false, "docker"); err != nil {
		t.Errorf("no --listen-all: %v", err)
	}
}

func TestRuntimeName(t *testing.T) {
	for in, want := range map[string]string{"": "docker", "docker": "docker", "kubernetes": "kubernetes"} {
		t.Setenv(EnvRuntime, in)
		if got, err := RuntimeName(); err != nil || got != want {
			t.Errorf("RC_RUNTIME=%q: %q, %v; want %q", in, got, err, want)
		}
	}
	t.Setenv(EnvRuntime, "podman")
	if _, err := RuntimeName(); err == nil {
		t.Error("RC_RUNTIME=podman accepted")
	}
}

// openStore keeps trying an unreachable database for RC_DB_WAIT_SECONDS,
// then fails; without the variable it fails on the first try.
func TestOpenStoreWaitIsBounded(t *testing.T) {
	const dsn = "postgres://nobody:nothing@127.0.0.1:1/none?sslmode=disable&connect_timeout=1"
	ctx := context.Background()
	for _, c := range []struct {
		wait     string
		min, max time.Duration
	}{{"", 0, time.Second}, {"2", 2 * time.Second, 5 * time.Second}} {
		t.Setenv(EnvDBWait, c.wait)
		start := time.Now()
		if _, err := openStore(ctx, dsn); err == nil {
			t.Fatalf("RC_DB_WAIT_SECONDS=%q: opened an unreachable database", c.wait)
		}
		if d := time.Since(start); d < c.min || d > c.max {
			t.Errorf("RC_DB_WAIT_SECONDS=%q: gave up after %s, want %s to %s", c.wait, d, c.min, c.max)
		}
	}
}
