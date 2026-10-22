#!/usr/bin/env bash
# break.sh plants the fault: a large file in a hidden folder on the data
# volume fills it completely, so the app can no longer save. The folder comes
# from the variant and the file name from the seed. One failed save is logged
# the way the app would. Prints the file and its size for hint 3.
set -euo pipefail

: "${RC_VAR_JUNK_DIR:?break.sh needs RC_VAR_JUNK_DIR}"
seed="${RC_SEED:-0}"
dir="/srv/data/${RC_VAR_JUNK_DIR}"
junk="${dir}/export-$(( 1000 + seed % 9000 )).tmp"

mkdir -p "$dir"
# dd stops with "No space left on device", which is the point.
dd if=/dev/zero of="$junk" bs=1M 2>/dev/null || true
chown -R maleapp:maleapp "/srv/data/${RC_VAR_JUNK_DIR%%/*}"

if runuser -u maleapp -- /usr/local/bin/save-reading; then
    echo '{"error": "volume is not full"}' >&2
    exit 2
fi

printf '{"junk": "%s", "junk_mb": %d}\n' "$junk" "$(( $(stat -c %s "$junk") / 1048576 ))"
