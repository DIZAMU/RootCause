package dbt_test

import (
	"context"
	"math/rand"
	"net/url"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/DIZAMU/RootCause/internal/dbt"
	"github.com/DIZAMU/RootCause/internal/play"
	"github.com/DIZAMU/RootCause/internal/store"
	"github.com/DIZAMU/RootCause/internal/testenv"
	"github.com/jackc/pgx/v5/pgconn"
)

// helperDSN, when set, turns TestHelperBuild into a dbt build in a child
// process that the parent test kills, like a crashed engine.
const helperDSN = "RC_DBT_HELPER_DSN"

// TestHelperBuild is not a test on its own: it runs one build when a parent
// test starts the test binary as a child process with RC_DBT_HELPER_DSN set.
func TestHelperBuild(t *testing.T) {
	dsn := os.Getenv(helperDSN)
	if dsn == "" {
		return
	}
	_ = dbt.Build(context.Background(), dsn)
}

// buildWorld is a database of the test's own, migrated and empty.
type buildWorld struct {
	t   *testing.T
	st  *store.Store
	dsn string
	db  string
}

func newBuildWorld(t *testing.T) *buildWorld {
	t.Helper()
	testenv.Docker(t)
	dsn := testenv.OwnDatabase(t, "dbt")
	cfg, err := pgconn.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	// Logs of failed builds go to a temporary cache, not the user's.
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	return &buildWorld{t: t, st: testenv.Store(t, dsn), dsn: dsn, db: cfg.Database}
}

// build runs the production path: dbt under the statistics lock.
func (w *buildWorld) build(ctx context.Context) error {
	return play.BuildStats(ctx, w.st, w.dsn)
}

// containers lists this database's dbt containers, running or not.
func (w *buildWorld) containers() []string {
	w.t.Helper()
	out, err := exec.Command("docker", "ps", "-aq",
		"--filter", "label=rootcause.dbt=true",
		"--filter", "label=rootcause.database="+w.db).Output()
	if err != nil {
		w.t.Fatalf("docker ps: %v", err)
	}
	return strings.Fields(string(out))
}

// mustBuild builds and checks that no dbt container of this database is
// left afterwards.
func (w *buildWorld) mustBuild(step string) {
	w.t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	if err := w.build(ctx); err != nil {
		w.t.Fatalf("%s: build: %v", step, err)
	}
	if left := w.containers(); len(left) > 0 {
		w.t.Fatalf("%s: dbt containers left after the build: %v", step, left)
	}
}

// killedChild starts a build in a child process, kills the child (not the
// container) after delay, and returns. If the child was killed while its
// container ran, that container is now stale, as after an engine crash.
func (w *buildWorld) killedChild(delay time.Duration) {
	w.t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=^TestHelperBuild$", "-test.count=1")
	cmd.Env = append(os.Environ(), helperDSN+"="+w.dsn)
	if err := cmd.Start(); err != nil {
		w.t.Fatalf("start child build: %v", err)
	}
	time.Sleep(delay) // the random kill point itself, not a wait for readiness
	_ = cmd.Process.Kill()
	_ = cmd.Wait()
}

// waitForContainer polls until this database has a running dbt container.
func (w *buildWorld) waitForContainer(timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		out, _ := exec.Command("docker", "ps", "-q", "--filter", "status=running",
			"--filter", "label=rootcause.dbt=true", "--filter", "label=rootcause.database="+w.db).Output()
		if len(strings.TrimSpace(string(out))) > 0 {
			return true
		}
		time.Sleep(100 * time.Millisecond)
	}
	return false
}

// TestBuildRecoversFromKilledEngine: an engine killed during a build leaves
// its container running. The next build removes it and succeeds, and every
// build leaves no container behind.
func TestBuildRecoversFromKilledEngine(t *testing.T) {
	w := newBuildWorld(t)
	w.mustBuild("first build")

	cmd := exec.Command(os.Args[0], "-test.run=^TestHelperBuild$", "-test.count=1")
	cmd.Env = append(os.Environ(), helperDSN+"="+w.dsn)
	if err := cmd.Start(); err != nil {
		t.Fatalf("start child build: %v", err)
	}
	if !w.waitForContainer(time.Minute) {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		t.Fatal("the child build's container never ran")
	}
	_ = cmd.Process.Kill()
	_ = cmd.Wait()
	if len(w.containers()) == 0 {
		t.Fatal("no stale container after killing the child; the test proves nothing")
	}
	w.mustBuild("build after the kill")
}

// TestBuildFailureKeepsLog: a failed build saves the full dbt.log under the
// cache directory, names it in the error, and removes its container.
func TestBuildFailureKeepsLog(t *testing.T) {
	w := newBuildWorld(t)
	u, err := url.Parse(w.dsn)
	if err != nil {
		t.Fatal(err)
	}
	u.User = url.UserPassword(u.User.Username(), "wrong-password")

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	err = play.BuildStats(ctx, w.st, u.String())
	if err == nil {
		t.Fatal("build with a wrong password succeeded")
	}
	if strings.Contains(err.Error(), "wrong-password") {
		t.Error("the error shows the password")
	}
	m := regexp.MustCompile(`full log: (\S+)`).FindStringSubmatch(err.Error())
	if m == nil {
		t.Fatalf("error names no log: %v", err)
	}
	dir, _ := dbt.LogDir()
	if !strings.HasPrefix(m[1], dir+string(os.PathSeparator)) {
		t.Errorf("log %s is not under %s", m[1], dir)
	}
	data, rerr := os.ReadFile(m[1])
	if rerr != nil {
		t.Fatalf("read saved log: %v", rerr)
	}
	// dbt.log is dbt's debug log, far longer than the console output.
	if !strings.Contains(string(data), "Running with dbt=") || !strings.Contains(string(data), "[debug]") {
		t.Errorf("saved log is not dbt.log:\n%s", data)
	}
	if left := w.containers(); len(left) > 0 {
		t.Errorf("failed build left containers: %v", left)
	}
	w.mustBuild("build after the failure")
}

// TestBuildStress kills builds at random points, half by killing the
// engine's process and half by cancelling the build, and checks that the
// next build always succeeds and runs every node. It runs only with
// RC_DBT_STRESS=1; RC_DBT_STRESS_ROUNDS sets the rounds (default 10).
func TestBuildStress(t *testing.T) {
	if os.Getenv("RC_DBT_STRESS") != "1" {
		t.Skip("set RC_DBT_STRESS=1 to run the dbt stress test")
	}
	rounds := 10
	if s := os.Getenv("RC_DBT_STRESS_ROUNDS"); s != "" {
		n, err := strconv.Atoi(s)
		if err != nil || n < 1 {
			t.Fatalf("RC_DBT_STRESS_ROUNDS=%q", s)
		}
		rounds = n
	}
	w := newBuildWorld(t)
	start := time.Now()
	w.mustBuild("first build")
	full := time.Since(start)
	seed := time.Now().UnixNano()
	t.Logf("a full build takes %s; seed %d", full.Round(time.Millisecond), seed)
	rng := rand.New(rand.NewSource(seed))

	for i := 1; i <= rounds; i++ {
		delay := time.Duration(rng.Int63n(int64(full)))
		if i%2 == 1 {
			t.Logf("round %d: kill the engine after %s", i, delay.Round(time.Millisecond))
			w.killedChild(delay)
		} else {
			t.Logf("round %d: cancel the build after %s", i, delay.Round(time.Millisecond))
			ctx, cancel := context.WithTimeout(context.Background(), delay)
			_ = w.build(ctx)
			cancel()
		}
		w.mustBuild("rebuild in round " + strconv.Itoa(i))
	}
}
