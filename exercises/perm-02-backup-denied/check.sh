#!/usr/bin/env bash
# check.sh runs the backup the way the nightly job does, as the backup user,
# and passes when it produces a new archive owned by backup that holds the
# data. Anti-shortcuts: the backup user must not be root, and neither the
# script nor the target folder may be world-writable. Exit 0 pass, 1 fail,
# 2 could-not-run. One line of JSON on stdout.
set -uo pipefail

dataset="${RC_VAR_DATASET:-}"
if [ -z "$dataset" ]; then
    echo '{"passed": false, "feedback": "check_error"}' >&2
    exit 2
fi
script=/usr/local/bin/nightly-backup.sh

fail() { printf '{"passed": false, "feedback": "%s"}\n' "$1"; exit 1; }

[ "$(id -u backup 2>/dev/null)" = "34" ] || fail "backup_user_changed"
[ -f "$script" ] || fail "script_missing"
grep -q "tar -czf \"/srv/backup/${dataset}-" "$script" || fail "script_changed"
if [ -n "$(find "$script" /srv/backup -maxdepth 0 -perm -o+w)" ]; then
    fail "too_open"
fi
if [ -n "$(find "$script" -maxdepth 0 -perm /6000)" ]; then
    fail "too_open"
fi

before=$(find /srv/backup -maxdepth 1 -name "${dataset}-*.tar.gz" 2>/dev/null | wc -l)
if ! runuser -u backup -- "$script" >/dev/null 2>&1; then
    fail "backup_fails"
fi
archive=$(find /srv/backup -maxdepth 1 -name "${dataset}-*.tar.gz" -newer "$script" -printf '%T@ %p\n' 2>/dev/null | sort -n | tail -1 | cut -d' ' -f2)
after=$(find /srv/backup -maxdepth 1 -name "${dataset}-*.tar.gz" | wc -l)
if [ -z "$archive" ] || [ "$after" -le "$before" ]; then
    fail "no_archive"
fi
[ "$(stat -c %U "$archive")" = "backup" ] || fail "no_archive"
tar -tzf "$archive" 2>/dev/null | grep -q "^${dataset}/${dataset}-1.csv$" || fail "archive_incomplete"

printf '{"passed": true, "feedback": ""}\n'
exit 0
