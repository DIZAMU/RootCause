#!/usr/bin/env bash
# Installs the pinned k3d and kubectl binaries into ~/.local/bin (no sudo).
# Each download is checked against the SHA-256 pinned here before it is
# installed. Re-running is safe: a binary that already matches is kept.
#
# Usage: scripts/install-k8s-tools.sh [install-dir]
set -euo pipefail

K3D_VERSION=v5.9.0
K3D_SHA256=06d8f25bc3a971c4eb29e0ff08429b180402db0f4dec838c9eac427e296800a0
KUBECTL_VERSION=v1.37.1
KUBECTL_SHA256=65691ff77eb6fa44c908b77a1082c9f092c3b9733b5cefabec0d1104890e21a8

dest="${1:-$HOME/.local/bin}"
mkdir -p "$dest"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

# install_tool <name> <url> <sha256>
install_tool() {
	local name="$1" url="$2" sum="$3"
	if [[ -x "$dest/$name" ]] && echo "$sum  $dest/$name" | sha256sum --check --status; then
		echo "$name: already installed ($dest/$name)"
		return
	fi
	curl -fsSL --retry 3 -o "$tmp/$name" "$url"
	if ! echo "$sum  $tmp/$name" | sha256sum --check --status; then
		echo "$name: checksum mismatch for $url" >&2
		exit 1
	fi
	install -m 0755 "$tmp/$name" "$dest/$name"
	echo "$name: installed $dest/$name"
}

install_tool k3d "https://github.com/k3d-io/k3d/releases/download/$K3D_VERSION/k3d-linux-amd64" "$K3D_SHA256"
install_tool kubectl "https://dl.k8s.io/release/$KUBECTL_VERSION/bin/linux/amd64/kubectl" "$KUBECTL_SHA256"

case ":$PATH:" in
*":$dest:"*) ;;
*) echo "note: $dest is not on PATH" ;;
esac
