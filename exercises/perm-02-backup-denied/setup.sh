#!/usr/bin/env bash
# setup.sh builds the healthy backup: data in /srv/<dataset>, the script
# /usr/local/bin/nightly-backup.sh, the target /srv/backup owned by the backup
# user, and a log with earlier good runs. It then runs the backup once as the
# backup user to prove the healthy state works. It runs as root at session
# start and is deleted afterwards.
set -euo pipefail

: "${RC_VAR_DATASET:?setup.sh needs RC_VAR_DATASET}"
data="/srv/${RC_VAR_DATASET}"
script=/usr/local/bin/nightly-backup.sh
log=/var/log/nightly-backup.log

mkdir -p "$data"
for i in 1 2 3; do
    printf '%s record %d\n' "$RC_VAR_DATASET" "$i" > "$data/${RC_VAR_DATASET}-${i}.csv"
done
chmod 755 "$data"
chmod 644 "$data"/*.csv

cat > "$script" <<SCRIPT
#!/bin/sh
# Nightly backup of ${data}. Runs as the backup user.
set -e
stamp=\$(date +%Y%m%d-%H%M%S)
tar -czf "/srv/backup/${RC_VAR_DATASET}-\$stamp.tar.gz" -C /srv "${RC_VAR_DATASET}"
echo "\$(date '+%F %T') backup ok: ${RC_VAR_DATASET}-\$stamp.tar.gz" >> ${log}
SCRIPT
chown root:root "$script"
chmod 755 "$script"

mkdir -p /srv/backup
chown backup:backup /srv/backup
chmod 750 /srv/backup

touch "$log"
chown backup:backup "$log"
chmod 640 "$log"
for d in 3 2 1; do
    printf '%s 02:00:01 backup ok: %s-%s-020001.tar.gz\n' \
        "$(date -d "-${d} day" +%F)" "$RC_VAR_DATASET" "$(date -d "-${d} day" +%Y%m%d)" >> "$log"
done

runuser -u backup -- "$script"
rm -f /srv/backup/*.tar.gz
