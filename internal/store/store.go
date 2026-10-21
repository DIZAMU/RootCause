// Package store is the engine's PostgreSQL access layer: connection, schema
// migrations, and the source tables from spec section 9.1. It uses pgx.
package store

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Store wraps a connection pool.
type Store struct {
	pool   *pgxpool.Pool
	dbName string // the connected database's name, read once at Open
}

// Open connects to Postgres using the given DSN and verifies the connection.
func Open(ctx context.Context, dsn string) (*Store, error) {
	if dsn == "" {
		return nil, fmt.Errorf("no database DSN: set RC_DATABASE_URL")
	}
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("connect: %w", err)
	}
	var dbName string
	if err := pool.QueryRow(ctx, `SELECT current_database()`).Scan(&dbName); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping: %w", err)
	}
	return &Store{pool: pool, dbName: dbName}, nil
}

// Database returns the name of the connected database. Sandboxes are labelled
// with it, so a reaper only reclaims sandboxes that its own database started:
// the real profile and the test database share one Docker daemon.
func (s *Store) Database() string { return s.dbName }

// Close releases the pool.
func (s *Store) Close() {
	if s.pool != nil {
		s.pool.Close()
	}
}

// Pool exposes the underlying pool for other store files and tests.
func (s *Store) Pool() *pgxpool.Pool { return s.pool }

// DSNFromEnv returns the database DSN from RC_DATABASE_URL, or "" if unset.
func DSNFromEnv() string { return os.Getenv("RC_DATABASE_URL") }

// TestDSN returns the DSN integration tests may use: RC_TEST_DATABASE_URL, and
// only if its database name ends in "_test". Tests create sessions, change the
// language and re-sync exercises, so they must never touch the learner's real
// database. It returns "" when no safe test database is configured.
func TestDSN() string {
	dsn := os.Getenv("RC_TEST_DATABASE_URL")
	if dsn == "" {
		return ""
	}
	path := dsn
	if i := strings.IndexAny(path, "?"); i >= 0 {
		path = path[:i]
	}
	if !strings.HasSuffix(path, "_test") {
		return ""
	}
	return dsn
}
