# ADR 0001: Go engine, the docker CLI and one Runtime interface

Status: Accepted (Phase 2). Recorded 2026-10-27.

## Context

RootCause needs one binary that is the CLI, the local server, the grader and the sandbox controller, and it must later run sandboxes on Kubernetes as well as Docker.

## Decision

- The engine is written in Go (spec section 2).
- All sandbox control goes through the `Runtime` interface (spec section 8.3): Create, Exec, CopyIn, Restart, Destroy, List. Optional capabilities (an interactive shell, a terminal, diagnosis, a systemd check) are separate small interfaces.
- The Docker runtime shells out to the `docker` CLI instead of importing the Docker SDK.
- The Kubernetes runtime uses client-go, the one Kubernetes dependency spec section 2 names.

## Alternatives considered

The Docker Engine Go SDK: a large dependency tree for a handful of calls.

## Consequences

- Kubernetes was added in Phase 7 without changing a caller of the interface.
- Shelling out keeps the dependency list short, at the cost of parsing CLI output in `List` and `Diagnose`.
- `Restart` has no faithful Kubernetes form (a restarted container loses its files), so the Kubernetes runtime refuses it (ADR 0019).
