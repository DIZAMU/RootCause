# ADR 0017: Private GHCR for the API image, exercise images local

Status: Accepted (Phase 8). Recorded 2026-10-27.

## Context

The cluster must run the released API image. The repository is private, and the free plan gives 500 MB of private package storage and 1 GB of transfer a month.

## Decision

- Only the API image goes to `ghcr.io/dizamu/rootcause-api`, private, tagged `vX.Y.Z` only, pushed by `release.yml` with GITHUB_TOKEN.
- The cluster pulls it with a classic personal access token with only `read:packages` and a 90-day expiry. GitHub Packages accepts only classic tokens. A classic `read:packages` token can read every package the account can see.
- The token and the deploy key live in `~/.config/rootcause`, mode 600, and reach the cluster as Secrets created by `scripts/k8s-up.sh` from files, never in git, on a command line or in output.
- Exercise images stay local: built by `k8s-up.sh` and imported into the nodes; sandbox pods never pull.

## Consequences

- All nine images together are about 210 MB compressed, so exercise images on GHCR would fill the quota within about two releases.
- The token must be renewed every 90 days.
- The API image's exercises and the imported images come from different trees; a changed exercise bumps its version and so its tag, so a missing image fails loudly.
