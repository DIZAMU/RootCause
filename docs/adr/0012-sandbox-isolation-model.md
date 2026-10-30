# ADR 0012 · The sandbox isolation model

Accepted on 2026-10-27.

## Context

A sandbox runs a deliberately broken machine with the learner as root. It must not reach the host, the rest of the system or the internet.

## Decision

In Docker mode, every sandbox gets the following (ARCHITECTURE.md section 13.1).
- `--cap-drop ALL`, with only allowlisted capabilities added back. These are CHOWN, DAC_OVERRIDE, FOWNER, FSETID, KILL, SETGID, SETUID and NET_BIND_SERVICE.
- `no-new-privileges`, `--pids-limit`, memory and CPU limits, and `--init`.
- Never the Docker socket, the host network or privileged mode.
- Network `none`, or an internal network with no route out.
- userns-remap (ADR 0010).

In Kubernetes mode there are four layers, and each one holds on its own.
1. The pod spec the runtime builds. It sets `hostUsers: false`, seccomp `RuntimeDefault`, drop ALL plus the allowlist, no privilege escalation, no service account token, no service links, only `emptyDir` volumes, `imagePullPolicy: Never`, no cluster DNS, and limits.
2. A ValidatingAdmissionPolicy on `sandboxes` that denies any pod breaking those rules, whoever creates it. That includes the API, Argo CD and an administrator. Argo CD manages the policy, so self-heal restores it (ADR 0016).
3. Pod Security Admission, `baseline` on `sandboxes` and `restricted` on `rootcause-system`. It stays on because it also covers procMount, sysctls, AppArmor, SELinux options and host ports, which the admission policy does not repeat. It should not be removed just because the policy exists.

   Today the layers overlap and the first one wins. API validation refuses host namespaces when `hostUsers` is false, and refuses privileged together with `allowPrivilegeEscalation: false`. Pod Security refuses hostPath, privileged containers, capabilities outside its baseline and unconfined seccomp before the policy runs. So the policy's rules b, c and d, and the matching parts of g and h, are backstops that only act if an earlier layer stops refusing. `TestSandboxAdmission` names the layer that must refuse each case and checks that layer's message. Pod Security is never turned off to test the backstops.
4. NetworkPolicies. Both `sandboxes` and `rootcause-system` are default-deny. A check runner may reach sandboxes on the internal network, and only runners may connect to them.

The API's service account holds a Role in `sandboxes` only, for pods (create, get, list, delete, exec and log). `scripts/k8s-prove.sh` checks an `auth can-i` matrix, the network blocks, the admission cases and the published ports.

## Consequences

- Every rule exists in the pod builder and is enforced again by admission and Pod Security, so one mistake does not open a sandbox.
- Giving an exercise a new capability, volume type or image source needs a change to the allowlist and to the policy, and a deliberate decision.
