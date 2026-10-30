# ADR 0015 · GitOps with Argo CD core and a two-merge release

Accepted on 2026-10-27.

## Context

A merged change should reach the cluster without anyone running kubectl by hand, on a laptop where WSL2 is capped at 8 GB of memory.

## Decision

- Argo CD core v3.5.6, without the UI, SSO, notifications or the ApplicationSet controller. It is installed from its pinned, checksum-verified manifest, with images pinned by digest. One Application syncs `deploy/k8s` from `main` with automated sync, prune and self-heal.
- Measured memory use is 226 MiB idle and 369 MiB at peak during a sync. That adds about 0.4 GB to WSL2's use, which stays under the 0.75 GB set as the limit.
- A release is a tag and two merges. The author tags `vX.Y.Z` on `main`, and `release.yml` validates every exercise and pushes the API image. The author then pins the image digest in `deploy/k8s/kustomization.yaml` in a pull request, which CI checks. Argo CD rolls it out once merged.
- No workflow commits or pushes to the repository.
- `scripts/k8s-up.sh --dev` turns automated sync off to try the working tree, and the GitOps proof fails while it is off.

## Alternatives considered

- Flux. It is lighter, but Argo CD was the planned choice and fits the memory budget.
- A workflow that commits the new tag. It would break the rule that only the author commits.
- Argo CD Image Updater. It adds another component with its own write access.

## Consequences

- Every deployed state is a reviewed, signed commit on `main`.
- A release needs one more pull request than a bot-driven flow would.
