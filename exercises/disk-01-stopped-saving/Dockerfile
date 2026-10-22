# Healthy machine for disk-01: a Debian server where "maleapp" saves meter
# readings to a data volume at /srv/data (a small tmpfs the engine mounts).
# The save command is part of the machine, like any installed software. The
# data on the volume is written by setup.sh at session start.
#
# Pinned by digest, so a rebuild always gets the same base. To update:
# docker pull debian:13-slim, then
# docker inspect --format '{{index .RepoDigests 0}}' debian:13-slim.
FROM debian:13-slim@sha256:62bd423aeee912639ca8ee681c2b762cb3e25137e0a92b20be455898a9f034ad

# procps: ps. nano, vim-tiny: the learner's editors. df, du and find come
# with the base system.
RUN apt-get update \
 && apt-get install -y --no-install-recommends procps nano vim-tiny \
 && rm -rf /var/lib/apt/lists/* \
 && useradd --system --home-dir /srv/data --shell /usr/sbin/nologin maleapp \
 && mkdir -p /var/log/maleapp \
 && chown maleapp:maleapp /var/log/maleapp

COPY app/save-reading /usr/local/bin/save-reading
RUN chmod 755 /usr/local/bin/save-reading

ENTRYPOINT ["sleep", "infinity"]
