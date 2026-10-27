# ADR 0006: Stars, recorded time and the 2 up / 2 down rule

Status: Accepted (Phases 2 and 4). Recorded 2026-10-27.

## Context

The tool must adapt to weak spots without rewarding speed or punishing exploration.

## Decision

- Stars come only from hints used: 3, 2 or 1, and 0 for giving up (spec section 6). Time is recorded, never scored. Failed checks cost nothing.
- Topic levels 1 to 3 move by a 2 up / 2 down rule (spec section 7.1). The engine is the authority; dbt replays the same events and a test asserts both agree.
- The next exercise: review first, then the weakest topic, never the same exercise twice in a row (spec section 7.2).

## Alternatives considered

Elo-style ratings now: too little data with one learner and 8 exercises.

## Consequences

- Simple to explain to a learner and to test.
- Elo ratings stay open until the catalog passes about 30 exercises (OQ-6).
