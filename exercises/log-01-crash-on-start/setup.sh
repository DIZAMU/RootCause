#!/usr/bin/env bash
# setup.sh builds the healthy state: lagerapp's config with the variant's
# port, its stock files in /var/lib/lagerapp, and the app started by its init
# script. It waits until /health answers, so the healthy state is proven. It
# runs as root at session start and is deleted afterwards.
set -euo pipefail

: "${RC_VAR_PORT:?setup.sh needs RC_VAR_PORT}"

mkdir -p /etc/lagerapp /var/lib/lagerapp /var/log/lagerapp
cat > /etc/lagerapp/lagerapp.ini <<INI
# lagerapp configuration
[server]
port = ${RC_VAR_PORT}

[storage]
data_dir = /var/lib/lagerapp
INI
for item in skruer bolter muttere; do
    printf 'item,count\n%s,100\n' "$item" > "/var/lib/lagerapp/${item}.csv"
done
chown -R lagerapp:lagerapp /var/lib/lagerapp /var/log/lagerapp

/etc/init.d/lagerapp start >/dev/null
for _ in $(seq 50); do
    curl -sf --max-time 1 "http://127.0.0.1:${RC_VAR_PORT}/health" >/dev/null && exit 0
    sleep 0.1
done
echo "lagerapp did not become healthy" >&2
exit 1
