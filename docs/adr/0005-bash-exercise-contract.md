# ADR 0005: Bash scripts and the exercise contract

Status: Accepted (Phase 1). Recorded 2026-10-27.

## Context

Exercises are written by hand and must be easy to read and review, and the learner must never be able to read the fault or the check.

## Decision

- Each exercise is a folder with `exercise.yaml`, a Dockerfile, optional `setup.sh`, `break.sh`, `check.sh`, `solution.sh` and EN/NO texts (spec section 4).
- Scripts are Bash. setup, break and check are copied in only for the command that needs them, run as root and deleted; their values reach only that command, never the sandbox environment.
- `check.sh` exits 0 for a pass and 1 for a fail; anything else is infrastructure and voids the session.
- `solution.sh` never enters an image or a session; CI uses it.

## Consequences

- The image holds only what a real server would have.
- Every check has an anti-shortcut assertion, and CI runs break then check (must fail) and break, solution, check (must pass) for 3 seeds.
