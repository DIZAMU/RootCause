#!/usr/bin/env bash
# break.sh plants the fault: the unit is disabled, so it no longer starts at
# boot, but it keeps running for now. Exactly what happens when someone
# starts a service by hand and never enables it. Prints the unit name for
# hint 3.
set -euo pipefail

systemctl disable vaktliste.service 2>/dev/null
systemctl is-active --quiet vaktliste.service
if systemctl is-enabled --quiet vaktliste.service; then
    echo '{"error": "vaktliste is still enabled"}' >&2
    exit 2
fi

printf '{"unit": "vaktliste.service"}\n'
