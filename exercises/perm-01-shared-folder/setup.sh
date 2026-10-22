#!/usr/bin/env bash
# setup.sh builds the healthy share: a department group with two members, an
# outsider, and /srv/shares/<dept> owned by the group, group-writable and
# setgid so new files keep the group. It runs as root at session start and is
# deleted afterwards.
set -euo pipefail

: "${RC_VAR_DEPT:?setup.sh needs RC_VAR_DEPT}"
: "${RC_VAR_MEMBER:?setup.sh needs RC_VAR_MEMBER}"
: "${RC_VAR_COLLEAGUE:?setup.sh needs RC_VAR_COLLEAGUE}"

share="/srv/shares/${RC_VAR_DEPT}"

groupadd "$RC_VAR_DEPT"
useradd -m -s /bin/bash -G "$RC_VAR_DEPT" "$RC_VAR_MEMBER"
useradd -m -s /bin/bash -G "$RC_VAR_DEPT" "$RC_VAR_COLLEAGUE"
useradd -m -s /bin/bash per

mkdir -p "$share"
printf 'Budsjett 2027, utkast\n' > "$share/budsjett.txt"
printf 'Rapport Q3\n' > "$share/rapport.txt"
chown -R "root:${RC_VAR_DEPT}" "$share"
chmod 2770 "$share"
chmod 660 "$share"/*.txt
