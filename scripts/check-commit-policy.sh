#!/usr/bin/env bash
# check-commit-policy.sh checks every commit in <base>..<head> against the
# authorship rules (spec section 15):
#
#   1. The message passes .githooks/commit-msg, the same check the local hook
#      runs: no Co-authored-by trailer, no tool attribution line.
#   2. Author and committer email are the owner's GitHub noreply address.
#      The one exception: a merge commit that GitHub itself created in the
#      branch ("Update branch"), whose committer is GitHub. Its committer
#      check is skipped, and the output says so. Its author and signature are
#      still checked.
#   3. The commit is signed and verified:
#        --signatures api  GitHub's verification (commit.verification), the
#                          "Verified" badge. Needs gh, GITHUB_TOKEN and
#                          GITHUB_REPOSITORY. The default on GitHub Actions.
#        --signatures git  git's own check (%G? must be G), against the
#                          local gpg.ssh.allowedSignersFile. The default
#                          elsewhere. It cannot check the merge commits
#                          GitHub signs with its own GPG key (status E).
#
# Usage: scripts/check-commit-policy.sh [--signatures api|git] <base> <head>
set -euo pipefail

owner_email="117295462+DIZAMU@users.noreply.github.com"
github_committer="noreply@github.com"

mode=git
if [ "${GITHUB_ACTIONS:-}" = true ]; then
    mode=api
fi
if [ "${1:-}" = --signatures ]; then
    mode="${2:?--signatures needs api or git}"
    shift 2
fi
case "$mode" in api|git) ;; *) echo "unknown signature mode: $mode" >&2; exit 2 ;; esac
[ "$#" -eq 2 ] || { echo "usage: check-commit-policy.sh [--signatures api|git] <base> <head>" >&2; exit 2; }
base="$1" head="$2"

root="$(cd "$(dirname "$0")/.." && pwd)"
hook="$root/.githooks/commit-msg"
msg="$(mktemp)"
trap 'rm -f "$msg"' EXIT

commits="$(git rev-list --reverse "$base..$head")"
if [ -z "$commits" ]; then
    echo "no commits in $base..$head"
    exit 0
fi

failures=0
fail() {
    echo "FAIL $1: $2"
    failures=$((failures + 1))
}

for sha in $commits; do
    short="${sha:0:12}"
    before=$failures
    IFS=$'\t' read -r author committer committer_name parents <<<"$(git log -1 --format=$'%ae\t%ce\t%cn\t%p' "$sha")"
    nparents=$(wc -w <<<"$parents")

    git log -1 --format=%B "$sha" > "$msg"
    if ! out="$(bash "$hook" "$msg" 2>&1)"; then
        fail "$short" "message: ${out#commit-msg: }"
    fi

    if [ "$author" != "$owner_email" ]; then
        fail "$short" "author email $author is not the owner's"
    fi
    if [ "$committer" != "$owner_email" ]; then
        if [ "$committer" = "$github_committer" ] && [ "$nparents" -gt 1 ]; then
            echo "note $short: merge commit created by GitHub (committer $committer_name <$committer>); committer check skipped"
        else
            fail "$short" "committer email $committer is not the owner's"
        fi
    fi

    if [ "$mode" = api ]; then
        if ! v="$(gh api "repos/$GITHUB_REPOSITORY/commits/$sha" --jq '.commit.verification | "\(.verified) \(.reason)"')"; then
            fail "$short" "could not read GitHub's signature verification"
        elif [ "${v%% *}" != true ]; then
            fail "$short" "signature not verified by GitHub (reason: ${v#* })"
        fi
    else
        status="$(git log -1 --format=%G? "$sha")"
        if [ "$status" != G ]; then
            fail "$short" "signature status $status, want G (good signature from an allowed signer)"
        fi
    fi

    if [ "$failures" -eq "$before" ]; then
        echo "ok   $short $(git log -1 --format=%s "$sha")"
    fi
done

if [ "$failures" -gt 0 ]; then
    echo "$failures problem(s) found. See spec section 15."
    exit 1
fi
echo "ok: every commit in $base..$head follows the commit policy"
