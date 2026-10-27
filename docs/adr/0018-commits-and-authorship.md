# ADR 0018: Commits: Conventional, SSH-signed, owner only

Status: Accepted (Phase 0). Recorded 2026-10-27.

## Context

The repository is the owner's portfolio. Every change must be attributable and reviewed.

## Decision

- Conventional Commits, signed with an SSH key, author `DIZAMU` with the GitHub noreply address.
- Only the owner commits, pushes and merges. No Co-authored-by trailers and no tool attribution anywhere.
- `.githooks/commit-msg` rejects violating messages locally, and `commit-policy.yml` checks again in CI, including that GitHub shows every commit as Verified.
- Short-lived branches, merged to `main` through a PR with a merge commit.

## Consequences

- Tools prepare changes and commit blocks; the owner reviews each diff and commits.
- Squash and rebase merges are not used, since they drop the signatures.
