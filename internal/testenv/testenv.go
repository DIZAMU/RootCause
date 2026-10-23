// Package testenv gives integration tests one rule for when they may skip.
// A test skips only when what it needs was never configured: RC_TEST_DATABASE_URL
// is unset, or Docker is missing and RC_REQUIRE_DOCKER is not 1. Anything
// configured but broken (an unsafe or unreachable database, a missing Docker
// that CI requires) fails the test, so CI can never pass by skipping.
package testenv

import (
	"context"
	"net/url"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/DIZAMU/RootCause/internal/store"
	"github.com/jackc/pgx/v5"
)

// DSN returns the test database DSN. It skips the test when
// RC_TEST_DATABASE_URL is unset and fails it when the variable names a
// database that does not end in _test.
func DSN(t testing.TB) string {
	t.Helper()
	dsn, err := store.TestDSN()
	if err != nil {
		t.Fatal(err)
	}
	if dsn == "" {
		t.Skip("RC_TEST_DATABASE_URL not set")
	}
	return dsn
}

// Store opens and migrates the database in dsn, failing the test if it is
// unreachable. The store is closed by t.Cleanup, so register it before any
// cleanup that still needs the connection.
func Store(t testing.TB, dsn string) *store.Store {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	st, err := store.Open(ctx, dsn)
	if err != nil {
		t.Fatalf("test database not reachable: %v", err)
	}
	t.Cleanup(st.Close)
	if err := st.Migrate(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return st
}

// NoDocker skips the test, or fails it when RC_REQUIRE_DOCKER is 1. Call it
// when Docker, or a Docker runtime the test needs, is not usable.
func NoDocker(t testing.TB, args ...any) {
	t.Helper()
	if os.Getenv("RC_REQUIRE_DOCKER") == "1" {
		t.Fatal(append([]any{"docker required (RC_REQUIRE_DOCKER=1):"}, args...)...)
	}
	t.Skip(args...)
}

// Docker checks that the docker daemon answers, and calls NoDocker if not.
func Docker(t testing.TB) {
	t.Helper()
	if err := exec.Command("docker", "info").Run(); err != nil {
		NoDocker(t, "docker daemon not reachable:", err)
	}
}

var ownName = regexp.MustCompile(`^[a-z0-9_]+$`)

// OwnDatabase creates an empty database used only by the calling test and
// returns its DSN. Its name is the test database's name with "_<name>" put
// before the "_test" suffix, so rootcause_test gives rootcause_<name>_test,
// and the _test guard still holds. A copy left by an earlier, killed run is
// dropped first. The database is dropped again after the test.
//
// Tests that run dbt use it: a dbt build reads every row in the database, so
// rows that other test packages add or delete at the same time would make its
// tests fail at random.
func OwnDatabase(t testing.TB, name string) string {
	t.Helper()
	if !ownName.MatchString(name) {
		t.Fatalf("OwnDatabase: name %q must match %s", name, ownName)
	}
	base := DSN(t)
	u, err := url.Parse(base)
	if err != nil {
		t.Fatalf("parse RC_TEST_DATABASE_URL: %v", err)
	}
	baseName := strings.TrimPrefix(u.Path, "/")
	db := strings.TrimSuffix(baseName, "_test") + "_" + name + "_test"
	quoted := pgx.Identifier{db}.Sanitize()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	admin, err := store.Open(ctx, base)
	if err != nil {
		t.Fatalf("test database not reachable: %v", err)
	}
	t.Cleanup(admin.Close)
	for _, sql := range []string{"DROP DATABASE IF EXISTS " + quoted + " WITH (FORCE)", "CREATE DATABASE " + quoted} {
		if _, err := admin.Pool().Exec(ctx, sql); err != nil {
			t.Fatalf("%s: %v", sql, err)
		}
	}
	// Registered after admin.Close, so it runs before it.
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if _, err := admin.Pool().Exec(ctx, "DROP DATABASE IF EXISTS "+quoted+" WITH (FORCE)"); err != nil {
			t.Errorf("drop %s: %v", db, err)
		}
	})

	u.Path = "/" + db
	return u.String()
}
