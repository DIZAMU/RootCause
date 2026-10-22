#!/usr/bin/env bash
# solution.sh is the reference fix, used only by CI. It repairs both possible
# faults, the way the log describes them, and starts the app again.
set -euo pipefail

conf=/etc/lagerapp/lagerapp.ini
sed -i 's|^data_dir = .*|data_dir = /var/lib/lagerapp|' "$conf"
grep -q '^port = ' "$conf" || sed -i "/^\[server\]$/a port = ${RC_VAR_PORT:?}" "$conf"

/etc/init.d/lagerapp start
for _ in $(seq 50); do
    curl -sf --max-time 1 "http://127.0.0.1:${RC_VAR_PORT}/health" >/dev/null && exit 0
    sleep 0.1
done
exit 1
