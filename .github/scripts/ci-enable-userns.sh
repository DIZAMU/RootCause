#!/usr/bin/env bash
# ci-enable-userns.sh turns on Docker's user namespace remapping on a CI
# runner, the isolation the engine requires for sandboxes (ARCHITECTURE.md section
# 13.1). It merges "userns-remap": "default" into /etc/docker/daemon.json,
# restarts Docker, and waits until `docker info` reports name=userns.
#
# Restarting Docker with userns-remap switches its storage root, so start
# containers (Postgres, sandboxes) only after this script.
#
# Usage (as root): .github/scripts/ci-enable-userns.sh
set -euo pipefail

conf=/etc/docker/daemon.json
current='{}'
if [ -s "$conf" ]; then
    current="$(cat "$conf")"
fi
printf '%s\n' "$current" | jq '. + {"userns-remap": "default"}' > "$conf.new"
mv "$conf.new" "$conf"
echo "daemon.json:"
cat "$conf"

systemctl restart docker

# Poll for the concrete condition, with a time limit.
if ! timeout 60 bash -c 'until docker info --format "{{.SecurityOptions}}" 2>/dev/null | grep -q "name=userns"; do sleep 1; done'; then
    echo "ci-enable-userns: docker did not come back with userns-remap within 60s" >&2
    docker info >&2 || true
    exit 1
fi
docker info --format 'security options: {{.SecurityOptions}}, cgroup v{{.CgroupVersion}}'
