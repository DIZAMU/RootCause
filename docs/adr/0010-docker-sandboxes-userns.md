# ADR 0010 · Docker mode sandboxes use user namespace remapping

Accepted on 2026-10-27.

## Context

The learner is root inside the sandbox. Root in a container must not be root on the host.

## Decision

- In Docker mode, sandboxes are containers on the local Docker Engine.
- The daemon runs with `userns-remap`, set in `/etc/docker/daemon.json` as `{"userns-remap": "default"}`. Container root then maps to host UID 100000.
- The CLI refuses to start a session when `docker info` shows neither `name=userns` nor `name=rootless`.

## Alternatives considered

Rootless Docker. It needs a second daemon setup per user and changes how the CLI finds Docker.

## Consequences

- Setup is one config change and a restart. Images are pulled again into the remapped storage.
- Files copied into a sandbox are owned by a UID that sandbox root cannot chmod, so scripts run as `bash script` instead of relying on an execute bit.
