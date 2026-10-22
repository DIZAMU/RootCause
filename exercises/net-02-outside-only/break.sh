#!/usr/bin/env bash
# break.sh plants the fault: the site's listen directive is bound to the
# loopback address, so the site still works on the server itself but no other
# machine can reach it. nginx is restarted on the new config, the way the
# change would have been applied. Prints the config file for hint 3.
set -euo pipefail

: "${RC_VAR_PORT:?break.sh needs RC_VAR_PORT}"
: "${RC_VAR_SITE:?break.sh needs RC_VAR_SITE}"
conf="/etc/nginx/conf.d/${RC_VAR_SITE}.conf"

sed -i "s/^\([[:space:]]*listen\) ${RC_VAR_PORT};/\1 127.0.0.1:${RC_VAR_PORT};/" "$conf"
grep -q "listen 127.0.0.1:${RC_VAR_PORT};" "$conf"

pkill -x nginx || true
for _ in $(seq 50); do
    pgrep -x nginx >/dev/null || break
    sleep 0.1
done
if pgrep -x nginx >/dev/null; then
    echo '{"error": "nginx did not stop"}' >&2
    exit 2
fi
nginx

printf '{"conf": "%s"}\n' "$conf"
