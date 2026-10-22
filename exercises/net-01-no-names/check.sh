#!/usr/bin/env bash
# check.sh passes when the internal names resolve to their addresses through
# the system's normal lookup (getent, which follows nsswitch.conf and
# resolv.conf), the way every program on the server looks names up.
# Anti-shortcut: the zone's names must not be written into /etc/hosts.
# Exit 0 pass, 1 fail, 2 could-not-run. One line of JSON on stdout.
set -uo pipefail

zone="${RC_VAR_ZONE:-}"
if [ -z "$zone" ]; then
    echo '{"passed": false, "feedback": "check_error"}' >&2
    exit 2
fi

fail() { printf '{"passed": false, "feedback": "%s"}\n' "$1"; exit 1; }

if grep -qiF "$zone" /etc/hosts; then
    fail "hosts_file_used"
fi

while read -r name ip; do
    got="$(getent hosts "${name}.${zone}" | awk '{print $1; exit}')"
    [ -n "$got" ] || fail "names_unresolved"
    [ "$got" = "$ip" ] || fail "wrong_address"
done <<LIST
fil01 10.20.0.11
print01 10.20.0.12
intranett 10.20.0.20
LIST

printf '{"passed": true, "feedback": ""}\n'
exit 0
