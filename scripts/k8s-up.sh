#!/usr/bin/env bash
# Brings up the v2 cluster (spec section 8.2) and deploys RootCause into it.
# Safe to re-run: every step creates what is missing and updates the rest.
#
#   1. The k3d cluster from deploy/k8s/k3d.yaml (started if it exists).
#   2. Images: the API image (rootcause built with the dashboard, plus the
#      exercises) and every exercise image, built with docker and imported
#      into the cluster's nodes. Nothing is pushed anywhere.
#   3. The rootcause-db Secret. Its password is generated once and kept in
#      deploy/k8s/.env, which git ignores (spec section 13.2). It never
#      appears in a manifest, on a command line or in this script's output.
#   4. The manifests (kubectl apply -k deploy/k8s), and the NetworkPolicy
#      that lets the API reach the Kubernetes API server's address.
#   5. A restart of the API, so it runs the image just built.
#
# Needs docker, k3d, kubectl (scripts/install-k8s-tools.sh), Go and Node.
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
cd "$root"
cluster=rootcause
context="k3d-$cluster"
envfile="$root/deploy/k8s/.env"

for tool in docker k3d kubectl go; do
	command -v "$tool" >/dev/null || { echo "k8s-up: $tool not found on PATH" >&2; exit 1; }
done

echo "== cluster"
if k3d cluster get "$cluster" >/dev/null 2>&1; then
	k3d cluster start "$cluster" --wait >/dev/null
else
	k3d cluster create --config deploy/k8s/k3d.yaml
fi
kubectl config use-context "$context" >/dev/null
kc() { kubectl --context "$context" "$@"; }

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

echo "== images"
scripts/build-web.sh >/dev/null
mkdir "$tmp/api"
CGO_ENABLED=0 go build -tags webdist -o "$tmp/api/rootcause" ./cmd/rootcause
cp -r exercises "$tmp/api/exercises"
docker build -q -t rootcause/api:local -f deploy/k8s/api/Dockerfile "$tmp/api" >/dev/null
images=(rootcause/api:local)
for dir in exercises/*/; do
	id="$(basename "$dir")"
	version="$(awk '$1 == "version:" { print $2; exit }' "$dir/exercise.yaml")"
	tag="rootcause/$id:v$version"
	docker build -q -t "$tag" "$dir" >/dev/null
	images+=("$tag")
done
k3d image import --cluster "$cluster" "${images[@]}" >/dev/null
echo "imported ${#images[@]} images"

echo "== secret"
if [[ ! -f "$envfile" ]]; then
	(
		umask 077
		printf 'POSTGRES_PASSWORD=%s\n' "$(head -c 32 /dev/urandom | base64 | tr -dc 'A-Za-z0-9')" >"$envfile"
	)
	echo "generated a password in deploy/k8s/.env"
fi
password="$(sed -n 's/^POSTGRES_PASSWORD=//p' "$envfile")"
[[ -n "$password" ]] || { echo "k8s-up: no POSTGRES_PASSWORD in $envfile" >&2; exit 1; }
(
	umask 077
	printf '%s' "$password" >"$tmp/password"
	printf 'postgres://rootcause:%s@postgres:5432/rootcause?sslmode=disable' "$password" >"$tmp/database-url"
)
unset password
kc apply -f deploy/k8s/namespaces.yaml >/dev/null
kc create secret generic rootcause-db -n rootcause-system \
	--from-file=password="$tmp/password" --from-file=database-url="$tmp/database-url" \
	--dry-run=client -o yaml | kc apply -f - >/dev/null
rm -f "$tmp/password" "$tmp/database-url"
echo "secret rootcause-db applied"

echo "== manifests"
kc apply -k deploy/k8s
apiserver="$(kc get endpointslice -n default -l kubernetes.io/service-name=kubernetes \
	-o jsonpath='{.items[0].endpoints[0].addresses[0]} {.items[0].ports[0].port}')"
read -r apiserver_ip apiserver_port <<<"$apiserver"
[[ -n "$apiserver_ip" && -n "$apiserver_port" ]] || { echo "k8s-up: cannot find the API server's address" >&2; exit 1; }
kc apply -f - <<YAML
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: api-to-apiserver
  namespace: rootcause-system
spec:
  podSelector:
    matchLabels:
      app: rootcause-api
  policyTypes: [Egress]
  egress:
    - to:
        - ipBlock:
            cidr: $apiserver_ip/32
      ports:
        - port: $apiserver_port
          protocol: TCP
YAML

echo "== rollout"
kc rollout status statefulset/postgres -n rootcause-system --timeout=180s
kc rollout restart deployment/rootcause-api -n rootcause-system >/dev/null
kc rollout status deployment/rootcause-api -n rootcause-system --timeout=180s
echo "Dashboard: http://localhost:7300"
