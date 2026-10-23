package dbt

import (
	"context"
	"errors"
	"testing"
)

// TestBuildRefusesOtherHosts checks that a DSN for any database but the
// compose Postgres is refused before docker is called.
func TestBuildRefusesOtherHosts(t *testing.T) {
	for _, dsn := range []string{
		"postgres://u:p@db.example.com:5432/rootcause_test",
		"postgres://u:p@127.0.0.1:5433/rootcause_test",
	} {
		if err := Build(context.Background(), t.TempDir(), dsn); !errors.Is(err, ErrUnsupportedDatabase) {
			t.Errorf("%s: err = %v, want ErrUnsupportedDatabase", dsn, err)
		}
	}
}

// Build itself, against the test database, is covered by the selector's
// TestScriptedSelection, the only test that runs dbt. Two dbt builds at once
// would rebuild the same tables, so the build is tested in one place.
