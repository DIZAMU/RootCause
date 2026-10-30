# ADR 0020 · The API image is public

Accepted on 2026-10-30. Replaces the private package part of ADR 0017.

## Context

With the repository public, anyone should be able to run the released API image in Kubernetes mode without a GitHub token.

## Decision

- The package `ghcr.io/dizamu/rootcause-api` is public. Releases are still tagged `vX.Y.Z` and pushed only by `release.yml`.
- Exercise images stay local, as in ADR 0017. They are built from the checked-out repository and imported into the nodes.
- The GitOps setup stays the author's own. `scripts/k8s-up.sh` without `--dev` still expects the author's token and read-only deploy key, because Argo CD follows this repository's `main`.
- Anyone else runs Kubernetes mode with `scripts/k8s-up.sh --dev`, which builds every image from the checked-out repository and needs no credentials.

## Consequences

- No token is needed to try Kubernetes mode or to pull a released API image.
- Public packages do not count toward the storage quota, so the reasons for keeping exercise images local are now simplicity and the import step, not cost.
