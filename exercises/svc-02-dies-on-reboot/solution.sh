#!/usr/bin/env bash
# solution.sh is the reference fix, used only by CI: enable the unit, so
# systemd starts it at every boot.
set -euo pipefail

systemctl enable vaktliste.service
