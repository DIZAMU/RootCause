# Architecture decision records

One record per decision that shapes RootCause (spec sections 2 and 20). Each says what was decided, why, what else was considered and what follows from it. A changed decision gets a new record that replaces the old one; records are not rewritten.

| ADR | Decision |
|---|---|
| [0001](0001-go-engine-and-runtime-interface.md) | Go engine, the docker CLI and one Runtime interface |
| [0002](0002-dashboard-embedded-local-only.md) | Dashboard embedded in the binary, local only |
| [0003](0003-postgres-compose-migrations.md) | PostgreSQL, pgx and hand-rolled migrations |
| [0004](0004-dbt-in-a-container.md) | dbt Core in a container, from the embedded project |
| [0005](0005-bash-exercise-contract.md) | Bash scripts and the exercise contract |
| [0006](0006-grading-and-adaptation.md) | Stars, recorded time and the 2 up / 2 down rule |
| [0007](0007-seeded-variants.md) | Randomized variants from a seed |
| [0008](0008-languages-and-tickets.md) | English and Norwegian from day one, helpdesk tickets |
| [0009](0009-single-local-profile.md) | No accounts, one local profile |
| [0010](0010-v1-docker-sandboxes-userns.md) | v1 sandboxes: Docker with user namespace remapping (OQ-3 v1) |
| [0011](0011-v2-k3d-user-namespaces.md) | v2 sandboxes: k3d with pod user namespaces (OQ-3 v2) |
| [0012](0012-sandbox-isolation-model.md) | The sandbox isolation model |
| [0013](0013-web-terminal-exec.md) | Web terminal through the Kubernetes exec API |
| [0014](0014-ci-github-actions.md) | CI on GitHub Actions |
| [0015](0015-gitops-argo-cd.md) | GitOps with Argo CD core and a two-merge release |
| [0016](0016-argo-cd-trust-boundary.md) | Argo CD is a trust boundary |
| [0017](0017-ghcr-private-images.md) | Private GHCR for the API image, exercise images local |
| [0018](0018-commits-and-authorship.md) | Commits: Conventional, SSH-signed, owner only |
| [0019](0019-known-limits.md) | Known limits |
