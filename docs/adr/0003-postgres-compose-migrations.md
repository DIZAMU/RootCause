# ADR 0003 · PostgreSQL, pgx and hand-written migrations

Accepted on 2026-10-27.

## Context

Sessions and events must be stored durably and be readable by dbt.

## Decision

- PostgreSQL, run with Docker Compose in Docker mode and as a StatefulSet in Kubernetes mode.
- The Go driver is pgx v5.
- Migrations are versioned SQL files embedded in the binary and applied on startup under a Postgres advisory lock.
- Integration tests only ever use a database whose name ends in `_test`.

## Consequences

- There is no migration framework to learn or keep up to date.
- Two starts at the same time cannot apply a migration twice.
