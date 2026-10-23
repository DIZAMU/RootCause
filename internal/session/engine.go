// Package session runs the exercise lifecycle (spec section 8.4): it creates a
// sandbox from an exercise, plants the fault, grades checks, and logs every
// action as an event. It wires the exercise loader, the runtime and the store
// together and is the engine the CLI drives.
package session

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/DIZAMU/RootCause/internal/exercise"
	"github.com/DIZAMU/RootCause/internal/grading"
	"github.com/DIZAMU/RootCause/internal/runtime"
	"github.com/DIZAMU/RootCause/internal/store"
)

// Errors the CLI turns into translated messages (spec section 12).
var (
	// ErrGiveUpTooEarly means give up was asked for before hint 3.
	ErrGiveUpTooEarly = errors.New("give up is only available after hint 3")
	// ErrNoShell means the runtime cannot open an interactive shell.
	ErrNoShell = errors.New("runtime has no interactive shell")
)

// Builder builds an exercise image. The docker runtime implements it.
type Builder interface {
	BuildImage(ctx context.Context, tag, dir string) error
}

// Engine orchestrates sessions.
type Engine struct {
	rt      runtime.Runtime
	builder Builder
	sheller runtime.Sheller
	st      *store.Store
	exDir   string
}

// New builds an Engine. exercisesDir is the folder holding exercise folders.
func New(rt runtime.Runtime, builder Builder, sheller runtime.Sheller, st *store.Store, exercisesDir string) *Engine {
	return &Engine{rt: rt, builder: builder, sheller: sheller, st: st, exDir: exercisesDir}
}

// CheckResult is the outcome of grading a session.
type CheckResult struct {
	Passed   bool
	Feedback string
	Infra    bool         // true if the check could not run (exit 2); the session is voided
	Stars    int          // stars awarded, when Passed
	Level    *LevelChange // set if this result changed the topic level
}

// LevelChange records a topic moving up or down a level.
type LevelChange struct {
	Topic string
	From  int
	To    int
}

// Start creates a session and its sandbox, builds the healthy state, plants the
// fault, and logs session_started. The seed makes the variant and fault
// reproducible.
func (e *Engine) Start(ctx context.Context, learnerID int, exerciseID string, seed int64) (store.Session, error) {
	ex, err := exercise.Load(filepath.Join(e.exDir, exerciseID))
	if err != nil {
		return store.Session{}, err
	}
	if err := e.checkHost(ctx, ex); err != nil {
		return store.Session{}, err
	}

	// Make sure the catalog row exists (FK target for the session).
	if err := e.st.SyncExercises(ctx, []store.ExerciseRow{
		{ID: ex.ID, Version: ex.Version, Topic: ex.Topic, Difficulty: ex.Difficulty},
	}); err != nil {
		return store.Session{}, err
	}

	tag := imageTag(ex)
	if err := e.builder.BuildImage(ctx, tag, ex.Dir()); err != nil {
		return store.Session{}, err
	}

	variant := ex.SelectVariant(seed)
	sessionID := newID()
	if _, err = e.st.CreateSession(ctx, store.NewSession{
		ID:              sessionID,
		LearnerID:       learnerID,
		ExerciseID:      ex.ID,
		ExerciseVersion: ex.Version,
		Difficulty:      ex.Difficulty,
		Seed:            seed,
		VariantValues:   variant,
		TTL:             time.Duration(ex.SessionTTLMin) * time.Minute,
	}); err != nil {
		return store.Session{}, err
	}

	// Once the session row exists, undo it (and any sandbox) if setup fails, so
	// a failed start never leaves a dangling active session or a leaked
	// container. (The reaper is the backstop.)
	var sandboxID string
	defer func() {
		if err != nil {
			if sandboxID != "" {
				_ = e.rt.Destroy(context.Background(), sandboxID)
			}
			_ = e.st.AddEvent(context.Background(), sessionID, "voided",
				map[string]any{"reason": "start_failed", "error": err.Error()})
			_ = e.st.EndSession(context.Background(), sessionID, "voided", nil)
		}
	}()

	sb, err := e.createSandbox(ctx, ex, e.specFor(ex, sessionID))
	if err != nil {
		return store.Session{}, fmt.Errorf("create sandbox: %w", err)
	}
	sandboxID = sb.ID

	if err = e.st.SetSandbox(ctx, sessionID, sandboxID); err != nil {
		return store.Session{}, err
	}

	var derived map[string]any
	if derived, err = e.prepare(ctx, sandboxID, ex, scriptEnv(seed, variant)); err != nil {
		return store.Session{}, err
	}
	if err = e.st.SetDerived(ctx, sessionID, derived); err != nil {
		return store.Session{}, err
	}

	if err = e.st.AddEvent(ctx, sessionID, "session_started", map[string]any{
		"exercise": ex.ID, "seed": seed, "variant": variant,
	}); err != nil {
		return store.Session{}, err
	}

	var started store.Session
	started, err = e.st.SessionByID(ctx, sessionID)
	return started, err
}

// Labels on every sandbox, read by the reaper.
const (
	labelSession  = "rootcause.session"  // the session id
	labelDatabase = "rootcause.database" // the database that owns the session
	labelSelftest = "rootcause.selftest" // "true" on selftest sandboxes
	labelRunner   = "rootcause.runner"   // "true" on check runner containers
)

// specFor builds the sandbox spec for an exercise session.
func (e *Engine) specFor(ex *exercise.Exercise, sessionID string) runtime.SandboxSpec {
	labels := map[string]string{labelSession: sessionID}
	if e.st != nil {
		labels[labelDatabase] = e.st.Database()
	}
	var tmpfs []runtime.Tmpfs
	for _, t := range ex.Tmpfs {
		tmpfs = append(tmpfs, runtime.Tmpfs{Path: t.Path, Size: t.Size})
	}
	return runtime.SandboxSpec{
		Name:    "rootcause-" + sessionID,
		Image:   imageTag(ex),
		Memory:  ex.Resources.Memory,
		CPUs:    ex.Resources.CPUs,
		Network: ex.Network,
		Caps:    ex.Capabilities,
		Labels:  labels,
		Init:    ex.Init,
		Tmpfs:   tmpfs,
	}
}

// checkHost refuses an exercise the host cannot run, before anything is
// built or stored: an init: systemd exercise needs a runtime that confirms
// systemd can boot. A runtime without that capability is not asked.
func (e *Engine) checkHost(ctx context.Context, ex *exercise.Exercise) error {
	if ex.Init != "systemd" {
		return nil
	}
	if sc, ok := e.rt.(runtime.SystemdChecker); ok {
		return sc.CheckSystemd(ctx)
	}
	return nil
}

// notReady builds a readiness-timeout error. If the runtime can describe the
// sandbox, the error ends with that description (state, exit code, last log
// lines). ctx has usually expired by now, so the description gets its own
// short deadline.
func (e *Engine) notReady(sandboxID string, msg string) error {
	d, ok := e.rt.(runtime.Diagnoser)
	if !ok {
		return errors.New(msg)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return fmt.Errorf("%s: %s", msg, d.Diagnose(ctx, sandboxID))
}

// createSandbox creates a sandbox and waits until it is ready for setup.sh,
// so no script runs while the machine is still booting. On a readiness
// failure it destroys the sandbox and returns the error.
func (e *Engine) createSandbox(ctx context.Context, ex *exercise.Exercise, spec runtime.SandboxSpec) (runtime.Sandbox, error) {
	sb, err := e.rt.Create(ctx, spec)
	if err != nil {
		return runtime.Sandbox{}, err
	}
	if err := e.waitReady(ctx, ex, sb.ID, true); err != nil {
		_ = e.rt.Destroy(context.Background(), sb.ID)
		return runtime.Sandbox{}, err
	}
	return sb, nil
}

// prepare turns a fresh sandbox into the broken machine: it runs setup.sh (if
// the exercise has one) to build the healthy state, then break.sh to plant the
// fault, and returns the derived values break.sh printed (spec section 4.3).
// Both scripts are copied in, run as root and deleted, so neither the healthy
// config nor the fault logic stays readable inside the sandbox.
func (e *Engine) prepare(ctx context.Context, sandboxID string, ex *exercise.Exercise, env map[string]string) (map[string]any, error) {
	if _, err := os.Stat(filepath.Join(ex.Dir(), "setup.sh")); err == nil {
		res, err := e.execScript(ctx, sandboxID, ex.Dir(), "setup.sh", env)
		if err != nil {
			return nil, fmt.Errorf("setup: %w", err)
		}
		if res.ExitCode != 0 {
			return nil, fmt.Errorf("setup.sh exited %d: %s", res.ExitCode, strings.TrimSpace(string(res.Stderr)))
		}
	}
	res, err := e.execScript(ctx, sandboxID, ex.Dir(), "break.sh", env)
	if err != nil {
		return nil, fmt.Errorf("plant fault: %w", err)
	}
	if res.ExitCode != 0 {
		return nil, fmt.Errorf("break.sh exited %d: %s", res.ExitCode, strings.TrimSpace(string(res.Stderr)))
	}
	return lastJSONObject(res.Stdout), nil
}

// Check grades the active session through runCheck (restart and runner
// handling), logs check_run, and on a pass marks the session solved and
// destroys the sandbox. Stars/levels are applied in a later step.
func (e *Engine) Check(ctx context.Context, sessionID string) (CheckResult, error) {
	sess, err := e.st.SessionByID(ctx, sessionID)
	if err != nil {
		return CheckResult{}, err
	}
	ex, err := exercise.Load(filepath.Join(e.exDir, sess.ExerciseID))
	if err != nil {
		return CheckResult{}, err
	}
	spec := e.specFor(ex, sessionID)
	res, err := e.runCheck(ctx, ex, spec, sess.SandboxID, scriptEnv(sess.Seed, sess.VariantValues))
	if err != nil {
		return CheckResult{}, fmt.Errorf("run check: %w", err)
	}

	// Exit 0 is a pass and exit 1 a fail (spec section 4.3). Exit 2, or any
	// other code (a crash, an out-of-memory kill), means the check could not
	// run, so it must never count against the learner.
	parsed := parseCheck(res.Stdout)
	result := CheckResult{
		Passed:   res.ExitCode == 0 && parsed.Passed,
		Feedback: parsed.Feedback,
		Infra:    res.ExitCode != 0 && res.ExitCode != 1,
	}

	if err := e.st.AddEvent(ctx, sessionID, "check_run", map[string]any{
		"passed": result.Passed, "feedback": result.Feedback, "exit": res.ExitCode,
	}); err != nil {
		return result, err
	}

	switch {
	case result.Infra:
		// Infrastructure error: void the session, it does not count.
		_ = e.st.AddEvent(ctx, sessionID, "voided", map[string]any{"reason": "check_error"})
		_ = e.st.EndSession(ctx, sessionID, "voided", nil)
		_ = e.rt.Destroy(ctx, sess.SandboxID)
	case result.Passed:
		stars := grading.Stars(sess.HintsRevealed)
		result.Stars = stars
		if err := e.st.AddEvent(ctx, sessionID, "solved", map[string]any{"hints": sess.HintsRevealed, "stars": stars}); err != nil {
			return result, err
		}
		if err := e.st.EndSession(ctx, sessionID, "solved", &stars); err != nil {
			return result, err
		}
		_ = e.rt.Destroy(ctx, sess.SandboxID)
		lc, err := e.applyLevel(ctx, sess.LearnerID, ex.Topic, sessionID)
		if err != nil {
			return result, err
		}
		result.Level = lc
	}
	return result, nil
}

// readyTimeout bounds how long a restarted sandbox may take to accept commands.
const readyTimeout = 30 * time.Second

// runCheck runs check.sh the way the exercise asks (spec section 4.2): it
// restarts the sandbox first if check.before is "restart", then runs the
// check inside the sandbox, or, for runs_from "runner", inside a separate
// runner container on the internal network that reaches the sandbox by name
// through RC_SANDBOX_HOST. spec is the spec the sandbox was created from.
// Check and the selftest both grade through here.
func (e *Engine) runCheck(ctx context.Context, ex *exercise.Exercise, spec runtime.SandboxSpec, sandboxID string, env map[string]string) (runtime.ExecResult, error) {
	if ex.Check.Before == "restart" {
		if err := e.rt.Restart(ctx, sandboxID); err != nil {
			return runtime.ExecResult{}, err
		}
		if err := e.waitReady(ctx, ex, sandboxID, false); err != nil {
			return runtime.ExecResult{}, err
		}
	}
	if ex.Check.RunsFrom != "runner" {
		return e.execScript(ctx, sandboxID, ex.Dir(), "check.sh", env)
	}

	rs := runnerSpec(spec)
	_ = e.rt.Destroy(ctx, rs.Name) // a runner left by an interrupted check
	runner, err := e.rt.Create(ctx, rs)
	if err != nil {
		return runtime.ExecResult{}, fmt.Errorf("create runner: %w", err)
	}
	defer func() { _ = e.rt.Destroy(context.Background(), runner.ID) }()

	runnerEnv := make(map[string]string, len(env)+1)
	for k, v := range env {
		runnerEnv[k] = v
	}
	runnerEnv["RC_SANDBOX_HOST"] = spec.Name
	return e.execScript(ctx, runner.ID, ex.Dir(), "check.sh", runnerEnv)
}

// runnerSpec derives the runner container's spec from the sandbox's: the same
// image and locked-down limits, no added capabilities, the internal network
// (the loader requires it for runner checks), and a label marking it as a
// runner. It keeps the sandbox's labels, so the reaper treats an orphaned
// runner like its sandbox.
func runnerSpec(sandbox runtime.SandboxSpec) runtime.SandboxSpec {
	labels := make(map[string]string, len(sandbox.Labels)+1)
	for k, v := range sandbox.Labels {
		labels[k] = v
	}
	labels[labelRunner] = "true"
	return runtime.SandboxSpec{
		Name:    sandbox.Name + "-runner",
		Image:   sandbox.Image,
		Memory:  sandbox.Memory,
		CPUs:    sandbox.CPUs,
		Network: "internal",
		Labels:  labels,
	}
}

// waitReady blocks until a new or restarted sandbox is ready, or readyTimeout
// passes. A plain sandbox is ready once it accepts a command. A systemd
// sandbox is ready once boot has finished; see waitBooted for strict.
func (e *Engine) waitReady(ctx context.Context, ex *exercise.Exercise, sandboxID string, strict bool) error {
	ctx, cancel := context.WithTimeout(ctx, readyTimeout)
	defer cancel()
	if ex.Init == "systemd" {
		return e.waitBooted(ctx, sandboxID, strict)
	}
	for {
		res, err := e.rt.Exec(ctx, sandboxID, []string{"true"}, nil)
		if err == nil && res.ExitCode == 0 {
			return nil
		}
		select {
		case <-ctx.Done():
			return e.notReady(sandboxID, fmt.Sprintf("sandbox not ready within %s", readyTimeout))
		case <-time.After(100 * time.Millisecond):
		}
	}
}

// waitBooted waits for systemd to finish booting. `systemctl
// is-system-running --wait` blocks until boot is over and prints the state.
// Until systemd's bus is up the command itself fails, so it is retried until
// ctx ends.
//
// With strict, used for a freshly created sandbox, only "running" is ready:
// "degraded" is an error naming the failed units, so a unit that is broken
// before the learner has touched anything is never accepted silently.
// Without strict, used after the check's restart, "degraded" also counts as
// booted: the learner may have broken a unit, and check.sh grades that.
func (e *Engine) waitBooted(ctx context.Context, sandboxID string, strict bool) error {
	var state string
	for {
		res, err := e.rt.Exec(ctx, sandboxID, []string{"systemctl", "is-system-running", "--wait"}, nil)
		state = strings.TrimSpace(string(res.Stdout))
		switch {
		case err == nil && state == "running":
			return nil
		case err == nil && state == "degraded" && !strict:
			return nil
		case err == nil && state == "degraded":
			failed, _ := e.rt.Exec(ctx, sandboxID, []string{"systemctl", "--failed", "--no-legend", "--plain"}, nil)
			return fmt.Errorf("sandbox booted degraded, failed units: %s", strings.TrimSpace(string(failed.Stdout)))
		}
		select {
		case <-ctx.Done():
			return e.notReady(sandboxID, fmt.Sprintf("sandbox not booted within %s (last state %q)", readyTimeout, state))
		case <-time.After(100 * time.Millisecond):
		}
	}
}

// applyLevel recomputes the topic level from all its finished outcomes and, if
// this session changed it, logs a level_changed event (spec section 7.1). It
// returns the change, or nil if the level held.
func (e *Engine) applyLevel(ctx context.Context, learnerID int, topic, sessionID string) (*LevelChange, error) {
	outs, err := e.st.TopicOutcomes(ctx, learnerID, topic)
	if err != nil {
		return nil, err
	}
	if len(outs) == 0 {
		return nil, nil
	}
	levels := levelOutcomes(outs)
	after := grading.Level(levels)
	before := grading.Level(levels[:len(levels)-1])
	if after == before {
		return nil, nil
	}
	lc := &LevelChange{Topic: topic, From: before, To: after}
	if err := e.st.AddEvent(ctx, sessionID, "level_changed",
		map[string]any{"topic": topic, "from": before, "to": after}); err != nil {
		return nil, err
	}
	return lc, nil
}

// TopicLevel returns the learner's current level in a topic (spec section
// 7.1), replayed from every finished session in that topic.
func (e *Engine) TopicLevel(ctx context.Context, learnerID int, topic string) (int, error) {
	outs, err := e.st.TopicOutcomes(ctx, learnerID, topic)
	if err != nil {
		return 0, err
	}
	return grading.Level(levelOutcomes(outs)), nil
}

// levelOutcomes turns stored sessions into the inputs the level rule needs.
func levelOutcomes(outs []store.Outcome) []grading.Outcome {
	levels := make([]grading.Outcome, len(outs))
	for i, o := range outs {
		levels[i] = grading.Outcome{
			Kind:       grading.Classify(o.Status, o.Stars, o.HadCheck),
			Difficulty: o.Difficulty,
		}
	}
	return levels
}

// Hint reveals the next hint tier and logs hint_revealed. It returns the tier
// number (1-based), capped at the number of hints the exercise defines.
func (e *Engine) Hint(ctx context.Context, sessionID string) (int, error) {
	sess, err := e.st.SessionByID(ctx, sessionID)
	if err != nil {
		return 0, err
	}
	ex, err := exercise.Load(filepath.Join(e.exDir, sess.ExerciseID))
	if err != nil {
		return 0, err
	}
	en, err := ex.LoadI18n("en")
	if err != nil {
		return 0, err
	}
	if sess.HintsRevealed >= len(en.Hints) {
		return sess.HintsRevealed, nil // all hints already revealed
	}
	tier, err := e.st.IncHints(ctx, sessionID)
	if err != nil {
		return 0, err
	}
	if err := e.st.AddEvent(ctx, sessionID, "hint_revealed", map[string]any{"tier": tier}); err != nil {
		return tier, err
	}
	return tier, nil
}

// Reset rebuilds the sandbox for the same variant and re-plants the fault,
// keeping revealed hints. It logs reset (spec section 6). If the rebuild
// fails, the session has no working sandbox, so it is voided with a voided
// event, the same way as a failed Start.
func (e *Engine) Reset(ctx context.Context, sessionID string) error {
	sess, err := e.st.SessionByID(ctx, sessionID)
	if err != nil {
		return err
	}
	ex, err := exercise.Load(filepath.Join(e.exDir, sess.ExerciseID))
	if err != nil {
		return err
	}
	_ = e.rt.Destroy(ctx, sess.SandboxID)

	void := func(sandboxID string, err error) error {
		bg := context.Background()
		if sandboxID != "" {
			_ = e.rt.Destroy(bg, sandboxID)
		}
		_ = e.st.AddEvent(bg, sessionID, "voided",
			map[string]any{"reason": "reset_failed", "error": err.Error()})
		_ = e.st.EndSession(bg, sessionID, "voided", nil)
		return err
	}
	sb, err := e.createSandbox(ctx, ex, e.specFor(ex, sessionID))
	if err != nil {
		return void("", fmt.Errorf("recreate sandbox: %w", err))
	}
	if err := e.st.SetSandbox(ctx, sessionID, sb.ID); err != nil {
		return void(sb.ID, err)
	}
	if _, err := e.prepare(ctx, sb.ID, ex, scriptEnv(sess.Seed, sess.VariantValues)); err != nil {
		return void(sb.ID, err)
	}
	return e.st.AddEvent(ctx, sessionID, "reset", nil)
}

// GiveUp ends the session (0 stars) and logs gave_up. It is only allowed once
// hint 3 has been revealed (spec section 6). It returns any level change.
func (e *Engine) GiveUp(ctx context.Context, sessionID string) (*LevelChange, error) {
	sess, err := e.st.SessionByID(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	if sess.HintsRevealed < 3 {
		return nil, ErrGiveUpTooEarly
	}
	ex, err := exercise.Load(filepath.Join(e.exDir, sess.ExerciseID))
	if err != nil {
		return nil, err
	}
	if err := e.st.AddEvent(ctx, sessionID, "gave_up", nil); err != nil {
		return nil, err
	}
	zero := 0
	if err := e.st.EndSession(ctx, sessionID, "gave_up", &zero); err != nil {
		return nil, err
	}
	_ = e.rt.Destroy(ctx, sess.SandboxID)
	return e.applyLevel(ctx, sess.LearnerID, ex.Topic, sessionID)
}

// Expire ends a session whose TTL has passed: it logs expired, marks the
// session expired, destroys the sandbox, and applies the level rule (an expiry
// after a check counts as a fail; see grading.Classify). Used by the reaper.
func (e *Engine) Expire(ctx context.Context, sess store.Session) (*LevelChange, error) {
	if err := e.st.AddEvent(ctx, sess.ID, "expired", nil); err != nil {
		return nil, err
	}
	if err := e.st.EndSession(ctx, sess.ID, "expired", nil); err != nil {
		return nil, err
	}
	_ = e.rt.Destroy(ctx, sess.SandboxID)

	// Apply the level rule if we can determine the topic; a missing exercise
	// definition should not block expiry.
	ex, err := exercise.Load(filepath.Join(e.exDir, sess.ExerciseID))
	if err != nil {
		return nil, nil
	}
	return e.applyLevel(ctx, sess.LearnerID, ex.Topic, sess.ID)
}

// Shell opens an interactive shell in the session's sandbox (v1).
func (e *Engine) Shell(ctx context.Context, sessionID string) error {
	sess, err := e.st.SessionByID(ctx, sessionID)
	if err != nil {
		return err
	}
	if e.sheller == nil {
		return ErrNoShell
	}
	return e.sheller.Shell(ctx, sess.SandboxID, []string{"/bin/bash"})
}

// execScript copies a script into the sandbox under /root (root-owned,
// non-sticky), runs it as root with `bash`, deletes it, and returns the raw
// result including the exit code. It invokes the script with `bash` rather than
// setting an execute bit, because under cap-drop + userns the copied file is
// owned by a UID the sandbox root cannot chmod. The caller decides what a
// non-zero exit means (an error for break, a graded result for check).
//
// env is passed to this one command through `env NAME=value ...`, never to the
// container, so the learner cannot read the seed or variant values. Each value
// is its own argument, so nothing is interpreted by a shell.
func (e *Engine) execScript(ctx context.Context, sandboxID, dir, name string, env map[string]string) (runtime.ExecResult, error) {
	data, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		return runtime.ExecResult{}, err
	}
	path := "/root/" + name
	if err := e.rt.CopyIn(ctx, sandboxID, path, data); err != nil {
		return runtime.ExecResult{}, err
	}
	cmd := []string{"env"}
	for _, k := range sortedKeys(env) {
		cmd = append(cmd, k+"="+env[k])
	}
	cmd = append(cmd, "bash", "-c", `bash "$0"; rc=$?; rm -f "$0"; exit $rc`, path)
	return e.rt.Exec(ctx, sandboxID, cmd, nil)
}

// scriptEnv builds the RC_SEED and RC_VAR_<NAME> values the exercise scripts
// receive (spec section 4.3).
func scriptEnv(seed int64, variant map[string]string) map[string]string {
	env := map[string]string{"RC_SEED": strconv.FormatInt(seed, 10)}
	for k, v := range variant {
		env["RC_VAR_"+envName(k)] = v
	}
	return env
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// --- helpers ---------------------------------------------------------------

func imageTag(ex *exercise.Exercise) string {
	return fmt.Sprintf("rootcause/%s:v%d", ex.ID, ex.Version)
}

// envName turns a variant name into the RC_VAR_<NAME> suffix (site_name -> SITE_NAME).
func envName(name string) string {
	return strings.ToUpper(strings.ReplaceAll(name, "-", "_"))
}

func newID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return "s-" + hex.EncodeToString(b)
}

type checkOut struct {
	Passed   bool   `json:"passed"`
	Feedback string `json:"feedback"`
}

// parseCheck reads the last JSON object printed by check.sh.
func parseCheck(stdout []byte) checkOut {
	var c checkOut
	if line := lastJSONLine(stdout); line != "" {
		_ = json.Unmarshal([]byte(line), &c)
	}
	return c
}

// lastJSONObject decodes the last JSON object line into a generic map.
func lastJSONObject(stdout []byte) map[string]any {
	m := map[string]any{}
	if line := lastJSONLine(stdout); line != "" {
		_ = json.Unmarshal([]byte(line), &m)
	}
	return m
}

// lastJSONLine returns the last non-empty line that looks like a JSON object.
func lastJSONLine(stdout []byte) string {
	lines := strings.Split(strings.TrimSpace(string(stdout)), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		s := strings.TrimSpace(lines[i])
		if strings.HasPrefix(s, "{") && strings.HasSuffix(s, "}") {
			return s
		}
	}
	return ""
}
