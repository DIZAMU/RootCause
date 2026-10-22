#!/usr/bin/env bash
# setup.sh builds the healthy resolver chain: dnsmasq serves the internal zone
# on 127.0.0.1, resolv.conf points at it, and nsswitch.conf asks files, then
# DNS. It runs as root at session start and is deleted afterwards.
#
# resolv.conf is a Docker bind mount: it is rewritten in place (">"), because
# replacing the file fails with "Device or resource busy".
set -euo pipefail

: "${RC_VAR_ZONE:?setup.sh needs RC_VAR_ZONE}"
zone="$RC_VAR_ZONE"

mkdir -p /etc/dnsmasq.d
cat > /etc/dnsmasq.conf <<CONF
# Local DNS server for the office network. Site settings live in
# /etc/dnsmasq.d/.
conf-dir=/etc/dnsmasq.d/,*.conf
CONF

cat > /etc/dnsmasq.d/office.conf <<CONF
# Internal zone ${zone}
listen-address=127.0.0.1
bind-interfaces
no-resolv
domain=${zone}
host-record=fil01.${zone},10.20.0.11
host-record=print01.${zone},10.20.0.12
host-record=intranett.${zone},10.20.0.20
CONF

printf 'nameserver 127.0.0.1\nsearch %s\n' "$zone" > /etc/resolv.conf
sed -i 's/^hosts:.*/hosts:          files dns/' /etc/nsswitch.conf

dnsmasq --test
dnsmasq
