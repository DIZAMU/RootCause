# The API and dashboard image for the cluster (spec section 8.2). The context
# is a directory scripts/k8s-up.sh assembles: the rootcause binary, built with
# -tags webdist so the dashboard is embedded, and the exercises folder, whose
# scripts the engine copies into sandboxes. Nothing else: no shell tools are
# needed, and the process runs as an unprivileged user.
FROM debian:13-slim@sha256:62bd423aeee912639ca8ee681c2b762cb3e25137e0a92b20be455898a9f034ad
COPY rootcause /opt/rootcause/rootcause
COPY exercises /opt/rootcause/exercises
USER 10001:10001
ENV RC_ROOT=/opt/rootcause HOME=/tmp
ENTRYPOINT ["/opt/rootcause/rootcause"]
CMD ["serve", "--listen-all"]
