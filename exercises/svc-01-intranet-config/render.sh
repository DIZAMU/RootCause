#!/usr/bin/env bash
# render.sh builds the HEALTHY state of the machine from the variant values.
#
# It is baked into the image and run at container start (by entrypoint.sh) and
# again by solution.sh as the reference fix. Keeping the correct config in one
# place means the healthy machine and the reference solution can never drift
# apart. It never plants a fault.
set -euo pipefail

: "${RC_VAR_PORT:?render.sh needs RC_VAR_PORT}"
: "${RC_VAR_SITE_NAME:?render.sh needs RC_VAR_SITE_NAME}"

conf=/etc/nginx/conf.d/rootcause.conf
html=/usr/share/nginx/html/index.html

# The nginx image ships a default site on port 80. Remove it so the only site
# is ours, on the variant's port. (A learner who "fixes" the fault by falling
# back to the default config will then be serving nothing on the required port.)
rm -f /etc/nginx/conf.d/default.conf

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
