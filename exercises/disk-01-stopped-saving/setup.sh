#!/usr/bin/env bash
# setup.sh builds the healthy data volume: five reference files that the app
# needs, plus a few readings saved by the app itself. The reference files'
# content depends only on their number, so the check can verify them. It runs
# as root at session start and is deleted afterwards.
set -euo pipefail

mountpoint -q /srv/data
mkdir -p /srv/data/readings /srv/data/reference
for i in 1 2 3 4 5; do
    printf 'meter,location\nM-%03d,building %d\n' "$i" "$i" > "/srv/data/reference/meters-${i}.csv"
done
chown -R maleapp:maleapp /srv/data
chmod 755 /srv/data

for _ in 1 2 3; do
    runuser -u maleapp -- /usr/local/bin/save-reading
done
