#!/usr/bin/env bash
# check.sh grades the end state of the share. It passes when both department
# members can create a file in the share and edit the existing files, the
# original files are still there, and the share is closed to everyone else:
# not world-writable, and the outsider "per" cannot write. Exit 0 pass, 1
# fail, 2 could-not-run. One line of JSON on stdout.
set -uo pipefail

dept="${RC_VAR_DEPT:-}"
member="${RC_VAR_MEMBER:-}"
colleague="${RC_VAR_COLLEAGUE:-}"
if [ -z "$dept" ] || [ -z "$member" ] || [ -z "$colleague" ]; then
    echo '{"passed": false, "feedback": "check_error"}' >&2
    exit 2
fi
share="/srv/shares/$dept"

fail() { printf '{"passed": false, "feedback": "%s"}\n' "$1"; exit 1; }

# The original files must still exist with their content.
grep -q '^Budsjett 2027' "$share/budsjett.txt" 2>/dev/null || fail "files_missing"
grep -q '^Rapport Q3' "$share/rapport.txt" 2>/dev/null || fail "files_missing"

# Anti-shortcut: opening the share to everyone is not a fix.
if [ -n "$(find "$share" -perm -o+w -print -quit)" ]; then
    fail "share_too_open"
fi
probe="$share/.rc-probe-$$"
if runuser -u per -- touch "$probe" 2>/dev/null; then
    rm -f "$probe"
    fail "share_too_open"
fi

# Both members can create a file and edit every existing file. Each member
# is checked, so giving the share to one user is not a fix.
for user in "$member" "$colleague"; do
    if ! runuser -u "$user" -- touch "$probe" 2>/dev/null; then
        fail "member_cannot_save"
    fi
    rm -f "$probe"
    for f in "$share"/budsjett.txt "$share"/rapport.txt; do
        runuser -u "$user" -- test -w "$f" || fail "member_cannot_edit"
    done
done

printf '{"passed": true, "feedback": ""}\n'
exit 0
