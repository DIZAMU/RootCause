# ADR 0015: GitOps with Argo CD core and a two-merge release

Status: Accepted (Phase 8). Recorded 2026-10-27.

## Context

Spec section 18 asks that a merged change reaches the cluster without manual kubectl, on a laptop with WSL2 capped at 8 GB (OQ-4).

## Decision

- Argo CD core v3.5.6 (no UI, SSO, notifications or ApplicationSet controller), installed from its pinned, checksum-verified manifest with images pinned by digest. One Application syncs `deploy/k8s` from `main` with automated sync, prune and self-heal.
- Measured: 226 MiB idle, 369 MiB at peak during a sync, about 0.4 GB added to WSL2's use (2.5 to 2.6 GB at peak in all), below the agreed limit of 0.75 GB.
- A release is two merges and a tag: the owner tags `vX.Y.Z` on `main`; `release.yml` validates every exercise and pushes the API image; the owner pins its digest in `deploy/k8s/kustomization.yaml` in a PR, which CI checks; Argo CD rolls it out.
- No workflow commits or pushes to the repository.
- `scripts/k8s-up.sh --dev` turns automated sync off to try the working tree; the GitOps proof fails while it is off.

## Alternatives considered

- Flux: lighter, but Argo CD is named in spec section 2 and fits the budget.
- A workflow that commits the new tag: breaks the rule that only the owner commits.
- Argo CD Image Updater: another component and its own write access.

## Consequences

- Every deployed state is a reviewed, signed commit on `main`.
- A release needs one more PR than a bot-driven flow.
