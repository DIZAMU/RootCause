#!/usr/bin/env bash
# setup.sh builds the healthy state: nginx serves the site on every address
# at the variant's port, so other machines on the network can reach it. It
# runs as root at session start and is deleted afterwards.
set -euo pipefail

: "${RC_VAR_PORT:?setup.sh needs RC_VAR_PORT}"
: "${RC_VAR_SITE:?setup.sh needs RC_VAR_SITE}"

conf="/etc/nginx/conf.d/${RC_VAR_SITE}.conf"
html=/usr/share/nginx/html/index.html

cat > "$conf" <<CONF
# ${RC_VAR_SITE}: internal web app for the office network
server {
    listen ${RC_VAR_PORT};
    server_name ${RC_VAR_SITE};
    root /usr/share/nginx/html;
    index index.html;

    location / {
        try_files \$uri \$uri/ =404;
    }
}
CONF

cat > "$html" <<HTML
<!doctype html>
<!-- rootcause:net-02:original -->
<html lang="en">
<head><meta charset="utf-8"><title>${RC_VAR_SITE}</title></head>
<body><h1>${RC_VAR_SITE}</h1><p>Internal web app.</p></body>
</html>
HTML

nginx
