#!/usr/bin/env bash
# check.sh runs on a separate runner machine on the same network, never inside
# the sandbox, so a site that answers only on the server's own loopback fails.
# It passes when the original page is served at the variant's port.
# RC_SANDBOX_HOST is the sandbox's host name. Exit 0 pass, 1 fail, 2
# could-not-run. One line of JSON on stdout.
set -uo pipefail

host="${RC_SANDBOX_HOST:-}"
port="${RC_VAR_PORT:-}"
if [ -z "$host" ] || [ -z "$port" ]; then
    echo '{"passed": false, "feedback": "check_error"}' >&2
    exit 2
fi

fail() { printf '{"passed": false, "feedback": "%s"}\n' "$1"; exit 1; }

body="$(curl -s --max-time 3 "http://${host}:${port}/" 2>/dev/null)"
[ -n "$body" ] || fail "unreachable_from_outside"
printf '%s' "$body" | grep -q 'rootcause:net-02:original' || fail "wrong_content"

printf '{"passed": true, "feedback": ""}\n'
exit 0
