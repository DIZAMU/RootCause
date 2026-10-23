#!/usr/bin/env bash
# build-web.sh builds the dashboard (web/) into web/dist, from the committed
# package-lock.json (npm ci), after a type check. Then build the binary with
# the dashboard embedded:
#
#   go build -tags webdist -o rootcause ./cmd/rootcause
#
# Usage: scripts/build-web.sh
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
want="$(cat "$root/web/.nvmrc")"
have="$(node --version 2>/dev/null || true)"
case "$have" in
  "v$want".*) ;;
  *) echo "build-web.sh needs Node $want (web/.nvmrc), found: ${have:-none}" >&2; exit 2 ;;
esac

cd "$root/web"
npm ci --no-audit --no-fund
npm run build
