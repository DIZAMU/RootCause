# ADR 0001 · A Go engine, the docker CLI and one Runtime interface

Accepted on 2026-10-27.

## Context

RootCause needs one binary that is the CLI, the local server, the grader and the sandbox controller. It also has to run sandboxes on Kubernetes as well as on Docker.

## Decision

- The engine is written in Go (ARCHITECTURE.md section 2).
- All sandbox control goes through the `Runtime` interface (ARCHITECTURE.md section 8.3), with Create, Exec, CopyIn, Restart, Destroy and List. Optional abilities such as an interactive shell, a terminal, diagnosis and a systemd check are separate small interfaces.
- The Docker runtime calls the `docker` CLI instead of importing the Docker SDK.
- The Kubernetes runtime uses client-go, the one Kubernetes dependency the design names.

## Alternatives considered

The Docker Engine Go SDK. It brings a large dependency tree for a handful of calls.

## Consequences

- Kubernetes mode was added later without changing a single caller of the interface.
- Calling the CLI keeps the dependency list short. The cost is parsing CLI output in `List` and `Diagnose`.
- `Restart` has no faithful Kubernetes form, because a restarted container loses its files. The Kubernetes runtime refuses it (ADR 0019).
