#!/usr/bin/env bash
# Watches Argo CD roll out a merge on its own (spec section 14): start it
# before you merge a pull request that changes deploy/k8s, then merge and run
# nothing else. It records the revision the rootcause Application is synced
# to, waits until main on GitHub moves past it, then until Argo CD reports
# Synced and Healthy at the new commit, and prints both revisions with times.
# It only reads; it never applies or syncs anything.
#
# Usage: scripts/k8s-watch-sync.sh [timeout-minutes]   (default 15)
set -euo pipefail

context=k3d-rootcause
kc() { kubectl --context "$context" "$@"; }
repo_url=git@github.com:DIZAMU/RootCause.git
timeout_min="${1:-15}"
deadline=$(($(date +%s) + timeout_min * 60))

app() { kc get application rootcause -n argocd -o jsonpath="$1"; }
now() { date '+%Y-%m-%d %H:%M:%S'; }
main_sha() { git ls-remote "$repo_url" refs/heads/main | cut -f1; }

before="$(app '{.status.sync.revision}')"
[[ -n "$before" ]] || { echo "k8s-watch-sync: the rootcause Application has no synced revision" >&2; exit 1; }
policy="$(app '{.spec.syncPolicy.automated.prune} {.spec.syncPolicy.automated.selfHeal}')"
[[ "$policy" == "true true" ]] || { echo "k8s-watch-sync: automated sync is off; run scripts/k8s-up.sh without --dev first" >&2; exit 1; }
echo "$(now)  before: Argo CD synced at $before. Merge now; run nothing else."

target=""
while [[ -z "$target" ]]; do
	sha="$(main_sha)"
	if [[ -n "$sha" && "$sha" != "$before" ]]; then
		target="$sha"
		echo "$(now)  main moved to $target"
		break
	fi
	[[ "$(date +%s)" -lt "$deadline" ]] || { echo "$(now)  main did not move within $timeout_min minutes" >&2; exit 1; }
	sleep 10
done

state=""
while :; do
	state="$(app '{.status.sync.status} {.status.health.status} {.status.sync.revision}')"
	if [[ "$state" == "Synced Healthy $target" ]]; then
		echo "$(now)  after: Argo CD synced at $target, Synced and Healthy"
		echo "before $before"
		echo "after  $target"
		exit 0
	fi
	[[ "$(date +%s)" -lt "$deadline" ]] || break
	sleep 5
done
echo "$(now)  Argo CD did not reach Synced/Healthy at $target within $timeout_min minutes (state: $state)" >&2
app '{range .status.conditions[*]}{.type}: {.message}{"\n"}{end}' >&2
exit 1
