#!/usr/bin/env bash
# test-commit-msg.sh checks .githooks/commit-msg against messages it must accept
# and messages it must reject. Exit 0 when every case behaves as expected.
set -uo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
hook="$root/.githooks/commit-msg"
tmp="$(mktemp)"
trap 'rm -f "$tmp"' EXIT
failures=0

# expect <accept|reject> <message>
expect() {
    printf '%s\n' "$2" > "$tmp"
    if bash "$hook" "$tmp" 2>/dev/null; then got=accept; else got=reject; fi
    if [ "$got" != "$1" ]; then
        echo "FAIL: expected $1, got $got: $2"
        failures=$((failures + 1))
    fi
}

expect accept "feat(cli): add play commands"
expect accept $'fix: handle empty config\n\nThe generated config is now validated.'
expect reject $'feat: x\n\nCo-authored-by: Someone <a@b.c>'
expect reject $'feat: x\n\n  co-authored-by: someone <a@b.c>'
expect reject $'feat: x\n\nGenerated with some tool'
expect reject $'feat: x\n\n\xf0\x9f\xa4\x96 Generated with [Some Tool](https://example.com)'
expect reject $'feat: x\n\nClaude-Session: https://claude.ai/code/session_0123'
expect reject $'feat: x\n\nSee https://claude.ai/code/session_0123'
expect accept $'docs: explain sessions\n\nA session is one attempt at one variant.'

if [ "$failures" -gt 0 ]; then
    echo "$failures case(s) failed"
    exit 1
fi
echo "ok: all commit-msg cases behave as expected"
