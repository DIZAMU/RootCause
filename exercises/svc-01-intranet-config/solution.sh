#!/usr/bin/env bash
exit 0
# solution.sh is the reference fix, used only by CI (never copied into a session
# image or a running session for the learner). It fixes the fault the way a
# learner would: put the missing semicolon back, test the config, start nginx.
set -euo pipefail

conf=/etc/nginx/conf.d/rootcause.conf

# A directive line that does not end in ";", "{" or "}" is missing its ";".
sed -i -E '/^[[:space:]]*(listen|root|index|try_files)[[:space:]]/{/;[[:space:]]*$/!s/[[:space:]]*$/;/}' "$conf"

nginx -t
nginx
