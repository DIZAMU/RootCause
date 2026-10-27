# ADR 0014: CI on GitHub Actions

Status: Accepted (Phase 6). Recorded 2026-10-27.

## Context

Every change must be checked the same way, on a private repository with limited free minutes.

## Decision

- Workflows: `ci.yml`, `exercises.yml`, `dbt.yml`, `commit-policy.yml` and, since Phase 8, `release.yml` (spec section 14).
- Every action is pinned by commit SHA. Workflows have no permissions by default; each job gets read access to the repository, and package access only where it needs it.
- Any skipped Go test fails CI. Only changed exercises are validated per PR; all of them before a release.
- Workflows start on every PR and decide inside the job what to run, so they can become required checks.
- Scripts never put a credential on a command line; `scripts/ci-check-credential-args.sh` checks it.

## Consequences

- On the free plan a red check does not block the merge button; the rule is never to merge red. The ruleset waits until the owner decides to make the repository public.
- The workflows and the commit check run from the PR's own commits; closing that gap waits with the public items.
