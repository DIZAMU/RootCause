#!/usr/bin/env bash
# solution.sh is the reference fix, used only by CI: find the large hidden
# file the way a learner would and delete it.
set -euo pipefail

find /srv/data -xdev -type f -size +10M -path '*/.*' -delete
