# ADR 0016: Argo CD is a trust boundary

Status: Accepted (Phases 8 and 9). Recorded 2026-10-27.

## Context

Argo CD's application controller has cluster-wide write through its ClusterRole. Whatever is on `main` under `deploy/k8s`, it applies.

## Decision

- The AppProject `rootcause` limits what it is asked to do: this repository as the only source, `rootcause-system` and `sandboxes` as the only destinations, only the namespaced kinds `deploy/k8s` uses, and as the only cluster-wide kinds the ValidatingAdmissionPolicy and its binding. The core install has no `default` project.
- The admission policy on `sandboxes` binds to every requester, Argo CD's controller included.
- The policy is managed by Argo CD (Phase 9, D1): the whitelist grants Argo CD no power it lacks, and self-heal restores a deleted or weakened policy within seconds. A bad commit could weaken it, as it could the Role or the NetworkPolicies today; CI checks the rendered policy's rules, its Fail policy and its Deny binding.
- Argo CD reads the repository with a read-only deploy key and pulls nothing else.

## Consequences

- The trust in Argo CD equals the trust in `main`: signed commits, review and green checks.
- `scripts/k8s-gitops-prove.sh` proves the project refuses another namespace and another repository.
