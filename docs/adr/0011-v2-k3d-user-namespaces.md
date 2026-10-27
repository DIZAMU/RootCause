# ADR 0011: v2 sandboxes: k3d with pod user namespaces (OQ-3 v2)

Status: Accepted (Phases 7 and 9). Recorded 2026-10-27.

## Context

v2 runs sandboxes as pods in a local k3d cluster. k3d runs its node containers privileged and in the host's user namespace, so node root is host root, whatever Docker's userns-remap says. Spec section 20 listed gVisor or user namespaces for v2.

## Decision

- Every sandbox pod runs with `hostUsers: false`: pod root maps to an unprivileged host UID range (verified on this kernel: pod root is host UID 3236954112 and up, not 0).
- Since Phase 9 a ValidatingAdmissionPolicy denies any pod in `sandboxes` without it, for every requester (ADR 0012).
- The privileged k3d nodes are accepted as a known limit for a local, single-user cluster, with the compensations in ADR 0012: user namespaces, the admission policy, Pod Security Admission, RBAC limited to pods in `sandboxes`, default-deny NetworkPolicies, no service account token in sandboxes, and nothing published beyond the host's 127.0.0.1.
- Rootless k3d (on rootless Docker or Podman) is the future fix for node root. It needs sudo for packages and subordinate ID setup, and k3d's support for it is experimental.

## Alternatives considered

- gVisor (runsc as a RuntimeClass): a new runtime in a custom k3d node image on every node, changed exec and cgroup behaviour, and it would not remove the privileged nodes.
- No isolation beyond Pod Security Admission: pod root would be host root.

## Consequences

- systemd cannot boot in a user-namespaced pod (the cgroup filesystem stays owned by host root), so `init: systemd` exercises are refused on Kubernetes (ADR 0019).
- A compromise of the node container itself is host root.
