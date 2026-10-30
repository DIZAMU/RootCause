# ADR 0018 · Conventional, SSH-signed commits by the author

Accepted on 2026-10-27.

## Context

Every change should be attributable to a person and reviewed before it lands.

## Decision

- Commits follow Conventional Commits and are signed with an SSH key, authored by DIZAMU with the GitHub noreply address.
- Only the author commits, pushes and merges. There are no Co-authored-by trailers and no tool attribution lines.
- `.githooks/commit-msg` rejects messages that break these rules, and `commit-policy.yml` checks again in CI, including that GitHub shows every commit as Verified.
- Work happens on short-lived branches, merged to `main` through a pull request with a merge commit.

## Consequences

- Every diff is reviewed by the author before it is committed.
- Squash and rebase merges are not used, because they drop the signatures.
