#!/usr/bin/env bash
# check.sh grades the END STATE, never the commands typed. It passes only when
# the nginx config is valid AND the original site is served on the required
# port. Exit 0 pass, 1 fail, 2 could-not-run. It prints one line of JSON whose
# feedback key maps to a symptom text in i18n (never the fix).
set -uo pipefail

port="${RC_VAR_PORT:-}"
marker="rootcause:svc-01:original"

fail() { printf '{"passed": false, "feedback": "%s"}\n' "$1"; exit 1; }

if [ -z "$port" ]; then
    echo '{"passed": false, "feedback": "check_error"}' >&2
    exit 2
fi

# 1. The config must pass its own test. An invalid or empty config fails here.
if ! nginx -t >/dev/null 2>&1; then
    fail "config_invalid"
fi

# 2. The site must actually respond on the required port. A default config
#    listens on :80, not the variant's port, so this is the anti-shortcut:
#    falling back to the default config leaves the required port unreachable.
body="$(curl -s --max-time 3 "http://localhost:${port}/" 2>/dev/null)"
if [ -z "$body" ]; then
    fail "site_unreachable"
fi

# 3. It must be the ORIGINAL page, not a replacement or placeholder. The marker
#    lives in the original index.html, which break.sh never touches.
if ! printf '%s' "$body" | grep -q "$marker"; then
    fail "wrong_content"
fi

printf '{"passed": true, "feedback": ""}\n'
exit 0
