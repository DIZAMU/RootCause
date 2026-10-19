#!/usr/bin/env bash
# solution.sh is the reference fix, used only by CI (never copied into a session
# image or a running session for the learner). It restores the correct config by
# re-rendering the healthy state, then starts nginx.
set -euo pipefail

/opt/rootcause/render.sh        # rewrite the correct config and page
pkill -x nginx 2>/dev/null || true
sleep 0.3
nginx                            # start with the now-valid config
