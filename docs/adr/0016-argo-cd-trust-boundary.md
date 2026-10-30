# ADR 0016 · Argo CD is a trust boundary

Accepted on 2026-10-27.

## Context

Argo CD's application controller can write anywhere in the cluster through its ClusterRole, and it applies whatever is on `main` under `deploy/k8s`.

## Decision

- The AppProject `rootcause` limits what it is asked to do. This repository is the only source and `rootcause-system` and `sandboxes` are the only destinations. Only the namespaced kinds `deploy/k8s` uses are allowed, and the only cluster-wide kinds are the ValidatingAdmissionPolicy and its binding. The core install has no `default` project.
- The admission policy on `sandboxes` binds to every requester, Argo CD's controller included.
- Argo CD manages the policy itself. The allowlist gives Argo CD no power it did not already have, and self-heal restores a deleted or weakened policy within seconds. A bad commit could weaken the policy, just as it could weaken the Role or the NetworkPolicies, so CI checks the rendered policy's rules, its Fail policy and its Deny binding.
- Argo CD reads the repository with a read-only deploy key and pulls nothing else.

## Consequences

- Trusting Argo CD means trusting `main`, which is protected by signed commits, review and green checks.
- `scripts/k8s-gitops-prove.sh` proves that the project refuses another namespace and another repository.
