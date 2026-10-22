#!/usr/bin/env bash
# break.sh plants the fault, chosen by seed: either the backup script loses
# its execute bit, or the target folder is handed to root so the backup user
# can no longer write there. It logs tonight's failed run the way the job
# would, and prints the blocked path for hint 3.
set -euo pipefail

: "${RC_VAR_DATASET:?break.sh needs RC_VAR_DATASET}"
script=/usr/local/bin/nightly-backup.sh
log=/var/log/nightly-backup.log
seed="${RC_SEED:-0}"

if (( seed % 2 == 0 )); then
    chmod 644 "$script"
    blocked="$script"
    msg="/bin/sh: 1: ${script}: Permission denied"
else
    chown root:root /srv/backup
    chmod 755 /srv/backup
    blocked=/srv/backup
    msg="tar (child): /srv/backup/${RC_VAR_DATASET}-$(date +%Y%m%d)-020001.tar.gz: Cannot open: Permission denied"
fi

printf '%s 02:00:01 %s\n' "$(date +%F)" "$msg" >> "$log"
printf '{"blocked": "%s"}\n' "$blocked"
