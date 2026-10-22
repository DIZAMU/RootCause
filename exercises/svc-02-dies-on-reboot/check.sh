#!/usr/bin/env bash
# check.sh runs after the engine has rebooted the sandbox (check.before:
# restart), so only what starts at boot is running. It passes when the
# vaktliste unit is active with the real app as its main process and /health
# answers on the variant's port. Anti-shortcut: starting the app by hand
# before the check does not survive the reboot. Exit 0 pass, 1 fail, 2
# could-not-run. One line of JSON on stdout.
set -uo pipefail

port="${RC_VAR_PORT:-}"
if [ -z "$port" ]; then
    echo '{"passed": false, "feedback": "check_error"}' >&2
    exit 2
fi

fail() { printf '{"passed": false, "feedback": "%s"}\n' "$1"; exit 1; }

systemctl is-active --quiet vaktliste.service || fail "not_running_after_reboot"

pid="$(systemctl show -p MainPID --value vaktliste.service)"
[ "${pid:-0}" -gt 0 ] || fail "not_running_after_reboot"
tr '\0' ' ' < "/proc/${pid}/cmdline" | grep -q '^/usr/bin/python3 /opt/vaktliste/app.py $' || fail "wrong_program"

# The unit is active; give the app a moment to bind its port.
body=""
for _ in $(seq 50); do
    body="$(curl -s --max-time 1 "http://127.0.0.1:${port}/health" 2>/dev/null)" && [ -n "$body" ] && break
    sleep 0.1
done
[ "$body" = '{"service": "vaktliste", "status": "ok"}' ] || fail "health_failed"

printf '{"passed": true, "feedback": ""}\n'
exit 0
