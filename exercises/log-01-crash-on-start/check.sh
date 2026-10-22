#!/usr/bin/env bash
# check.sh passes when lagerapp runs as its own user and its /health endpoint
# reports the correct configuration: status ok, the variant's port, the real
# data directory and all three stock files. Anti-shortcut: a stub server, an
# empty directory at the wrong path, or the app run as root all fail.
# Exit 0 pass, 1 fail, 2 could-not-run. One line of JSON on stdout.
set -uo pipefail

port="${RC_VAR_PORT:-}"
if [ -z "$port" ]; then
    echo '{"passed": false, "feedback": "check_error"}' >&2
    exit 2
fi

fail() { printf '{"passed": false, "feedback": "%s"}\n' "$1"; exit 1; }

pgrep -u lagerapp -f '^/usr/bin/python3 /opt/lagerapp/app.py$' >/dev/null || fail "app_not_running"

health="$(curl -s --max-time 3 "http://127.0.0.1:${port}/health" 2>/dev/null)"
[ -n "$health" ] || fail "health_unreachable"

want="{\"data_dir\": \"/var/lib/lagerapp\", \"port\": ${port}, \"records\": 3, \"status\": \"ok\"}"
[ "$health" = "$want" ] || fail "wrong_config_loaded"

printf '{"passed": true, "feedback": ""}\n'
exit 0
