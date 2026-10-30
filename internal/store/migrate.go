package store

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"sort"

	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed migrations/*.sql
var migrationFS embed.FS

// migrateLockKey names the Postgres advisory lock that serializes migrations.
// Two processes starting at once, such as two CLI commands on a new install or
// two test packages, then never apply the same migration twice.
const migrateLockKey int64 = 7300_0001

// Migrate applies every migration not yet applied, in filename order, each in
// its own transaction. Applied migrations are recorded in schema_migrations, so
// re-running is a no-op. Migrations are versioned SQL files (ARCHITECTURE.md section 9.1).
//
// It holds an advisory lock while it works, so concurrent callers wait their
// turn and then find the migrations already applied.
func (s *Store) Migrate(ctx context.Context) error {
	// An advisory lock belongs to one database connection, so everything here
	// runs on a single connection taken from the pool.
	conn, err := s.pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("acquire connection: %w", err)
	}
	defer conn.Release()
	if _, err := conn.Exec(ctx, `SELECT pg_advisory_lock($1)`, migrateLockKey); err != nil {
		return fmt.Errorf("lock migrations: %w", err)
	}
	defer func() {
		// If the unlock fails, close the connection: that releases the lock,
		// and the pool will not hand out a connection that still holds it.
		if _, err := conn.Exec(context.Background(), `SELECT pg_advisory_unlock($1)`, migrateLockKey); err != nil {
			_ = conn.Conn().Close(context.Background())
		}
	}()

	if _, err := conn.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version    TEXT PRIMARY KEY,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
		)`); err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}

	// Read the applied set only after taking the lock, so a migration another
	// process just applied is seen and skipped.
	applied, err := appliedMigrations(ctx, conn)
	if err != nil {
		return err
	}

	files, err := migrationFiles()
	if err != nil {
		return err
	}

	for _, file := range files {
		version := baseName(file)
		if applied[version] {
			continue
		}
		sqlBytes, err := migrationFS.ReadFile(file)
		if err != nil {
			return fmt.Errorf("read %s: %w", file, err)
		}
		if err := applyOne(ctx, conn, version, string(sqlBytes)); err != nil {
			return fmt.Errorf("apply %s: %w", version, err)
		}
	}
	return nil
}

// migrationFiles lists the embedded migration files in the order to apply them.
func migrationFiles() ([]string, error) {
	files, err := fs.Glob(migrationFS, "migrations/*.sql")
	if err != nil {
		return nil, fmt.Errorf("list migrations: %w", err)
	}
	sort.Strings(files)
	return files, nil
}

// applyOne runs one migration and records it, atomically.
func applyOne(ctx context.Context, conn *pgxpool.Conn, version, sql string) error {
	tx, err := conn.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, sql); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx,
		`INSERT INTO schema_migrations (version) VALUES ($1)`, version); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func appliedMigrations(ctx context.Context, conn *pgxpool.Conn) (map[string]bool, error) {
	rows, err := conn.Query(ctx, `SELECT version FROM schema_migrations`)
	if err != nil {
		return nil, fmt.Errorf("read schema_migrations: %w", err)
	}
	defer rows.Close()

	applied := make(map[string]bool)
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		applied[v] = true
	}
	return applied, rows.Err()
}

// baseName returns "migrations/0001_init.sql" -> "0001_init.sql".
func baseName(path string) string {
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '/' {
			return path[i+1:]
		}
	}
	return path
}
