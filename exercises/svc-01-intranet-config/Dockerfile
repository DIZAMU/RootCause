# Healthy machine for svc-01: a Debian-based nginx, ready to serve an internal
# site. Debian paths (/etc/nginx/conf.d, /usr/share/nginx/html) match what a
# learner meets on a real Ubuntu/Debian server.
#
# The image holds no site config and no RootCause scripts. setup.sh renders the
# healthy site at session start and is deleted afterwards, so the correct
# config can never be read or restored from inside the sandbox.
#
# Pinned to nginx's stable line (1.30.x, Debian 13) by digest, so a rebuild
# always gets the same base. To update: docker pull nginx:<tag>, then
# docker inspect --format '{{index .RepoDigests 0}}' nginx:<tag>.
FROM nginx:1.30.4@sha256:d5792f71a9496b833bc08ea834a758c46e2b6a6306c10f4be926f38a656cdc1c

# The base image links nginx's logs to the container's output. A real server
# keeps them as files in /var/log/nginx, so the learner can read them there.
#
# curl: the check fetches the site. procps: break and solution use pkill.
# nano, vim-tiny: the learner needs an editor to fix a config file.
RUN apt-get update \
 && apt-get install -y --no-install-recommends curl procps nano vim-tiny \
 && rm -rf /var/lib/apt/lists/* \
 && rm -f /etc/nginx/conf.d/default.conf \
 && rm -f /var/log/nginx/access.log /var/log/nginx/error.log \
 && touch /var/log/nginx/access.log /var/log/nginx/error.log

# PID 1 only keeps the container alive. The engine runs it with --init, so a
# real init reaps stopped processes. nginx is started by setup.sh.
ENTRYPOINT ["sleep", "infinity"]
