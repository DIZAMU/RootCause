# Healthy machine for svc-01: a Debian-based nginx, ready to serve an internal
# site. Debian paths (/etc/nginx/conf.d, /usr/share/nginx/html) match what a
# learner meets on a real Ubuntu/Debian server.
#
# The image holds no site config and no RootCause scripts. setup.sh renders the
# healthy site at session start and is deleted afterwards, so the correct
# config can never be read or restored from inside the sandbox.
FROM nginx:1.27

# curl: the check fetches the site. procps: break and solution use pkill.
# nano, vim-tiny: the learner needs an editor to fix a config file.
RUN apt-get update \
 && apt-get install -y --no-install-recommends curl procps nano vim-tiny \
 && rm -rf /var/lib/apt/lists/* \
 && rm -f /etc/nginx/conf.d/default.conf

# PID 1 only keeps the container alive. The engine runs it with --init, so a
# real init reaps stopped processes. nginx is started by setup.sh.
ENTRYPOINT ["sleep", "infinity"]
