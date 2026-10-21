#!/usr/bin/env bash
# setup.sh builds the HEALTHY state of the machine from the variant values and
# starts nginx. The engine copies it in at session start, runs it as root, then
# deletes it, the same way as break.sh. It is never part of the image, so the
# learner cannot read the correct config or re-run it as a shortcut.
set -euo pipefail

: "${RC_VAR_PORT:?setup.sh needs RC_VAR_PORT}"
: "${RC_VAR_SITE_NAME:?setup.sh needs RC_VAR_SITE_NAME}"

conf=/etc/nginx/conf.d/rootcause.conf
html=/usr/share/nginx/html/index.html

# Unquoted heredoc: ${RC_VAR_*} expand, \$uri stays literal for nginx.
cat > "$conf" <<CONF
# RootCause svc-01: the ${RC_VAR_SITE_NAME} intranet site
server {
    listen ${RC_VAR_PORT};
    server_name localhost;
    root /usr/share/nginx/html;
    index index.html;

    location / {
        try_files \$uri \$uri/ =404;
    }
}
CONF

# The original page carries a marker the check looks for, so a replacement or
# empty page cannot pass.
cat > "$html" <<HTML
<!doctype html>
<!-- rootcause:svc-01:original -->
<html lang="en">
<head>
  <meta charset="utf-8">
  <title>${RC_VAR_SITE_NAME} intranet</title>
</head>
<body>
  <h1>Welcome to the ${RC_VAR_SITE_NAME} intranet</h1>
  <p>Internal site. Authorized staff only.</p>
</body>
</html>
HTML

nginx   # daemonizes and returns; the healthy site is now served
