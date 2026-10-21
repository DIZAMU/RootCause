// Package cli implements the rootcause command-line interface (spec section
// 10). It is a thin layer over the session engine and the store; all
// user-facing text comes from the i18n bundle.
package cli

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/DIZAMU/RootCause/internal/exercise"
	"github.com/DIZAMU/RootCause/internal/i18n"
	"github.com/DIZAMU/RootCause/internal/runtime/docker"
	"github.com/DIZAMU/RootCause/internal/session"
	"github.com/DIZAMU/RootCause/internal/store"
)

// App holds everything a command needs.
type App struct {
	root    string
	st      *store.Store
	eng     *session.Engine
	rt      *docker.Runtime
	bundle  *i18n.Bundle
	learner store.Learner
	out     *os.File
}

// NewApp builds the app for commands that need the database. It assumes
// `rootcause init` has already brought Postgres up; if the database is
// unreachable it returns a clear "run init" error.
func NewApp(ctx context.Context) (*App, error) {
	root, err := findRoot()
	if err != nil {
		return nil, err
	}
	bundle, err := i18n.Load(filepath.Join(root, "locales"))
	if err != nil {
		return nil, err
	}
	dsn, err := resolveDSN(root)
	if err != nil {
		return nil, err
	}
	st, err := store.Open(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("%s", bundle.T(defaultLang(), "error.needs_init", nil))
	}
	if err := st.Migrate(ctx); err != nil {
		return nil, err
	}
	if err := syncExercises(ctx, st, root); err != nil {
		return nil, err
	}
	learner, err := st.EnsureLearner(ctx, defaultName())
	if err != nil {
		return nil, err
	}
	rt, err := docker.New()
	if err != nil {
		return nil, err
	}
	eng := session.New(rt, rt, rt, st, filepath.Join(root, "exercises"))

	// Expire any sessions past their TTL before running the command. v1 has no
	// long-running daemon, so this lazy pass keeps sandboxes from lingering.
	// Best-effort: a reap error must not block the command.
	_, _ = session.NewReaper(eng, st, 0).ReapOnce(ctx)

	return &App{root: root, st: st, eng: eng, rt: rt, bundle: bundle, learner: learner, out: os.Stdout}, nil
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

// Init brings Postgres up, migrates, syncs exercises and ensures the profile.
// It returns the number of exercises synced.
func Init(ctx context.Context) (string, int, error) {
	root, err := findRoot()
	if err != nil {
		return "", 0, err
	}
	bundle, err := i18n.Load(filepath.Join(root, "locales"))
	if err != nil {
		return "", 0, err
	}
	if err := composeUp(ctx, root); err != nil {
		return "", 0, err
	}
	dsn, err := resolveDSN(root)
	if err != nil {
		return "", 0, err
	}
	st, err := store.Open(ctx, dsn)
	if err != nil {
		return "", 0, err
	}
	defer st.Close()
	if err := st.Migrate(ctx); err != nil {
		return "", 0, err
	}
	if err := syncExercises(ctx, st, root); err != nil {
		return "", 0, err
	}
	if _, err := st.EnsureLearner(ctx, defaultName()); err != nil {
		return "", 0, err
	}
	rows, _ := st.Exercises(ctx)
	learner, _ := st.EnsureLearner(ctx, defaultName())
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
			return "", fmt.Errorf("project root not found: run from inside the repo, or set RC_ROOT")
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
		return "", fmt.Errorf("no RC_DATABASE_URL and cannot read deploy/compose/.env: %w", err)
	}
	user, db, pw := env["POSTGRES_USER"], env["POSTGRES_DB"], env["POSTGRES_PASSWORD"]
	if user == "" || db == "" || pw == "" {
		return "", fmt.Errorf("deploy/compose/.env is missing POSTGRES_USER/DB/PASSWORD")
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
		return fmt.Errorf("docker compose up: %w: %s", err, strings.TrimSpace(string(out)))
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
	return fmt.Errorf("postgres did not become healthy in time")
}

// syncExercises loads every exercise and upserts the catalog.
func syncExercises(ctx context.Context, st *store.Store, root string) error {
	rows, err := loadAllExercises(root)
	if err != nil {
		return err
	}
	return st.SyncExercises(ctx, rows)
}

func loadAllExercises(root string) ([]store.ExerciseRow, error) {
	exRoot := filepath.Join(root, "exercises")
	entries, err := os.ReadDir(exRoot)
	if err != nil {
		return nil, fmt.Errorf("read exercises dir: %w", err)
	}
	var rows []store.ExerciseRow
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		ex, err := exercise.Load(filepath.Join(exRoot, e.Name()))
		if err != nil {
			return nil, err
		}
		rows = append(rows, store.ExerciseRow{
			ID: ex.ID, Version: ex.Version, Topic: ex.Topic, Difficulty: ex.Difficulty,
		})
	}
	return rows, nil
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
