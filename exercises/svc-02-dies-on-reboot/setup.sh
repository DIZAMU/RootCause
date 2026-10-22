#!/usr/bin/env bash
# setup.sh builds the healthy state: vaktliste's config with the variant's
# port, and the unit enabled and started, so it survives a reboot. It waits
# until /health answers. It runs as root at session start and is deleted
# afterwards.
set -euo pipefail

: "${RC_VAR_PORT:?setup.sh needs RC_VAR_PORT}"

mkdir -p /etc/vaktliste
printf 'PORT=%s\n' "$RC_VAR_PORT" > /etc/vaktliste/vaktliste.env
systemctl enable --now vaktliste.service

for _ in $(seq 50); do
    curl -sf --max-time 1 "http://127.0.0.1:${RC_VAR_PORT}/health" >/dev/null && exit 0
    sleep 0.1
done
echo "vaktliste did not become healthy" >&2
exit 1
