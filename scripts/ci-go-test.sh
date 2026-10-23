#!/usr/bin/env bash
# ci-go-test.sh runs go test for CI and fails when tests did not really run:
#
#   - go test fails, or
#   - any test is skipped (except the ones in allowed_skips), or
#   - a test named with --require did not pass, or
#   - a package that has test files on origin/main reports "no test files".
#
# Integration tests skip only when their database or Docker was never
# configured, so in CI a skip means a broken setup, not a pass.
#
# Usage: scripts/ci-go-test.sh [--require TestName]... [go test args]...
# Example: scripts/ci-go-test.sh --require TestScriptedSelection -tags webdist ./...
set -euo pipefail

# Skipped unless asked for on purpose (RC_DBT_STRESS=1).
allowed_skips=" TestBuildStress "

required=()
while [ "${1:-}" = "--require" ]; do
    required+=("${2:?--require needs a test name}")
    shift 2
done
[ "$#" -gt 0 ] || { echo "usage: ci-go-test.sh [--require TestName]... [go test args]..." >&2; exit 2; }

root="$(cd "$(dirname "$0")/.." && pwd)"
log="$(mktemp)"
trap 'rm -f "$log"' EXIT

cd "$root"
status=0
go test -count=1 -v "$@" 2>&1 | tee "$log" || status=$?
if [ "$status" -ne 0 ]; then
    echo "ci-go-test: go test failed (exit $status)" >&2
    exit 1
fi

failed=0

while read -r name; do
    case "$allowed_skips" in
        *" $name "*) ;;
        *) echo "ci-go-test: $name was skipped" >&2; failed=1 ;;
    esac
done < <(sed -nE 's/^[[:space:]]*--- SKIP: ([^ ]+).*/\1/p' "$log")

for name in "${required[@]}"; do
    if ! grep -qE "^[[:space:]]*--- PASS: ${name} " "$log"; then
        echo "ci-go-test: required test $name did not pass" >&2
        failed=1
    fi
done

if git rev-parse --verify --quiet origin/main >/dev/null; then
    module="$(go list -m)"
    while read -r dir; do
        [ -d "$dir" ] || continue
        pkg="$module"
        [ "$dir" = "." ] || pkg="$module/$dir"
        if grep -qE "^\?[[:space:]]+${pkg}[[:space:]]+\[no test files\]" "$log"; then
            echo "ci-go-test: $pkg has tests on origin/main but ran none" >&2
            failed=1
        fi
    done < <(git ls-tree -r --name-only origin/main | grep '_test\.go$' | xargs -r -n1 dirname | sort -u)
else
    echo "ci-go-test: origin/main not fetched; skipping the no-test-files check" >&2
fi

if [ "$failed" -ne 0 ]; then
    exit 1
fi
echo "ci-go-test: ok"
