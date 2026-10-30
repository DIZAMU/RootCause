# ADR 0005 · Bash scripts and the exercise contract

Accepted on 2026-10-27.

## Context

Exercises are written by hand, so they must be easy to read and review. The learner must never be able to read the fault or the check.

## Decision

- Each exercise is a folder with `exercise.yaml`, a Dockerfile, an optional `setup.sh`, `break.sh`, `check.sh`, `solution.sh` and texts in English and Norwegian (ARCHITECTURE.md section 4).
- The scripts are Bash. setup, break and check are copied in only for the command that needs them, run as root and then deleted. Their values reach only that command, never the sandbox environment.
- `check.sh` exits 0 for a pass and 1 for a fail. Anything else counts as an infrastructure error and voids the session.
- `solution.sh` never enters an image or a session. Only CI uses it.

## Consequences

- The image holds only what a real server would have.
- Every check has an assertion against shortcuts. For 3 seeds, CI runs break then check, which must fail, and break, solution, check, which must pass.
