#!/usr/bin/env bash
# check.sh passes when the data volume has room again (at least 16 MiB free),
# the app can save a reading as its own user, and the five reference files are
# still there, unchanged. Anti-shortcut: emptying the volume deletes the
# reference files and fails. Exit 0 pass, 1 fail, 2 could-not-run. One line of
# JSON on stdout.
set -uo pipefail

fail() { printf '{"passed": false, "feedback": "%s"}\n' "$1"; exit 1; }

if ! mountpoint -q /srv/data; then
    echo '{"passed": false, "feedback": "check_error"}' >&2
    exit 2
fi

for i in 1 2 3 4 5; do
    want="$(printf 'meter,location\nM-%03d,building %d\n' "$i" "$i")"
    [ "$(cat "/srv/data/reference/meters-${i}.csv" 2>/dev/null)" = "$want" ] || fail "data_missing"
done

avail_kb="$(df --output=avail -k /srv/data | tail -1 | tr -d ' ')"
[ "$avail_kb" -ge 16384 ] || fail "still_full"

before="$(find /srv/data/readings -name 'reading-*.csv' | wc -l)"
runuser -u maleapp -- /usr/local/bin/save-reading >/dev/null 2>&1 || fail "app_cannot_save"
after="$(find /srv/data/readings -name 'reading-*.csv' | wc -l)"
[ "$after" -gt "$before" ] || fail "app_cannot_save"

printf '{"passed": true, "feedback": ""}\n'
exit 0
