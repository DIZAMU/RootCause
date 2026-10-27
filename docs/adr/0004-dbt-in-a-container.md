# ADR 0004: dbt Core in a container, from the embedded project

Status: Accepted (Phases 4 and 6). Recorded 2026-10-27.

## Context

Statistics, the review queue and calibration are transformations of the engine's tables. They must never touch the engine's own tables, and a failed build must never change a session's result.

## Decision

- dbt Core with the Postgres adapter, in the pinned `dbt-postgres` image, writing only to schema `analytics` (spec section 9.2).
- The project is embedded in the binary and extracted into a fresh directory per build, so a build never reads the working tree.
- A build passes only when dbt exits 0 and runs exactly as many nodes as the project defines.
- In the cluster, a CronJob builds every 10 minutes; the API has no right to start Jobs.

## Consequences

- The selector falls back to topic levels alone when the marts are stale or a build fails.
- The node-count check of section 9.2 applies to builds the binary starts, not to the CronJob (ADR 0019).
