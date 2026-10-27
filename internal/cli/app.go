// Package cli implements the rootcause command-line interface (spec section
// 10). It is a thin layer over the session engine and the store; all
// user-facing text comes from the i18n bundle.
package cli

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/DIZAMU/RootCause/internal/i18n"
	"github.com/DIZAMU/RootCause/internal/play"
	"github.com/DIZAMU/RootCause/internal/session"
	"github.com/DIZAMU/RootCause/internal/store"
	"github.com/DIZAMU/RootCause/locales"
)

// App holds everything a command needs.
type App struct {
	root    string
	dsn     string // the database the store uses; dbt builds against it
	st      *store.Store
	eng     *session.Engine
	runtime string // the runtime RC_RUNTIME selects
	bundle  *i18n.Bundle
	learner store.Learner
	out     *os.File
}

// NewApp builds the app for commands that need the database. It assumes
// `rootcause init` has already brought Postgres up; if the database is
// unreachable it returns a clear "run init" error. Every error it returns is
// already translated: into English until the profile is loaded, then into the
// learner's language.
func NewApp(ctx context.Context) (*App, error) {
	bundle, err := i18n.Load(locales.FS)
	if err != nil {
		return nil, err
	}
	lang := defaultLang()
	failed := func(err error) (*App, error) {
		return nil, errors.New(Message(bundle, lang, err))
	}

	root, err := findRoot()
	if err != nil {
		return failed(err)
	}
	dsn, err := resolveDSN(root)
	if err != nil {
		return failed(err)
	}
	st, err := openStore(ctx, dsn)
	if err != nil {
		return failed(fail("error.needs_init", nil))
	}
	if err := st.Migrate(ctx); err != nil {
		st.Close()
		return failed(err)
	}
	if err := syncExercises(ctx, st, root); err != nil {
		st.Close()
		return failed(err)
	}
	learner, err := st.EnsureLearner(ctx, defaultName())
	if err != nil {
		st.Close()
		return failed(err)
	}
	lang = learner.Lang
	be, err := NewBackend(ctx)
	if err != nil {
		st.Close()
		return failed(err)
	}
	rtName, _ := RuntimeName()
	eng := session.New(be.Runtime, be.Builder, be.Sheller, st, filepath.Join(root, "exercises"))

	// Expire any sessions past their TTL before running the command. v1 has no
	// long-running daemon, so this lazy pass keeps sandboxes from lingering.
	// Best-effort: a reap error must not block the command.
	_, _ = session.NewReaper(eng, st, 0).ReapOnce(ctx)

	return &App{root: root, dsn: dsn, st: st, eng: eng, runtime: rtName, bundle: bundle, learner: learner, out: os.Stdout}, nil
}

// EnvDBWait is how many seconds NewApp keeps trying to reach the database
// before it gives up (default 0: one try). The cluster deployment sets it,
// because a new pod's network policy is in place only a moment after the pod
// starts.
const EnvDBWait = "RC_DB_WAIT_SECONDS"

// openStore opens the store, polling for up to RC_DB_WAIT_SECONDS until the
// database answers.
func openStore(ctx context.Context, dsn string) (*store.Store, error) {
	wait, err := strconv.Atoi(os.Getenv(EnvDBWait))
	if err != nil || wait < 0 {
		wait = 0
	}
	deadline := time.Now().Add(time.Duration(wait) * time.Second)
	for {
		st, err := store.Open(ctx, dsn)
		if err == nil || !time.Now().Before(deadline) {
			return st, err
		}
		select {
		case <-ctx.Done():
			return nil, err
		case <-time.After(500 * time.Millisecond):
		}
	}
}

// Close releases resources.
func (a *App) Close() {
	if a.st != nil {
		a.st.Close()
	}
}

// lang is the learner's current interface language.
func (a *App) lang() string { return a.learner.Lang }

// t translates a key in the learner's language.
func (a *App) t(key string, vars map[string]string) string {
	return a.bundle.T(a.lang(), key, vars)
}

// say prints a translated line.
func (a *App) say(key string, vars map[string]string) {
	fmt.Fprintln(a.out, a.t(key, vars))
}

// Usage returns the help text in lang. With no profile loaded yet, the CLI
// passes English.
func Usage(lang string) string {
	b, err := i18n.Load(locales.FS)
	if err != nil {
		return err.Error()
	}
	return b.T(lang, "help.usage", nil)
}

// UnknownCommand returns the help text for an unknown command, in the
// learner's language.
func (a *App) UnknownCommand(cmd string) string {
	return a.t("help.unknown_command", map[string]string{"command": cmd}) + "\n\n" + a.t("help.usage", nil)
}

// Describe renders a command's error for the learner, in their language.
func (a *App) Describe(err error) string { return Message(a.bundle, a.lang(), err) }

// Init brings Postgres up, migrates, syncs exercises and ensures the profile.
// It returns the number of exercises synced. Errors are already translated
// (into English: there may be no profile yet).
func Init(ctx context.Context) (string, int, error) {
	bundle, err := i18n.Load(locales.FS)
	if err != nil {
		return "", 0, err
	}
	failed := func(err error) (string, int, error) {
		return "", 0, errors.New(Message(bundle, defaultLang(), err))
	}

	root, err := findRoot()
	if err != nil {
		return failed(err)
	}
	dsn, err := resolveDSN(root)
	if err != nil {
		return failed(err)
	}
	if err := composeUp(ctx, root); err != nil {
		return failed(err)
	}
	st, err := store.Open(ctx, dsn)
	if err != nil {
		return failed(fail("error.db_unreachable", err))
	}
	defer st.Close()
	if err := st.Migrate(ctx); err != nil {
		return failed(err)
	}
	if err := syncExercises(ctx, st, root); err != nil {
		return failed(err)
	}
	learner, err := st.EnsureLearner(ctx, defaultName())
	if err != nil {
		return failed(err)
	}
	rows, err := st.Exercises(ctx)
	if err != nil {
		return failed(err)
	}
	return bundle.T(learner.Lang, "init.done", map[string]string{"count": fmt.Sprint(len(rows))}), len(rows), nil
}

// --- configuration helpers -------------------------------------------------

// findRoot locates the project root (the folder holding go.mod), honoring
// RC_ROOT if set, so the CLI can be run from anywhere inside the repo.
func findRoot() (string, error) {
	if r := os.Getenv("RC_ROOT"); r != "" {
		return r, nil
	}
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if fileExists(filepath.Join(dir, "go.mod")) {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fail("error.no_root", nil)
		}
		dir = parent
	}
}

// resolveDSN returns the database DSN from RC_DATABASE_URL, or builds it from
// the git-ignored deploy/compose/.env file.
func resolveDSN(root string) (string, error) {
	if v := os.Getenv("RC_DATABASE_URL"); v != "" {
		return v, nil
	}
	env, err := readEnvFile(filepath.Join(root, "deploy", "compose", ".env"))
	if err != nil {
		return "", fail("error.no_env", err)
	}
	user, db, pw := env["POSTGRES_USER"], env["POSTGRES_DB"], env["POSTGRES_PASSWORD"]
	if user == "" || db == "" || pw == "" {
		return "", fail("error.env_incomplete", nil)
	}
	return fmt.Sprintf("postgres://%s:%s@127.0.0.1:5432/%s?sslmode=disable", user, pw, db), nil
}

func readEnvFile(path string) (map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	env := make(map[string]string)
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if ok {
			env[strings.TrimSpace(k)] = strings.TrimSpace(v)
		}
	}
	return env, sc.Err()
}

// composeUp starts Postgres via Compose and waits until it is healthy.
func composeUp(ctx context.Context, root string) error {
	file := filepath.Join(root, "deploy", "compose", "docker-compose.yml")
	envFile := filepath.Join(root, "deploy", "compose", ".env")
	cmd := exec.CommandContext(ctx, "docker", "compose", "-f", file, "--env-file", envFile, "up", "-d")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fail("error.compose_failed", fmt.Errorf("docker compose up: %w: %s", err, strings.TrimSpace(string(out))))
	}
	// Wait for the health check to pass.
	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		out, _ := exec.CommandContext(ctx, "docker", "inspect",
			"--format", "{{.State.Health.Status}}", "rootcause-postgres-1").Output()
		if strings.TrimSpace(string(out)) == "healthy" {
			return nil
		}
		time.Sleep(time.Second)
	}
	return fail("error.postgres_unhealthy", nil)
}

// syncExercises loads every exercise and upserts the catalog.
func syncExercises(ctx context.Context, st *store.Store, root string) error {
	rows, err := loadAllExercises(root)
	if err != nil {
		return err
	}
	return st.SyncExercises(ctx, rows)
}

// loadAllExercises loads the catalog from the exercises folder.
func loadAllExercises(root string) ([]store.ExerciseRow, error) {
	rows, err := play.Catalog(filepath.Join(root, "exercises"))
	if errors.Is(err, play.ErrNoExercises) {
		return nil, fail("error.no_exercises", err)
	}
	return rows, err
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// defaultName is the local profile name; the OS user, or "learner".
func defaultName() string {
	if u := os.Getenv("USER"); u != "" {
		return u
	}
	return "learner"
}

// defaultLang is used only before a learner row is loaded.
func defaultLang() string { return "en" }
