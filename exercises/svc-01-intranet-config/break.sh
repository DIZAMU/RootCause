#!/usr/bin/env bash
# break.sh plants the fault: a syntax error in the nginx config, which takes the
# site down. It is deterministic in RC_SEED (same seed -> same broken line) and
# leaves no trace of itself. It prints one line of JSON with the broken line,
# which fills the {line} placeholder in hint 3.
set -uo pipefail

conf=/etc/nginx/conf.d/rootcause.conf
seed="${RC_SEED:-0}"

# Candidate lines: directives where dropping the trailing semicolon is a real
# syntax error. We avoid directives like server_name that accept a variable
# number of arguments, where a missing semicolon just swallows the next line
# and stays valid.
mapfile -t lines < <(grep -nE '^[[:space:]]*(listen|root|index|try_files)[[:space:]].*;[[:space:]]*$' "$conf" | cut -d: -f1)
if [ "${#lines[@]}" -eq 0 ]; then
    echo '{"error": "no candidate lines"}' >&2
    exit 2
fi

# Deterministic choice from the seed.
idx=$(( seed % ${#lines[@]} ))
target=${lines[$idx]}

# The fault: drop the trailing semicolon on that line. This is the classic
# nginx syntax error ("directive is not terminated by ;").
sed -i "${target}s/;[[:space:]]*$//" "$conf"

# Take the site down the way a bad config change would: stop nginx, then try to
# start it again. The start fails on the broken config, so nothing serves.
pkill -x nginx 2>/dev/null || true
sleep 0.3
nginx 2>/dev/null || true

# Report the line that is actually broken. nginx reports a later line (where it
# noticed the problem); the walkthrough explains that difference.
printf '{"line": %s}\n' "$target"
