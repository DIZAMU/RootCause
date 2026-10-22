#!/usr/bin/env bash
# solution.sh is the reference fix, used only by CI: listen on every address
# again, test the config and reload nginx.
set -euo pipefail

conf="/etc/nginx/conf.d/${RC_VAR_SITE:?}.conf"
sed -i "s/listen 127\.0\.0\.1:${RC_VAR_PORT:?};/listen ${RC_VAR_PORT};/" "$conf"
nginx -t
pkill -x nginx
for _ in $(seq 50); do
    pgrep -x nginx >/dev/null || break
    sleep 0.1
done
nginx
