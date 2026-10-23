// Package dbt runs the dbt project in dbt/ (spec section 9.2) in a container,
// against the database the engine uses. The container is locked down like a
// sandbox: no capabilities, a read-only root filesystem, the project mounted
// read-only, and the compose network only (never the host network). The
// password reaches the container only through its environment.
package dbt

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
)

// Image is the dbt Core image with the Postgres adapter (spec section 2),
// pinned by digest.
const Image = "ghcr.io/dbt-labs/dbt-postgres:1.9.0@sha256:37624cce09877ff03fd5f6b01355fdf5a3462a1b651d40d8b8edc1cf056eedf0"

// Timeout bounds one dbt build (spec section 9.3).
const Timeout = 120 * time.Second

// ErrUnsupportedDatabase means the DSN does not point at the compose Postgres,
// the only database the dbt container can reach without the host network.
var ErrUnsupportedDatabase = errors.New("dbt runs only against the compose Postgres on 127.0.0.1:5432")

// compose labels that identify the Postgres container from deploy/compose.
const (
	composeProject = "rootcause"
	composeService = "postgres"
)

// outputTail is how many of dbt's last output lines an error includes.
const outputTail = 30

// Build runs `dbt build` (models and tests) for the project in projectDir
// against the database in dsn. It returns nil on success. dbt's exit 1 (a
// model or test failed) and exit 2 (dbt could not run), and any other exit,
// are errors that end with dbt's last output lines.
func Build(ctx context.Context, projectDir, dsn string) error {
	cfg, err := pgconn.ParseConfig(dsn)
	if err != nil {
		return fmt.Errorf("parse database DSN: %w", err)
	}
	if !isLocal(cfg.Host) || cfg.Port != 5432 {
		return ErrUnsupportedDatabase
	}
	network, err := composeNetwork(ctx)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(ctx, Timeout)
	defer cancel()

	name := "rootcause-dbt-" + randomSuffix()
	env := map[string]string{
		"DBT_HOST":     composeService,
		"DBT_PORT":     "5432",
		"DBT_USER":     cfg.User,
		"DBT_PASSWORD": cfg.Password,
		"DBT_DBNAME":   cfg.Database,
		// Keep the read-only project untouched: targets and logs go to /tmp.
		"DBT_PROFILES_DIR": "/usr/app/dbt",
		"DBT_TARGET_PATH":  "/tmp/target",
		"DBT_LOG_PATH":     "/tmp/logs",
		"HOME":             "/tmp",
		// No network calls besides Postgres.
		"DBT_SEND_ANONYMOUS_USAGE_STATS": "false",
		"DBT_VERSION_CHECK":              "false",
	}
	args := []string{
		"run", "--rm", "--name", name,
		"--network", network,
		"--cap-drop", "ALL",
		"--security-opt", "no-new-privileges",
		"--pids-limit", "256",
		"--memory", "1g",
		"--read-only",
		"--tmpfs", "/tmp",
		"--mount", "type=bind,src=" + projectDir + ",dst=/usr/app/dbt,readonly",
	}
	// -e NAME with no value: docker reads the value from its own environment,
	// so no value, the password included, appears on a command line.
	for k := range env {
		args = append(args, "-e", k)
	}
	args = append(args, Image, "build")

	cmd := exec.CommandContext(ctx, "docker", args...)
	cmd.Env = os.Environ()
	for k, v := range env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	err = cmd.Run()

	// A killed docker client leaves its container running: remove it.
	if ctx.Err() != nil {
		_ = exec.Command("docker", "rm", "-f", name).Run()
		return fmt.Errorf("dbt build did not finish within %s:\n%s", Timeout, tail(out.String(), outputTail))
	}
	if err == nil {
		return nil
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return fmt.Errorf("dbt build exited %d:\n%s", ee.ExitCode(), tail(out.String(), outputTail))
	}
	return fmt.Errorf("run dbt container: %w", err)
}

// isLocal reports whether host is this machine's loopback address.
func isLocal(host string) bool {
	return host == "127.0.0.1" || host == "localhost" || host == "::1"
}

// composeNetwork returns the network of the running compose Postgres
// container, which the dbt container joins to reach it by service name.
func composeNetwork(ctx context.Context) (string, error) {
	out, err := exec.CommandContext(ctx, "docker", "ps",
		"--filter", "label=com.docker.compose.project="+composeProject,
		"--filter", "label=com.docker.compose.service="+composeService,
		"--format", `{{.Networks}}`).Output()
	if err != nil {
		return "", fmt.Errorf("find the compose Postgres: %w", err)
	}
	lines := strings.Fields(strings.TrimSpace(string(out)))
	if len(lines) == 0 {
		return "", fmt.Errorf("%w: the compose Postgres is not running (run: rootcause init)", ErrUnsupportedDatabase)
	}
	// One container; .Networks lists its networks separated by commas.
	network, _, _ := strings.Cut(lines[0], ",")
	return network, nil
}

// tail returns the last n lines of s.
func tail(s string, n int) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
}

func randomSuffix() string {
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
