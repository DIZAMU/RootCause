#!/usr/bin/env bash
# break.sh plants the fault on the whole share, the way a careless recursive
# command would: by seed, either group write is removed (chmod -R g-w) or the
# group owner is changed to "users" (chgrp -R users). Either way the members
# can no longer save. It prints the share's resulting group and mode for
# hint 3.
set -euo pipefail

: "${RC_VAR_DEPT:?break.sh needs RC_VAR_DEPT}"
share="/srv/shares/${RC_VAR_DEPT}"
seed="${RC_SEED:-0}"

if (( seed % 2 == 0 )); then
    chmod -R g-w "$share"
else
    chgrp -R users "$share"
fi

printf '{"share": "%s", "group_owner": "%s", "mode": "%s"}\n' \
    "$share" "$(stat -c %G "$share")" "$(stat -c %A "$share")"
