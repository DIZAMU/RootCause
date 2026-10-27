# ADR 0010: v1 sandboxes: Docker with user namespace remapping (OQ-3 v1)

Status: Accepted (Phase 2). Recorded 2026-10-27.

## Context

The learner is root inside the sandbox. Root in a container must not be root on the host.

## Decision

- Sandboxes are Docker containers on the local Docker Engine in WSL2.
- The daemon runs with `userns-remap` (`/etc/docker/daemon.json` with `{"userns-remap": "default"}`). Container root maps to host UID 100000.
- The CLI refuses to start sessions when `docker info` shows neither `name=userns` nor `name=rootless`.

## Alternatives considered

Rootless Docker: needs a second daemon setup per user and changes how the CLI finds Docker.

## Consequences

- One config change and a restart; existing images are pulled again into the remapped storage.
- Files copied in are owned by a UID the sandbox root cannot chmod, so scripts run with `bash script` instead of an execute bit.
