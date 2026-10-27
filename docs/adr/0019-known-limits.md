# ADR 0019: Known limits

Status: Accepted (Phases 2 to 9). Recorded 2026-10-27.

## Context

Some limits were found and accepted rather than fixed. They are written down so nobody mistakes them for guarantees.

## Decision

Accepted for a single-user local practice tool:
- Checks run inside the sandbox, where the learner is root. A determined learner could replace a tool the check uses and fake a pass. Exercises where it matters run the check from a runner.
- `check.before: restart` restarts the container: files stay, but only PID 1 starts again, and setup.sh does not run again. Services that must start at boot use `init: systemd`.
- On Kubernetes there is no restart that keeps files, and `init: systemd` exercises (svc-02) are refused: systemd cannot boot in a user-namespaced pod.
- A check runner may reach any sandbox on the internal network, not only its own. With at most two sessions this is accepted.
- k3d's nodes are privileged and run as host root (ADR 0011).
- The cluster's dbt CronJob relies on dbt's own exit code, without the node-count check.
- Until Phase 9 the k3d Kubernetes API was published on the host's 0.0.0.0. Under WSL2's NAT mode it was reachable from Windows, not from the network, and still required the kubeconfig's client certificate; it broke the project's own 127.0.0.1 rule. `deploy/k8s/k3d.yaml` now binds it to 127.0.0.1, and `scripts/k8s-prove.sh` checks every published port.

## Consequences

- Each limit has a test or a script check where one is possible.
- Removing a limit needs a design change, recorded in a new ADR.
