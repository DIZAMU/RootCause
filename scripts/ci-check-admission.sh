#!/usr/bin/env bash
# Checks that deploy/k8s, as Argo CD renders it, holds the admission policy
# for sandbox pods in full (spec section 13.2): every rule's message from a
# to l, failurePolicy Fail, and a binding with validationActions exactly
# [Deny], scoped to sandboxes, with no matchConditions that could exempt a
# requester. ci.yml runs it with the pinned kubectl from
# scripts/install-k8s-tools.sh.
#
# It needs no cluster and no Python packages: kubectl renders the
# kustomization, `kubectl label --local` turns each object into JSON without
# contacting a server (the label it adds stays in this pipe), and Python's
# standard json module reads the result.
#
# Usage: scripts/ci-check-admission.sh [kubectl]   (default: kubectl on PATH)
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
cd "$root"
kubectl="${1:-kubectl}"

"$kubectl" kustomize deploy/k8s |
	"$kubectl" label --local -f - -o json rootcause.ci/rendered=true |
	python3 -c '
import json, sys
text, decoder, docs, i = sys.stdin.read(), json.JSONDecoder(), [], 0
while i < len(text):
    if text[i].isspace():
        i += 1
        continue
    doc, i = decoder.raw_decode(text, i)
    docs.extend(doc["items"] if doc.get("kind") == "List" else [doc])
def one(kind):
    found = [d for d in docs if d.get("kind") == kind and d["metadata"]["name"] == "rootcause-sandbox-pods"]
    if len(found) != 1:
        sys.exit(f"ci-check-admission: want one {kind} rootcause-sandbox-pods, found {len(found)}")
    return found[0]
policy, binding = one("ValidatingAdmissionPolicy"), one("ValidatingAdmissionPolicyBinding")
problems = []
spec = policy["spec"]
failure_policy = spec.get("failurePolicy")
if failure_policy != "Fail":
    problems.append(f"policy failurePolicy is {failure_policy!r}, want Fail")
if spec.get("matchConditions"):
    problems.append("policy has matchConditions")
messages = " ".join(v.get("message", "") for v in spec.get("validations", []))
rules = {
    "a": "must set hostUsers to false",
    "b": "may not use the host network, PID or IPC namespace",
    "c": "may not mount a hostPath volume",
    "d": "may be privileged",
    "e": "must set allowPrivilegeEscalation to false",
    "f": "must drop ALL capabilities",
    "g": "may add back only CHOWN, DAC_OVERRIDE, FOWNER, FSETID, KILL, SETGID, SETUID and NET_BIND_SERVICE",
    "h": "RuntimeDefault seccomp profile",
    "i": "must set automountServiceAccountToken to false",
    "j": "must use imagePullPolicy Never",
    "k": "must be a rootcause/ exercise image",
    "l": "may use only emptyDir volumes",
}
for key, text in rules.items():
    if text not in messages:
        problems.append(f"rule {key} is missing (no message with {text!r})")
selector = spec.get("matchConstraints", {}).get("namespaceSelector", {}).get("matchLabels", {})
if selector != {"kubernetes.io/metadata.name": "sandboxes"}:
    problems.append(f"policy namespaceSelector is {selector}, want sandboxes only")
b = binding["spec"]
policy_name, actions = b.get("policyName"), b.get("validationActions")
if policy_name != "rootcause-sandbox-pods":
    problems.append(f"binding policyName is {policy_name!r}")
if actions != ["Deny"]:
    problems.append(f"binding validationActions is {actions}, want exactly [Deny]")
if b.get("matchConditions"):
    problems.append("binding has matchConditions")
if b.get("matchResources", {}).get("namespaceSelector", {}).get("matchLabels", {}) != {"kubernetes.io/metadata.name": "sandboxes"}:
    problems.append("binding is not scoped to sandboxes")
if problems:
    sys.exit("ci-check-admission:\n  " + "\n  ".join(problems))
print(f"admission policy: {len(rules)} rules, failurePolicy Fail, binding [Deny], no exemptions")
'
