#!/usr/bin/env bash
# solution.sh is the reference fix, used only by CI. It repairs both possible
# faults: the script is executable again and the backup user owns its target.
set -euo pipefail

chmod 755 /usr/local/bin/nightly-backup.sh
chown backup:backup /srv/backup
chmod 750 /srv/backup
