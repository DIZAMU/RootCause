// Package store is the engine's PostgreSQL access layer: connection, schema
// migrations, and the source tables from spec section 9.1. It uses pgx.
package store

import (
	"context"
	"fmt"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Store wraps a connection pool.
type Store struct {
	pool *pgxpool.Pool
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
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping: %w", err)
	}
	return &Store{pool: pool}, nil
}

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
