#!/usr/bin/env bash
# Builds the API image (deploy/k8s/api/Dockerfile) under the tag given: the
# rootcause binary with the dashboard embedded (-tags webdist), plus the
# exercises folder. scripts/k8s-up.sh (local images) and release.yml (GHCR)
# both build it here, so the cluster runs what the release pushes.
#
# Usage: scripts/build-api-image.sh <tag> [version]
#   version goes into the image's org.opencontainers.image.version label
#   (default "dev").
set -euo pipefail

[[ $# -ge 1 && $# -le 2 ]] || { echo "usage: build-api-image.sh <tag> [version]" >&2; exit 2; }
tag="$1"
version="${2:-dev}"

root="$(cd "$(dirname "$0")/.." && pwd)"
cd "$root"

ctx="$(mktemp -d)"
trap 'rm -rf "$ctx"' EXIT

scripts/build-web.sh >/dev/null
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -tags webdist -o "$ctx/rootcause" ./cmd/rootcause
cp -r exercises "$ctx/exercises"
docker build -q --platform linux/amd64 \
	--label "org.opencontainers.image.version=$version" \
	--label "org.opencontainers.image.revision=$(git rev-parse HEAD)" \
	-t "$tag" -f deploy/k8s/api/Dockerfile "$ctx" >/dev/null
echo "built $tag"
