#!/usr/bin/env bash
# ci-changed-exercises.sh prints the ids of the exercises CI must validate,
# one per line (spec section 14):
#
#   all          every exercise
#   <base> <head>  the exercises with a file changed between the two commits.
#                Every exercise when the validation itself changed (this
#                script, validate-exercise.sh or exercises.yml).
#
# Exercises deleted in the change are left out.
#
# Usage: scripts/ci-changed-exercises.sh all
#        scripts/ci-changed-exercises.sh <base> <head>
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
cd "$root"

all() {
    for f in exercises/*/exercise.yaml; do
        basename "$(dirname "$f")"
    done
}

case "$#" in
    1)
        [ "$1" = all ] || { echo "usage: ci-changed-exercises.sh all | <base> <head>" >&2; exit 2; }
        all
        ;;
    2)
        changed="$(git diff --name-only "$1" "$2")"
        if grep -qxE 'scripts/ci-changed-exercises\.sh|scripts/validate-exercise\.sh|\.github/workflows/exercises\.yml' <<<"$changed"; then
            all
            exit 0
        fi
        sed -nE 's#^exercises/([^/]+)/.*#\1#p' <<<"$changed" | sort -u | while read -r id; do
            if [ -f "exercises/$id/exercise.yaml" ]; then
                echo "$id"
            fi
        done
        ;;
    *)
        echo "usage: ci-changed-exercises.sh all | <base> <head>" >&2
        exit 2
        ;;
esac
