# ADR 0019 · Known limits

Accepted on 2026-10-27.

## Context

Some limits were found and accepted rather than fixed. They are written down so nobody mistakes them for guarantees.

## Decision

These are accepted for a single-user practice tool that runs locally.
- Checks run inside the sandbox, where the learner is root. A determined learner could replace a tool the check uses and fake a pass. Exercises where this matters run the check from a separate runner.
- `check.before: restart` restarts the container. Files stay, but only PID 1 starts again and setup.sh does not run again. Services that must start at boot use `init: systemd`.
- In Kubernetes mode there is no restart that keeps files, and `init: systemd` exercises (svc-02) are refused, because systemd cannot boot in a pod with its own user namespace.
- A check runner can reach any sandbox on the internal network, not only its own. With at most two sessions this is accepted.
- While a check runs, the learner could read check.sh and the seed and variant values it receives, since both run as root in the same sandbox. It only spoils the learner's own practice, and the scripts are public anyway.
- A sandbox's writable filesystem has no size limit, so a learner could fill the host's disk. Docker limits it only on XFS with project quotas, which most hosts do not use.
- k3d's nodes are privileged and run as host root (ADR 0011).
- The cluster's dbt CronJob relies on dbt's own exit code, without the node count check.
- The workflows and the commit check run from a pull request's own commits, so a pull request could weaken the checks that judge it. Changes to `.github/`, `scripts/` and `.githooks/` therefore need careful review.
- Early on, k3d published the Kubernetes API on the host's 0.0.0.0. Under WSL2's NAT mode it was reachable from Windows but not from the network, and it still required the kubeconfig's client certificate. `deploy/k8s/k3d.yaml` now binds it to 127.0.0.1, and `scripts/k8s-prove.sh` checks every published port.

## Consequences

- Each limit has a test or a script check where one is possible.
- Removing a limit needs a design change, recorded in a new ADR.
