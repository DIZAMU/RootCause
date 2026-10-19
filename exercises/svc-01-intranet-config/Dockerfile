# Healthy machine for svc-01: a Debian-based nginx serving an internal site.
# Debian paths (/etc/nginx/conf.d, /usr/share/nginx/html) match what a learner
# meets on a real Ubuntu/Debian server.
FROM nginx:1.27

# curl: the check fetches the site. procps: break/solution stop nginx with pkill.
RUN apt-get update \
 && apt-get install -y --no-install-recommends curl procps \
 && rm -rf /var/lib/apt/lists/*

COPY render.sh entrypoint.sh /opt/rootcause/
RUN chmod +x /opt/rootcause/render.sh /opt/rootcause/entrypoint.sh \
 && rm -f /etc/nginx/conf.d/default.conf

# The port is a per-session variant, so nothing is EXPOSEd at build time.
ENTRYPOINT ["/opt/rootcause/entrypoint.sh"]
