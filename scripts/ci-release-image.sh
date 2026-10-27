#!/usr/bin/env bash
# CI helpers for the API image on GHCR (spec section 14). release.yml and
# ci.yml call them; they read GITHUB_TOKEN and GITHUB_ACTOR from the
# workflow and never print the token.
#
#   check-tag <tag> <sha>   the tag is vX.Y.Z, its commit <sha> is on
#                           origin/main, and the image tag is not on GHCR yet
#   check-kustomization     the digest pinned in deploy/k8s/kustomization.yaml
#                           is the zero placeholder, or the digest of a vX.Y.Z
#                           tag on GHCR
set -euo pipefail

registry=ghcr.io
repo=dizamu/rootcause-api
zero=sha256:0000000000000000000000000000000000000000000000000000000000000000
accept="application/vnd.oci.image.index.v1+json,application/vnd.oci.image.manifest.v1+json,application/vnd.docker.distribution.manifest.list.v2+json,application/vnd.docker.distribution.manifest.v2+json"

root="$(cd "$(dirname "$0")/.." && pwd)"
cd "$root"

die() { echo "ci-release-image: $*" >&2; exit 1; }

# curl_with <curl config line> <curl args...>: runs curl with one config line
# (a user or a header) read from stdin, so a credential never reaches a
# command line, where other processes could read it.
curl_with() {
	local line="$1"
	shift
	curl --config - "$@" <<<"$line"
}

# bearer prints a pull token for the repository, from GITHUB_TOKEN. GHCR
# answers 401 or 403 both when the token may not read the package and when
# the package does not exist yet (before the first release); bearer then
# prints nothing. Callers decide what that means.
bearer() {
	[[ -n "${GITHUB_TOKEN:-}" ]] || die "GITHUB_TOKEN is not set"
	local body code
	body="$(mktemp)"
	code="$(curl_with "user = \"${GITHUB_ACTOR:-x}:$GITHUB_TOKEN\"" -sS -o "$body" -w '%{http_code}' \
		"https://$registry/token?service=$registry&scope=repository:$repo:pull")" || { rm -f "$body"; die "GHCR token request failed"; }
	case "$code" in
	200) python3 -c 'import json, sys; print(json.load(sys.stdin).get("token", ""))' <"$body" ;;
	401 | 403) ;;
	*) rm -f "$body"; die "GHCR token request answered $code" ;;
	esac
	rm -f "$body"
}

# manifest_digest <token> <reference> prints the digest GHCR reports for a
# tag or digest, or nothing if it does not exist or may not be read.
manifest_digest() {
	local code headers
	headers="$(mktemp)"
	code="$(curl_with "header = \"Authorization: Bearer $1\"" -sS -o /dev/null -D "$headers" -w '%{http_code}' -I \
		-H "Accept: $accept" \
		"https://$registry/v2/$repo/manifests/$2")"
	case "$code" in
	200) tr -d '\r' <"$headers" | awk -F': ' 'tolower($1) == "docker-content-digest" { print $2 }' ;;
	401 | 403 | 404) ;;
	*) rm -f "$headers"; die "GHCR answered $code for $repo:$2" ;;
	esac
	rm -f "$headers"
}

# release_tags <token> prints the repository's vX.Y.Z tags.
release_tags() {
	local out
	out="$(curl_with "header = \"Authorization: Bearer $1\"" -fsS "https://$registry/v2/$repo/tags/list?n=1000")" || return 0
	python3 -c 'import json, sys; print("\n".join(json.load(sys.stdin).get("tags") or []))' <<<"$out" |
		grep -E '^v[0-9]+\.[0-9]+\.[0-9]+$' || true
}

check_tag() {
	local tag="$1" sha="$2"
	[[ "$tag" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]] || die "tag $tag is not vX.Y.Z"
	git rev-parse --verify -q origin/main >/dev/null || die "origin/main is not fetched (checkout needs fetch-depth: 0)"
	git merge-base --is-ancestor "$sha" origin/main || die "tag $tag points at $sha, which is not on main"
	local token
	token="$(bearer)"
	# No token is the first release: the package does not exist yet. If
	# instead the token may not read it, the push fails as well.
	if [[ -n "$token" && -n "$(manifest_digest "$token" "$tag")" ]]; then
		die "$registry/$repo:$tag already exists; release tags are never pushed twice"
	fi
	echo "tag $tag: vX.Y.Z, on main, not on GHCR yet"
}

check_kustomization() {
	local digest
	digest="$(awk '$1 == "newName:" && $2 == "'"$registry/$repo"'" { found = 1; next }
		found && $1 == "digest:" { print $2; exit }' deploy/k8s/kustomization.yaml)"
	[[ "$digest" =~ ^sha256:[0-9a-f]{64}$ ]] || die "no sha256 digest for $registry/$repo in deploy/k8s/kustomization.yaml"
	if grep -qE '^\s*newTag:' deploy/k8s/kustomization.yaml; then
		die "deploy/k8s/kustomization.yaml sets newTag; the release is pinned by digest only"
	fi
	if [[ "$digest" == "$zero" ]]; then
		echo "kustomization: zero placeholder, no release deployed yet"
		return
	fi
	local token tag
	token="$(bearer)"
	[[ -n "$token" ]] || die "$digest is pinned, but $registry/$repo cannot be read (no release yet, or no read access)"
	[[ -n "$(manifest_digest "$token" "$digest")" ]] || die "$registry/$repo@$digest does not exist"
	while read -r tag; do
		[[ -n "$tag" ]] || continue
		if [[ "$(manifest_digest "$token" "$tag")" == "$digest" ]]; then
			echo "kustomization: $digest is release $tag"
			return
		fi
	done < <(release_tags "$token")
	die "$registry/$repo@$digest exists but is no vX.Y.Z release"
}

case "${1:-}" in
check-tag)
	[[ $# -eq 3 ]] || die "usage: check-tag <tag> <sha>"
	check_tag "$2" "$3"
	;;
check-kustomization)
	[[ $# -eq 1 ]] || die "usage: check-kustomization"
	check_kustomization
	;;
*)
	echo "usage: ci-release-image.sh check-tag <tag> <sha> | check-kustomization" >&2
	exit 2
	;;
esac
