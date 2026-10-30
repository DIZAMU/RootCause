# Architecture decision records

Each record explains one decision that shapes RootCause. It says what was decided, why, what else was considered and what follows from it. The design itself is described in [`../ARCHITECTURE.md`](../ARCHITECTURE.md).

When a decision changes, a new record replaces the old one and the old one stays, marked as replaced. That way the history of the project's thinking stays readable.

| ADR | Decision |
|---|---|
| [0001](0001-go-engine-and-runtime-interface.md) | A Go engine, the docker CLI and one Runtime interface |
| [0002](0002-dashboard-embedded-local-only.md) | The dashboard is embedded in the binary and local only |
| [0003](0003-postgres-compose-migrations.md) | PostgreSQL, pgx and hand-written migrations |
| [0004](0004-dbt-in-a-container.md) | dbt Core in a container, from the embedded project |
| [0005](0005-bash-exercise-contract.md) | Bash scripts and the exercise contract |
| [0006](0006-grading-and-adaptation.md) | Stars, recorded time and the 2 up, 2 down rule |
| [0007](0007-seeded-variants.md) | Randomized variants from a seed |
| [0008](0008-languages-and-tickets.md) | English and Norwegian from the start, framed as helpdesk tickets |
| [0009](0009-single-local-profile.md) | No accounts, one local profile |
| [0010](0010-docker-sandboxes-userns.md) | Docker mode sandboxes use user namespace remapping |
| [0011](0011-k3d-user-namespaces.md) | Kubernetes mode sandboxes use pod user namespaces in k3d |
| [0012](0012-sandbox-isolation-model.md) | The sandbox isolation model |
| [0013](0013-web-terminal-exec.md) | The web terminal goes through the Kubernetes exec API |
| [0014](0014-ci-github-actions.md) | CI on GitHub Actions |
| [0015](0015-gitops-argo-cd.md) | GitOps with Argo CD core and a two-merge release |
| [0016](0016-argo-cd-trust-boundary.md) | Argo CD is a trust boundary |
| [0017](0017-ghcr-images-and-tokens.md) | The API image on GHCR, exercise images kept local (replaced by 0020) |
| [0018](0018-commits-and-authorship.md) | Conventional, SSH-signed commits by the author |
| [0019](0019-known-limits.md) | Known limits |
| [0020](0020-public-api-image.md) | The API image is public |
