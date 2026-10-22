#!/usr/bin/env bash
# break.sh plants the fault, chosen by seed: the config's data_dir points at a
# path that does not exist, or the port setting is missing. It then restarts
# the app the way a config change would be rolled out. The app crashes on
# start and writes the reason to its log. Prints the broken setting for
# hint 3.
set -euo pipefail

conf=/etc/lagerapp/lagerapp.ini
seed="${RC_SEED:-0}"

/etc/init.d/lagerapp stop >/dev/null
if pgrep -f /opt/lagerapp/app.py >/dev/null; then
    echo '{"error": "lagerapp did not stop"}' >&2
    exit 2
fi

if (( seed % 2 == 0 )); then
    sed -i 's|^data_dir = /var/lib/lagerapp$|data_dir = /var/lib/lager-app|' "$conf"
    setting=data_dir section=storage
else
    sed -i '/^port = /d' "$conf"
    setting=port section=server
fi

/etc/init.d/lagerapp start >/dev/null
# start-stop-daemon --background returns at once. Wait until the crash is
# logged, so the learner finds the error in the log from the start.
for _ in $(seq 50); do
    grep -q 'startup failed' /var/log/lagerapp/lagerapp.log && break
    sleep 0.1
done
grep -q 'startup failed' /var/log/lagerapp/lagerapp.log

printf '{"setting": "%s", "section": "%s"}\n' "$setting" "$section"
