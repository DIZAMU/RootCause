#!/usr/bin/env bash
# solution.sh is the reference fix, used only by CI. It repairs both possible
# faults: nsswitch.conf asks DNS again, and dnsmasq listens where resolv.conf
# points, after a restart.
set -euo pipefail

sed -i 's/^hosts:.*/hosts:          files dns/' /etc/nsswitch.conf
sed -i 's/^listen-address=.*/listen-address=127.0.0.1/' /etc/dnsmasq.d/office.conf
dnsmasq --test
pkill -x dnsmasq || true
for _ in $(seq 50); do
    pgrep -x dnsmasq >/dev/null || break
    sleep 0.1
done
dnsmasq
