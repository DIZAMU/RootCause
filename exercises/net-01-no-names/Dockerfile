# Healthy machine for net-01: a Debian server that resolves the office's
# internal names through a local DNS server (dnsmasq). The zone and the
# resolver settings are written by setup.sh at session start, so the image
# holds no exercise state.
#
# Pinned by digest, so a rebuild always gets the same base. To update:
# docker pull debian:13-slim, then
# docker inspect --format '{{index .RepoDigests 0}}' debian:13-slim.
FROM debian:13-slim@sha256:62bd423aeee912639ca8ee681c2b762cb3e25137e0a92b20be455898a9f034ad

# dnsmasq-base: the local DNS server, without a service unit. bind9-dnsutils:
# dig and nslookup. iproute2: ss. procps: ps, pkill. nano, vim-tiny: editors.
RUN apt-get update \
 && apt-get install -y --no-install-recommends dnsmasq-base bind9-dnsutils iproute2 procps nano vim-tiny \
 && rm -rf /var/lib/apt/lists/*

ENTRYPOINT ["sleep", "infinity"]
