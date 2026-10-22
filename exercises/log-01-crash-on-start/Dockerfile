# Healthy machine for log-01: a Debian server running "lagerapp", a small
# Python web service, under a classic init script. The app and its init script
# are part of the machine, like any installed software. Its config and data are
# written by setup.sh at session start, so the image holds no exercise state.
#
# Pinned by digest, so a rebuild always gets the same base. To update:
# docker pull debian:13-slim, then
# docker inspect --format '{{index .RepoDigests 0}}' debian:13-slim.
FROM debian:13-slim@sha256:62bd423aeee912639ca8ee681c2b762cb3e25137e0a92b20be455898a9f034ad

# python3: runs the app. curl: query its health endpoint. procps: ps, pgrep.
# nano, vim-tiny: the learner's editors.
RUN apt-get update \
 && apt-get install -y --no-install-recommends python3 curl procps nano vim-tiny \
 && rm -rf /var/lib/apt/lists/* \
 && useradd --system --home-dir /var/lib/lagerapp --shell /usr/sbin/nologin lagerapp

COPY app/app.py /opt/lagerapp/app.py
COPY app/lagerapp.init /etc/init.d/lagerapp
RUN chmod 755 /opt/lagerapp/app.py /etc/init.d/lagerapp

ENTRYPOINT ["sleep", "infinity"]
