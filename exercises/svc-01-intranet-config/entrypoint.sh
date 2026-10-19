#!/usr/bin/env bash
# entrypoint.sh is the container's PID 1. It renders the healthy site, starts
# nginx in the background, then sleeps so the container stays alive and nginx
# can be stopped and started during the session (no systemd involved).
set -euo pipefail

/opt/rootcause/render.sh
nginx                 # daemonizes and returns; master+workers run in background
exec sleep infinity   # keep the container running
