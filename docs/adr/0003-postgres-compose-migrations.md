# ADR 0003: PostgreSQL, pgx and hand-rolled migrations

Status: Accepted (Phase 2). Recorded 2026-10-27.

## Context

Sessions and events must be stored durably and read by dbt.

## Decision

- PostgreSQL, run with Docker Compose in v1 and as a StatefulSet in v2.
- The Go driver is pgx v5.
- Migrations are versioned SQL files embedded in the binary and applied on startup under a Postgres advisory lock.
- Integration tests use only a database whose name ends in `_test`.

## Consequences

- No migration framework to learn or update.
- Concurrent starts cannot apply a migration twice.
