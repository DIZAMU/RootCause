#!/usr/bin/env bash
# Prints the memory the k3d cluster uses (ARCHITECTURE.md section 8.2): each node
# container's usage from docker, then the working set of every container in
# the cluster, read from each node's container runtime (the cluster runs no
# metrics-server). Ends with the WSL2 total from free -m.
#
# Usage: scripts/k8s-memory.sh [namespace]   (only that namespace's pods)
set -euo pipefail

cluster=rootcause
only="${1:-}"

nodes="$(docker ps --format '{{.Names}}' --filter "label=k3d.cluster=$cluster" | grep -E -- '-(server|agent)-[0-9]+$' | sort)"
[[ -n "$nodes" ]] || { echo "k8s-memory: cluster $cluster is not running" >&2; exit 1; }

echo "== nodes (docker)"
# shellcheck disable=SC2086 # one argument per node
docker stats --no-stream --format '{{.Name}} {{.MemUsage}}' $nodes

echo "== containers (working set)"
for n in $nodes; do
	docker exec "$n" crictl stats -o json
done | python3 -c '
import json, sys
only = sys.argv[1]
dec = json.JSONDecoder()
text = sys.stdin.read()
rows, total, i = [], 0, 0
while i < len(text):
    if text[i].isspace():
        i += 1
        continue
    doc, i = dec.raw_decode(text, i)
    for s in doc.get("stats", []):
        labels = s["attributes"]["labels"]
        ns = labels.get("io.kubernetes.pod.namespace", "")
        if only and ns != only:
            continue
        mib = int((s.get("memory") or {}).get("workingSetBytes", {}).get("value", 0)) / 2**20
        total += mib
        rows.append((ns, labels.get("io.kubernetes.pod.name", ""), s["attributes"]["metadata"]["name"], mib))
for ns, pod, c, mib in sorted(rows):
    print(f"{mib:7.0f} MiB  {ns}/{pod}/{c}")
print(f"{total:7.0f} MiB  total")
' "$only"

echo "== WSL2 (free -m)"
free -m
