#!/usr/bin/env bash
# break.sh plants the fault, chosen by seed: either nsswitch.conf no longer
# asks DNS for host names ("hosts: files"), or dnsmasq is moved to listen on
# 127.0.0.2 while resolv.conf still points at 127.0.0.1. Either way internal
# names stop resolving. Prints the file that holds the fault for hint 3.
set -euo pipefail

seed="${RC_SEED:-0}"

if (( seed % 2 == 0 )); then
    sed -i 's/^hosts:.*/hosts:          files/' /etc/nsswitch.conf
    file=/etc/nsswitch.conf
else
    file=/etc/dnsmasq.d/office.conf
    sed -i 's/^listen-address=127\.0\.0\.1$/listen-address=127.0.0.2/' "$file"
    grep -q '^listen-address=127.0.0.2$' "$file"
    pkill -x dnsmasq || true
    for _ in $(seq 50); do
        pgrep -x dnsmasq >/dev/null || break
        sleep 0.1
    done
    if pgrep -x dnsmasq >/dev/null; then
        echo '{"error": "dnsmasq did not stop"}' >&2
        exit 2
    fi
    dnsmasq
fi

printf '{"fault_file": "%s"}\n' "$file"
