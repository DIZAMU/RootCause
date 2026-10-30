# ADR 0007 · Randomized variants from a seed

Accepted on 2026-10-27.

## Context

Repeating an exercise should not mean repeating the exact same machine, but a session must still be reproducible.

## Decision

- A seed selects one value per variant, such as file names, ports, users and the ticket's sender. The same seed always gives the same variant and the same fault.
- The values reach the scripts as `RC_SEED` and `RC_VAR_<NAME>`, and break.sh may print derived values that fill placeholders in the hints.
- A review comes back as a new variant, using the first of up to 20 seeds whose values differ from the last session's.

## Consequences

- Validation runs 3 seeds per exercise.
- A reset recreates the same variant.
