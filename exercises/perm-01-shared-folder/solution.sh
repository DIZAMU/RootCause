#!/usr/bin/env bash
# solution.sh is the reference fix, used only by CI. It repairs both possible
# faults: the department group owns the share again, the group can write, and
# the directory keeps its setgid bit so new files get the group.
set -euo pipefail

share="/srv/shares/${RC_VAR_DEPT:?}"
chgrp -R "$RC_VAR_DEPT" "$share"
chmod -R g+w "$share"
chmod g+s "$share"
