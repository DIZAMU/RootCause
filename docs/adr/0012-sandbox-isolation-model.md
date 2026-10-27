# ADR 0012: The sandbox isolation model

Status: Accepted (Phases 2, 7 and 9). Recorded 2026-10-27.

## Context

A sandbox runs a deliberately broken machine with the learner as root. It must not reach the host, the rest of the system or the internet.

## Decision

v1 (Docker), on every sandbox (spec section 13.1):
- `--cap-drop ALL` and only allowlisted capabilities back (CHOWN, DAC_OVERRIDE, FOWNER, FSETID, KILL, SETGID, SETUID, NET_BIND_SERVICE);
- `no-new-privileges`, `--pids-limit`, memory and CPU limits, `--init`;
- never the Docker socket, the host network or privileged mode;
- network `none`, or an internal network with no outside route;
- userns-remap (ADR 0010).

v2 (Kubernetes), four layers that each hold on their own:
1. The pod spec the runtime builds: `hostUsers: false`, seccomp `RuntimeDefault`, drop ALL plus the allowlist, no privilege escalation, no service account token, no service links, only `emptyDir` volumes, `imagePullPolicy: Never`, no cluster DNS, limits.
2. A ValidatingAdmissionPolicy on `sandboxes` (Phase 9) that denies any pod breaking those rules, whoever creates it: the API, Argo CD or an administrator. It is managed by Argo CD, so self-heal restores it (ADR 0016).
3. Pod Security Admission `baseline` on `sandboxes` and `restricted` on `rootcause-system`. It stays on: it also covers procMount, sysctls, AppArmor, SELinux options and host ports, which the admission policy does not repeat. Do not remove it because the policy exists.

Today the layers overlap, and the first one wins. API validation refuses host namespaces when `hostUsers` is false, and privileged with `allowPrivilegeEscalation: false`. Pod Security refuses hostPath, privileged containers, capabilities outside its baseline and unconfined seccomp before the policy runs. So the policy's rules b, c and d, and the matching parts of g and h, are backstops: they only act if an earlier layer stops refusing. `TestSandboxAdmission` names the layer that must refuse each case and checks that layer's message. Pod Security is never turned off to test the backstops.
4. NetworkPolicies: default-deny in `sandboxes` and `rootcause-system`; a check runner may reach sandboxes on the internal network, and only runners may connect to them.

The API's service account holds a Role in `sandboxes` only: pods create, get, list, delete, exec and log. `scripts/k8s-prove.sh` checks an `auth can-i` matrix, the network blocks, the admission cases and the published ports.

## Consequences

- Every rule exists in the pod builder, and is enforced again by admission and PSA, so one mistake does not open a sandbox.
- Adding a capability, a volume type or an image source to an exercise needs a change to the allowlist, the policy and an owner decision.
