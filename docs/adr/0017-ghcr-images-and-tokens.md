# ADR 0017 · The API image on GHCR, exercise images kept local

Accepted on 2026-10-27. Partly replaced by ADR 0020, which makes the API image public.

## Context

The cluster must run the released API image. While the repository was private, the free plan allowed 500 MB of private package storage and 1 GB of transfer a month.

## Decision

- Only the API image goes to `ghcr.io/dizamu/rootcause-api`. It is tagged `vX.Y.Z` only and pushed by `release.yml` with GITHUB_TOKEN.
- While the package was private, the cluster pulled it with a classic personal access token limited to `read:packages`, with a 90-day expiry. GitHub Packages accepts only classic tokens, and such a token can read every package the account can see.
- The token and the deploy key live in `~/.config/rootcause` with mode 600. They reach the cluster as Secrets that `scripts/k8s-up.sh` creates from those files, never through git, a command line or any output.
- Exercise images stay local. `k8s-up.sh` builds them and imports them into the nodes, and sandbox pods never pull.

## Consequences

- All nine images together are about 210 MB compressed, so pushing the exercise images would have filled the private quota within about two releases.
- The API image's exercises and the imported images come from different trees. A changed exercise bumps its version and so its image tag, which makes a missing image fail loudly instead of running a stale one.
