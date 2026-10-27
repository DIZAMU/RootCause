// Package dbt runs the dbt project (spec section 9.2) in a container, against
// the database the engine uses. The project is embedded in the binary
// (package dbtproject) and extracted into a fresh directory for each build, so
// a build never reads the working tree. The container is locked down like a
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
	"io/fs"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	dbtproject "github.com/DIZAMU/RootCause/dbt"
	"github.com/jackc/pgx/v5/pgconn"
	"go.yaml.in/yaml/v3"
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

// Labels on every dbt container. Before a build, containers left by an
// earlier build of the same database (a killed engine) are removed.
const (
	labelDBT      = "rootcause.dbt"
	labelDatabase = "rootcause.database"
)

// Environment variables that point dbt at a Postgres container outside the
// compose project: the docker network to join and the host name on it. Only
// tests and CI set them (dbt.yml runs Postgres as a service container).
const (
	envNetwork = "RC_DBT_NETWORK"
	envHost    = "RC_DBT_HOST"
)

// outputTail is how many of dbt's last output lines an error includes.
const outputTail = 30

// keepLogs is how many saved dbt.log files of failed builds are kept.
const keepLogs = 20

// Build runs `dbt build` (models and tests) for the embedded project against
// the database in dsn. It returns nil only when dbt exits 0 and ran every
// node the project defines. On any failure the full dbt.log is saved under
// the user cache directory, and the error ends with dbt's last output lines
// and the log's path.
//
// Callers hold the statistics lock (store.WithStatsLock), so one build runs
// at a time per database. Build relies on that when it removes the
// containers of earlier builds of the same database.
func Build(ctx context.Context, dsn string) error {
	cfg, err := pgconn.ParseConfig(dsn)
	if err != nil {
		return fmt.Errorf("parse database DSN: %w", err)
	}
	if !isLocal(cfg.Host) || cfg.Port != 5432 {
		return ErrUnsupportedDatabase
	}
	network, host, err := target(ctx)
	if err != nil {
		return err
	}
	want, err := ExpectedNodes(dbtproject.FS)
	if err != nil {
		return fmt.Errorf("count the project's nodes: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, Timeout)
	defer cancel()

	if err := removeStale(ctx, cfg.Database); err != nil {
		return err
	}
	removeOldExtracts()
	project, err := extract(dbtproject.FS)
	if err != nil {
		return fmt.Errorf("extract the dbt project: %w", err)
	}
	defer os.RemoveAll(project)

	name := "rootcause-dbt-" + randomSuffix()
	env := map[string]string{
		"DBT_HOST":     host,
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
	// No --rm: after a failure, dbt.log is copied out before the container
	// is removed. /tmp/logs is an anonymous volume, not part of the /tmp
	// tmpfs, because a tmpfs is emptied when the container stops.
	args := []string{
		"run", "--name", name,
		"--label", labelDBT + "=true",
		"--label", labelDatabase + "=" + cfg.Database,
		"--network", network,
		"--cap-drop", "ALL",
		"--security-opt", "no-new-privileges",
		"--pids-limit", "256",
		"--memory", "1g",
		"--read-only",
		"--tmpfs", "/tmp",
		"--mount", "type=volume,dst=/tmp/logs",
		"--mount", "type=bind,src=" + project + ",dst=/usr/app/dbt,readonly",
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
	runErr := cmd.Run()

	failure := verdict(ctx.Err(), runErr, out.String(), want)
	if failure == "" {
		removeContainer(name)
		return nil
	}
	logPath, logErr := saveLog(name, cfg.Database, out.Bytes())
	removeContainer(name)
	msg := failure + ":\n" + tail(out.String(), outputTail)
	if logErr != nil {
		return fmt.Errorf("%s\n(dbt.log not saved: %v)", msg, logErr)
	}
	return fmt.Errorf("%s\nfull log: %s", msg, logPath)
}

// verdict says why a finished build failed, or returns "" when it passed:
// the context still live, dbt exited 0, and its summary counts exactly want
// nodes. A build that exits 0 but ran fewer nodes, for example because dbt
// parsed a file caught mid-save, is a failure.
func verdict(ctxErr, runErr error, output string, want int) string {
	switch {
	case errors.Is(ctxErr, context.Canceled):
		return "dbt build cancelled"
	case ctxErr != nil:
		return fmt.Sprintf("dbt build did not finish within %s", Timeout)
	case runErr != nil:
		var ee *exec.ExitError
		if !errors.As(runErr, &ee) {
			return fmt.Sprintf("run dbt container: %v", runErr)
		}
		return fmt.Sprintf("dbt build exited %d", ee.ExitCode())
	}
	got, ok := executedNodes(output)
	if !ok {
		return "dbt build printed no node total"
	}
	if got != want {
		return fmt.Sprintf("dbt build ran %d nodes, the project has %d", got, want)
	}
	return ""
}

// target returns the docker network the dbt container joins and the
// Postgres host name on it: the compose network and service by default, or
// RC_DBT_NETWORK and RC_DBT_HOST when both are set (tests and CI only).
func target(ctx context.Context) (network, host string, err error) {
	network, host = os.Getenv(envNetwork), os.Getenv(envHost)
	switch {
	case network == "" && host == "":
		network, err = composeNetwork(ctx)
		return network, composeService, err
	case network == "" || host == "":
		return "", "", fmt.Errorf("set both %s and %s, or neither", envNetwork, envHost)
	case network == "host" || network == "none" || strings.HasPrefix(network, "container:"):
		return "", "", fmt.Errorf("%s=%s: dbt never uses the host network, and needs a network to reach Postgres", envNetwork, network)
	}
	return network, host, nil
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

// removeStale removes every dbt container of database db, running or not,
// with its log volume. Such a container is left only when the process that
// started it was killed: Build removes its own container on every path.
func removeStale(ctx context.Context, db string) error {
	out, err := exec.CommandContext(ctx, "docker", "ps", "-aq",
		"--filter", "label="+labelDBT+"=true",
		"--filter", "label="+labelDatabase+"="+db).Output()
	if err != nil {
		return fmt.Errorf("list stale dbt containers: %w", err)
	}
	ids := strings.Fields(string(out))
	if len(ids) == 0 {
		return nil
	}
	if out, err := exec.CommandContext(ctx, "docker", append([]string{"rm", "-f", "-v"}, ids...)...).CombinedOutput(); err != nil {
		return fmt.Errorf("remove stale dbt containers: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// removeContainer removes a build's container and its log volume. It runs
// with its own context, because the build's context may already be over.
func removeContainer(name string) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	_ = exec.CommandContext(ctx, "docker", "rm", "-f", "-v", name).Run()
}

// extractPrefix starts the name of every extracted project directory.
const extractPrefix = "rootcause-dbt-"

// removeOldExtracts deletes extracted project directories that a killed
// build left behind. A build lasts at most Timeout, so a directory older than
// twice that belongs to no running build, whatever its database.
func removeOldExtracts() {
	matches, _ := filepath.Glob(filepath.Join(os.TempDir(), extractPrefix+"*"))
	for _, m := range matches {
		if fi, err := os.Stat(m); err == nil && fi.IsDir() && time.Since(fi.ModTime()) > 2*Timeout {
			_ = os.RemoveAll(m)
		}
	}
}

// extract writes the project in fsys to a new temporary directory and returns
// its path. Directories are 0755 and files 0644: under userns-remap the
// container's root is an unprivileged host user, which reads them as "other".
func extract(fsys fs.FS) (string, error) {
	dir, err := os.MkdirTemp("", extractPrefix)
	if err != nil {
		return "", err
	}
	err = writeTree(fsys, dir)
	if err == nil {
		err = os.Chmod(dir, 0o755) // MkdirTemp creates it 0700
	}
	if err != nil {
		os.RemoveAll(dir)
		return "", err
	}
	return dir, nil
}

// WriteProject writes the embedded dbt project into dir, which must exist.
// The cluster's dbt CronJob runs it from there (spec section 8.2).
func WriteProject(dir string) error {
	return writeTree(dbtproject.FS, dir)
}

// writeTree copies fsys into dir: directories 0755, files 0644.
func writeTree(fsys fs.FS, dir string) error {
	return fs.WalkDir(fsys, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		dst := filepath.Join(dir, filepath.FromSlash(p))
		if d.IsDir() {
			return os.MkdirAll(dst, 0o755)
		}
		data, err := fs.ReadFile(fsys, p)
		if err != nil {
			return err
		}
		return os.WriteFile(dst, data, 0o644)
	})
}

// LogDir is where the dbt.log of failed builds is saved.
func LogDir() (string, error) {
	cache, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(cache, "rootcause", "dbt-logs"), nil
}

// saveLog copies dbt.log out of the container into LogDir and returns its
// path. If the container holds no dbt.log (dbt never started), the docker
// output is saved instead. Only the newest keepLogs files are kept.
func saveLog(name, db string, output []byte) (string, error) {
	dir, err := LogDir()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	dst := filepath.Join(dir, time.Now().UTC().Format("20060102T150405Z")+"-"+db+"-"+strings.TrimPrefix(name, "rootcause-dbt-")+".log")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := exec.CommandContext(ctx, "docker", "cp", name+":/tmp/logs/dbt.log", dst).Run(); err != nil {
		if err := os.WriteFile(dst, output, 0o600); err != nil {
			return "", err
		}
	}
	pruneLogs(dir)
	return dst, nil
}

// pruneLogs removes all but the newest keepLogs files in dir. Names start
// with a UTC timestamp, so name order is age order.
func pruneLogs(dir string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".log") {
			names = append(names, e.Name())
		}
	}
	slices.Sort(names)
	for len(names) > keepLogs {
		_ = os.Remove(filepath.Join(dir, names[0]))
		names = names[1:]
	}
}

// doneLine matches dbt's summary, for example
// "Done. PASS=55 WARN=0 ERROR=0 SKIP=0 NO-OP=0 TOTAL=55".
var doneLine = regexp.MustCompile(`(?m)Done\. .*\bTOTAL=(\d+)`)

// executedNodes returns the TOTAL from dbt's last summary line in output.
func executedNodes(output string) (int, bool) {
	m := doneLine.FindAllStringSubmatch(output, -1)
	if m == nil {
		return 0, false
	}
	n, err := strconv.Atoi(m[len(m)-1][1])
	return n, err == nil
}

// ExpectedNodes counts the nodes `dbt build` runs for the project in fsys:
// one per model (.sql under models/), one per singular test (.sql under
// tests/) and one per generic test listed in a properties file (.yml under
// models/), on a model, a source table or one of their columns. It reads the
// files itself, so a dbt run that silently skips nodes shows up as a
// mismatch.
func ExpectedNodes(fsys fs.FS) (int, error) {
	n := 0
	err := fs.WalkDir(fsys, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		inModels, inTests := strings.HasPrefix(p, "models/"), strings.HasPrefix(p, "tests/")
		switch ext := path.Ext(p); {
		case ext == ".sql" && (inModels || inTests):
			n++
		case (ext == ".yml" || ext == ".yaml") && inModels:
			data, err := fs.ReadFile(fsys, p)
			if err != nil {
				return err
			}
			tests, err := genericTests(data)
			if err != nil {
				return fmt.Errorf("%s: %w", p, err)
			}
			n += tests
		}
		return nil
	})
	return n, err
}

// testList is a node's generic tests. dbt accepts both keys.
type testList struct {
	Tests     []yaml.Node `yaml:"tests"`
	DataTests []yaml.Node `yaml:"data_tests"`
}

type testedNode struct {
	testList `yaml:",inline"`
	Columns  []testList `yaml:"columns"`
}

func (t testedNode) count() int {
	n := len(t.Tests) + len(t.DataTests)
	for _, c := range t.Columns {
		n += len(c.Tests) + len(c.DataTests)
	}
	return n
}

// genericTests counts the generic tests in one properties file.
func genericTests(data []byte) (int, error) {
	var f struct {
		Models  []testedNode `yaml:"models"`
		Sources []struct {
			Tables []testedNode `yaml:"tables"`
		} `yaml:"sources"`
	}
	if err := yaml.Unmarshal(data, &f); err != nil {
		return 0, err
	}
	n := 0
	for _, m := range f.Models {
		n += m.count()
	}
	for _, s := range f.Sources {
		for _, t := range s.Tables {
			n += t.count()
		}
	}
	return n, nil
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
