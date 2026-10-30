# ADR 0006 · Stars, recorded time and the 2 up, 2 down rule

Accepted on 2026-10-27.

## Context

The tool should adapt to weak spots without rewarding speed or punishing exploration.

## Decision

- Stars come only from the hints used. A solve earns 3, 2 or 1 stars, and giving up earns 0 (ARCHITECTURE.md section 6). Time is recorded but never scored, and failed checks cost nothing.
- Topic levels from 1 to 3 move by a 2 up, 2 down rule (ARCHITECTURE.md section 7.1). The engine is the authority. dbt replays the same events, and a test asserts that both agree.
- The next exercise is a due review first, then the weakest topic, and never the same exercise twice in a row (ARCHITECTURE.md section 7.2).

## Alternatives considered

Elo-style ratings from the start. With one learner and 8 exercises there is too little data for them to mean much.

## Consequences

- The rules are simple to explain to a learner and simple to test.
- Elo ratings can be reconsidered if the catalog grows to around 30 exercises.
