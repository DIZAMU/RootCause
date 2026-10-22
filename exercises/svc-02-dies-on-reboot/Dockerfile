# Healthy machine for svc-02: a Debian server that boots systemd as PID 1, so
# "start at boot" means what it means on a real server. The vaktliste app and
# its unit file are installed like a package would install them. Whether the
# unit is enabled, and its config, are set by setup.sh at session start.
#
# The engine runs this image with init: systemd: no docker init, and tmpfs on
# /run and /run/lock. journald and tmp.mount are masked because they need
# capabilities the sandbox does not grant; every other unit must boot cleanly.
#
# Pinned by digest, so a rebuild always gets the same base. To update:
# docker pull debian:13-slim, then
# docker inspect --format '{{index .RepoDigests 0}}' debian:13-slim.
FROM debian:13-slim@sha256:62bd423aeee912639ca8ee681c2b762cb3e25137e0a92b20be455898a9f034ad

# systemd: the service manager. python3: runs the app. curl: query it.
# procps: ps. nano, vim-tiny: the learner's editors.
RUN apt-get update \
 && apt-get install -y --no-install-recommends systemd python3 curl procps nano vim-tiny \
 && rm -rf /var/lib/apt/lists/* \
 && systemctl mask systemd-journald.service systemd-journald.socket \
      systemd-journald-dev-log.socket systemd-journal-flush.service tmp.mount \
 && useradd --system --home-dir /nonexistent --shell /usr/sbin/nologin vaktliste

COPY app/app.py /opt/vaktliste/app.py
COPY app/vaktliste.service /usr/lib/systemd/system/vaktliste.service

# systemd shuts down cleanly on SIGRTMIN+3, not on docker's default SIGTERM.
STOPSIGNAL SIGRTMIN+3
ENTRYPOINT ["/usr/lib/systemd/systemd"]
