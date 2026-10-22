# Healthy machine for net-02: a Debian-based nginx serving an internal web app
# to the rest of the network. The site config is rendered by setup.sh at
# session start, so the image holds no exercise state.
#
# Pinned to nginx's stable line (1.30.x, Debian 13) by digest, like svc-01.
FROM nginx:1.30.4@sha256:d5792f71a9496b833bc08ea834a758c46e2b6a6306c10f4be926f38a656cdc1c

# The base image links nginx's logs to the container's output. A real server
# keeps them as files in /var/log/nginx, so the learner can read them there.
#
# curl: test the site. iproute2: ss shows listening sockets. procps: ps,
# pkill. nano, vim-tiny: the learner's editors.
RUN apt-get update \
 && apt-get install -y --no-install-recommends curl iproute2 procps nano vim-tiny \
 && rm -rf /var/lib/apt/lists/* \
 && rm -f /etc/nginx/conf.d/default.conf \
 && rm -f /var/log/nginx/access.log /var/log/nginx/error.log \
 && touch /var/log/nginx/access.log /var/log/nginx/error.log

ENTRYPOINT ["sleep", "infinity"]
