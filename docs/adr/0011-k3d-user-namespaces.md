# ADR 0011 · Kubernetes mode sandboxes use pod user namespaces in k3d

Accepted on 2026-10-27.

## Context

Kubernetes mode runs sandboxes as pods in a local k3d cluster. k3d runs its node containers privileged and in the host's user namespace, so node root is host root, whatever Docker's userns-remap says. The options considered for isolating sandboxes were gVisor and user namespaces.

## Decision

- Every sandbox pod runs with `hostUsers: false`. Pod root maps to an unprivileged range of 65536 host UIDs, different for each pod and never 0. `scripts/k8s-prove.sh` checks this.
- A ValidatingAdmissionPolicy denies any pod in `sandboxes` without it, for every requester (ADR 0012).
- The privileged k3d nodes are accepted as a known limit for a local, single-user cluster. The compensations are listed in ADR 0012. They are user namespaces, the admission policy, Pod Security Admission, RBAC limited to pods in `sandboxes`, default-deny NetworkPolicies, no service account token in sandboxes, and nothing published beyond the host's 127.0.0.1.
- Rootless k3d, on rootless Docker or Podman, would remove node root. It needs sudo for packages and subordinate ID setup, and k3d's support for it is still experimental.

## Alternatives considered

- gVisor as a RuntimeClass. It needs a new runtime in a custom k3d node image on every node, changes exec and cgroup behaviour, and would not remove the privileged nodes.
- No isolation beyond Pod Security Admission. Pod root would then be host root.

## Consequences

- systemd cannot boot in a pod with its own user namespace, because the cgroup filesystem stays owned by host root. Exercises with `init: systemd` are refused in Kubernetes mode (ADR 0019).
- A compromise of the node container itself would mean host root.
