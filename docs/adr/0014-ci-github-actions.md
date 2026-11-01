# ADR 0014 · CI on GitHub Actions

Accepted on 2026-10-27.

## Context

Every change must be checked the same way, within the free plan's Actions minutes.

## Decision

- The workflows are `ci.yml`, `exercises.yml`, `dbt.yml`, `commit-policy.yml` and `release.yml` (ARCHITECTURE.md section 14). The scripts only CI uses live in `.github/scripts`.
- Every action is pinned by commit SHA. Workflows have no permissions by default. Each job gets read access to the repository, and package access only where it needs it.
- govulncheck fails CI when the code calls anything with a known vulnerability, in a module or in the Go standard library.
- Any skipped Go test fails CI. Only changed exercises are validated on a pull request, and all of them before a release.
- Workflows start on every pull request and decide inside the job what to run, so they can all be required checks.
- Scripts never put a credential on a command line, and `.github/scripts/ci-check-credential-args.sh` checks that.

## Consequences

- A ruleset on `main` requires every check, signed commits and a pull request.
- The workflows and the commit check run from the pull request's own commits, so a pull request could weaken the checks that judge it. This is listed as a known limit (ADR 0019).
