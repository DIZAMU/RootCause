#!/usr/bin/env bash
# Proves the Phase 8 done-when criteria on the running cluster (spec section
# 18), after scripts/k8s-up.sh without --dev:
#
#   1. Argo CD's rootcause Application syncs deploy/k8s from main with
#      automated sync, prune and self-heal on, and is Synced and Healthy at
#      the commit main points at on GitHub. A forgotten --dev fails here.
#   2. The API pod runs the release pinned in deploy/k8s/kustomization.yaml,
#      by digest, and so does the dbt CronJob. With RC_RELEASE_DIGEST set
#      (from release.yml's job summary), that digest must match too.
#   3. A change made by hand is reverted: the sandboxes ResourceQuota is
#      patched, and Argo CD must put it back.
#   4. The rootcause project refuses an Application that points at another
#      namespace or another repository, and the Application tracks nothing
#      outside rootcause-system and sandboxes.
#   5. The objects k8s-up.sh creates outside git (rootcause-db, ghcr-pull,
#      api-to-apiserver, the namespaces) are still there: prune leaves them.
#
# The test Applications are removed at the end. Exit 0 only if every check
# passes.
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
cd "$root"
context=k3d-rootcause
kc() { kubectl --context "$context" "$@"; }
failures=0
pass() { echo "PASS  $*"; }
fail() { echo "FAIL  $*"; failures=$((failures + 1)); }
# same <got> <want> <what>: pass if they are equal.
same() {
	if [[ "$1" == "$2" ]]; then pass "$3"; else fail "$3: got '$1', want '$2'"; fi
}
# exists <kind> <name> <namespace>: pass if the object is there.
exists() {
	if kc get "$1" "$2" -n "$3" >/dev/null 2>&1; then pass "$1 $3/$2 kept"; else fail "$1 $3/$2 missing"; fi
}
repo_url=git@github.com:DIZAMU/RootCause.git
image=ghcr.io/dizamu/rootcause-api

# quota_saved is the sandboxes ResourceQuota's pod limit before the drift
# test. If the test ends without Argo CD putting it back (a timeout, or the
# script is stopped), cleanup restores it, so the proof never leaves a weaker
# limit behind.
quota_saved=""
restore_quota() {
	[[ -n "$quota_saved" ]] || return 0
	local now
	now="$(kc get resourcequota sandboxes -n sandboxes -o jsonpath='{.spec.hard.pods}' 2>/dev/null || true)"
	if [[ "$now" != "$quota_saved" ]]; then
		if kc patch resourcequota sandboxes -n sandboxes --type merge \
			-p "{\"spec\":{\"hard\":{\"pods\":\"$quota_saved\"}}}" >/dev/null 2>&1; then
			echo "note  ResourceQuota pods restored to $quota_saved by the script, not by Argo CD"
		else
			echo "FAIL  could not restore ResourceQuota pods to $quota_saved; it is $now"
		fi
	fi
	quota_saved=""
}

cleanup() {
	restore_quota
	kc delete application -n argocd rootcause-escape-ns rootcause-escape-repo \
		--ignore-not-found --wait=false >/dev/null 2>&1 || true
}
trap cleanup EXIT

app() { kc get application rootcause -n argocd -o jsonpath="$1"; }

echo "== 1. the Application"
if ! kc get application rootcause -n argocd >/dev/null 2>&1; then
	if ! kc get crd applications.argoproj.io >/dev/null 2>&1; then
		echo "note  Argo CD is not installed in this cluster"
	fi
	fail "no Application rootcause in argocd (run scripts/k8s-up.sh without --dev)"
	echo
	echo "k8s-gitops-prove: $failures check(s) failed"
	exit 1
fi
section1_failures="$failures"
src="$(app '{.spec.project} {.spec.source.repoURL} {.spec.source.targetRevision} {.spec.source.path}')"
if [[ "$src" == "rootcause $repo_url main deploy/k8s" ]]; then
	pass "project rootcause, $repo_url, main, deploy/k8s"
else
	fail "Application source is '$src'"
fi
policy="$(app '{.spec.syncPolicy.automated.prune} {.spec.syncPolicy.automated.selfHeal}')"
if [[ "$policy" == "true true" ]]; then
	pass "automated sync with prune and self-heal"
else
	fail "automated sync is off or incomplete (prune selfHeal: '$policy'); a run with --dev turns it off"
fi
main_sha="$(git ls-remote "$repo_url" refs/heads/main | cut -f1)"
[[ -n "$main_sha" ]] || main_sha="$(git ls-remote origin refs/heads/main | cut -f1)"
state=""
for _ in $(seq 60); do
	state="$(app '{.status.sync.status} {.status.health.status} {.status.sync.revision}')"
	[[ "$state" == "Synced Healthy $main_sha" ]] && break
	sleep 5
done
if [[ "$state" == "Synced Healthy $main_sha" ]]; then
	pass "Synced and Healthy at main ($main_sha)"
else
	fail "state '$state', main is $main_sha"
fi
# When section 1 fails, Argo CD's own conditions usually say why (a refused
# spec, an unreachable repository, a manifest that does not render).
if [[ "$failures" -gt "$section1_failures" ]]; then
	conditions="$(app '{range .status.conditions[*]}{.type}: {.message}{"\n"}{end}')"
	if [[ -n "$conditions" ]]; then
		echo "Argo CD conditions:"
		while IFS= read -r line; do
			echo "      $line"
		done <<<"$conditions"
	else
		echo "note  the Application reports no conditions"
	fi
fi

echo "== 2. the release"
revision="$(app '{.status.sync.revision}')"
pinned=""
if git cat-file -e "$revision" 2>/dev/null; then
	pinned="$(git show "$revision:deploy/k8s/kustomization.yaml" |
		awk '$1 == "newName:" && $2 == "'"$image"'" { found = 1; next } found && $1 == "digest:" { print $2; exit }')"
else
	fail "commit $revision is not in this clone (git fetch origin, then re-run)"
fi
if [[ "$pinned" =~ ^sha256:[0-9a-f]{64}$ && "$pinned" != sha256:0000000000000000000000000000000000000000000000000000000000000000 ]]; then
	pass "kustomization at $revision pins $image@$pinned"
else
	fail "kustomization at $revision pins '$pinned'"
fi
deployed="$(kc get deployment rootcause-api -n rootcause-system -o jsonpath='{.spec.template.spec.containers[0].image}')"
cron="$(kc get cronjob dbt -n rootcause-system -o jsonpath='{.spec.jobTemplate.spec.template.spec.initContainers[0].image}')"
same "$deployed" "$image@$pinned" "Deployment image"
same "$cron" "$image@$pinned" "dbt CronJob image"
running="$(kc get pods -n rootcause-system -l app=rootcause-api,rootcause.instance=main \
	-o jsonpath='{range .items[*]}{.status.containerStatuses[0].imageID}{"\n"}{end}')"
if [[ "$(wc -l <<<"$running")" -eq 1 && "${running##*@}" == "$pinned" ]]; then
	pass "the API pod runs $pinned"
else
	fail "the API pod(s) run: $running"
fi
if [[ -n "${RC_RELEASE_DIGEST:-}" ]]; then
	same "$pinned" "$RC_RELEASE_DIGEST" "pinned digest equals RC_RELEASE_DIGEST"
else
	echo "note  RC_RELEASE_DIGEST not set; the release summary's digest was not compared"
fi

echo "== 3. drift is reverted"
want="$(kc get resourcequota sandboxes -n sandboxes -o jsonpath='{.spec.hard.pods}')"
quota_saved="$want"
kc patch resourcequota sandboxes -n sandboxes --type merge -p '{"spec":{"hard":{"pods":"40"}}}' >/dev/null
got=""
for _ in $(seq 60); do
	got="$(kc get resourcequota sandboxes -n sandboxes -o jsonpath='{.spec.hard.pods}')"
	[[ "$got" == "$want" ]] && break
	sleep 2
done
if [[ "$want" == 4 && "$got" == "$want" ]]; then
	quota_saved=""
	pass "ResourceQuota pods patched to 40, Argo CD put back $got"
else
	fail "ResourceQuota pods is $got two minutes after the patch, git says 4"
	restore_quota
fi

echo "== 4. the project's limits"
# escape <name> <repo> <namespace>: an Application in project rootcause that
# Argo CD must refuse with an InvalidSpecError and never sync.
escape() {
	kc apply -f - >/dev/null <<YAML
apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  name: $1
  namespace: argocd
spec:
  project: rootcause
  source:
    repoURL: $2
    targetRevision: main
    path: deploy/k8s
  destination:
    server: https://kubernetes.default.svc
    namespace: $3
YAML
	local cond=""
	for _ in $(seq 30); do
		cond="$(kc get application "$1" -n argocd -o jsonpath='{range .status.conditions[*]}{.type}={.message}{"\n"}{end}')"
		[[ "$cond" == *InvalidSpecError* ]] && break
		sleep 2
	done
	local synced
	synced="$(kc get application "$1" -n argocd -o jsonpath='{.status.sync.status}')"
	if [[ "$cond" == *InvalidSpecError* && "$synced" != Synced ]]; then
		pass "$1 refused: $(grep -m1 InvalidSpecError <<<"$cond" | cut -d= -f2-)"
	else
		fail "$1 not refused (conditions: '$cond', sync '$synced')"
	fi
	kc delete application "$1" -n argocd --wait=false >/dev/null
}
# kube-system holds nothing of RootCause's; the Application lists it as the
# default namespace, which the project does not allow.
escape rootcause-escape-ns "$repo_url" kube-system
escape rootcause-escape-repo https://github.com/argoproj/argocd-example-apps.git rootcause-system
outside="$(app '{range .status.resources[*]}{.namespace}{"\n"}{end}' | sort -u | grep -vxE 'rootcause-system|sandboxes' || true)"
if [[ -z "$outside" ]]; then
	pass "every tracked resource is in rootcause-system or sandboxes"
else
	fail "tracked resources outside the project's namespaces: $outside"
fi

echo "== 5. objects outside git"
for obj in "secret rootcause-db rootcause-system" "secret ghcr-pull rootcause-system" \
	"networkpolicy api-to-apiserver rootcause-system" "secret repo-rootcause argocd"; do
	read -r kind name ns <<<"$obj"
	exists "$kind" "$name" "$ns"
done
for ns in rootcause-system sandboxes; do
	if kc get namespace "$ns" >/dev/null 2>&1; then pass "namespace $ns kept"; else fail "namespace $ns missing"; fi
done

echo
if [[ "$failures" -gt 0 ]]; then
	echo "k8s-gitops-prove: $failures check(s) failed"
	exit 1
fi
echo "k8s-gitops-prove: every check passed"
