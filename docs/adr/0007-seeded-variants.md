# ADR 0007: Randomized variants from a seed

Status: Accepted (Phase 1). Recorded 2026-10-27.

## Context

Repeating an exercise should not mean repeating the exact same machine, but a session must be reproducible.

## Decision

- A seed selects one value per variant (file names, ports, users, ticket sender). The same seed always gives the same variant and the same fault.
- Values reach the scripts as `RC_SEED` and `RC_VAR_<NAME>`, and break.sh may print derived values that fill hint placeholders.
- A review returns as a new variant: the first of up to 20 seeds whose values differ from the last session's.

## Consequences

- Validation runs 3 seeds per exercise.
- Reset recreates the same variant.
