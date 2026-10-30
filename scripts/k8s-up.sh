#!/usr/bin/env bash
# Brings up the v2 cluster (ARCHITECTURE.md section 8.2) and deploys RootCause into it.
# Safe to re-run: every step creates what is missing and updates the rest.
#
# Default (GitOps, ARCHITECTURE.md section 14): Argo CD syncs deploy/k8s from main on
# GitHub, so the cluster runs the release pinned there, never the working
# tree.
#   1. The k3d cluster from deploy/k8s/k3d.yaml (started if it exists).
#   2. The exercise images, built with docker and imported into the nodes
#      (sandbox pods never pull).
#   3. The namespaces, and the Secrets that are never in git: rootcause-db
#      (a password generated once into git-ignored deploy/k8s/.env),
#      ghcr-pull (the owner's GHCR token) and Argo CD's repository
#      credential (the owner's read-only deploy key). No secret appears in a
#      manifest, on a command line or in this script's output.
#   4. Argo CD core, from its pinned and checksum-verified install manifest,
#      then deploy/argocd (the project and the Application).
#   5. The NetworkPolicy that lets the API reach the Kubernetes API server's
#      address, and a wait until Argo CD reports Synced and Healthy.
#
# --dev: the API image is built from the working tree (rootcause/api:local)
# and applied with deploy/k8s-dev. Argo CD's automated sync, if Argo CD is
# installed, is turned off first, or it would put the release back. A later
# run without --dev turns it on again.
#
# The owner's credentials (paths can be changed with these variables):
#   RC_GHCR_TOKEN_FILE     ~/.config/rootcause/ghcr-token
#                          a classic token with read:packages only
#   RC_ARGOCD_DEPLOY_KEY   ~/.config/rootcause/argocd-deploy-key
#                          the private half of a read-only deploy key
#
# Needs docker, k3d, kubectl (scripts/install-k8s-tools.sh), Go and Node.
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
cd "$root"
cluster=rootcause
context="k3d-$cluster"
envfile="$root/deploy/k8s/.env"
ghcr_token_file="${RC_GHCR_TOKEN_FILE:-$HOME/.config/rootcause/ghcr-token}"
deploy_key_file="${RC_ARGOCD_DEPLOY_KEY:-$HOME/.config/rootcause/argocd-deploy-key}"
ghcr_user=DIZAMU
repo_url=git@github.com:DIZAMU/RootCause.git

# Argo CD core (ARCHITECTURE.md section 8.2). To update: change the version, download
# manifests/core-install.yaml for it, put its sha256sum here, and pin the
# images it names by digest in deploy/argocd/install/kustomization.yaml.
ARGOCD_VERSION=v3.5.6
ARGOCD_SHA256=d3d9d3555ad77eb4b6d27b1ec0d2af7978a91537e84d54631f983c227f0a3d31

mode=gitops
case "${1:-}" in
"") ;;
--dev) mode=dev ;;
*) echo "usage: k8s-up.sh [--dev]" >&2; exit 2 ;;
esac

for tool in docker k3d kubectl go; do
	command -v "$tool" >/dev/null || { echo "k8s-up: $tool not found on PATH" >&2; exit 1; }
done

# need_secret_file <path> <what>: the file must exist and be private.
need_secret_file() {
	if [[ ! -s "$1" ]]; then
		echo "k8s-up: $2 not found at $1 (see README, Kubernetes)" >&2
		exit 1
	fi
	if [[ -n "$(find "$1" -perm /077)" ]]; then
		echo "k8s-up: $1 is readable by others; run chmod 600 $1" >&2
		exit 1
	fi
}

if [[ "$mode" == gitops ]]; then
	if grep -q 'digest: sha256:0\{64\}$' deploy/k8s/kustomization.yaml; then
		echo "k8s-up: deploy/k8s/kustomization.yaml pins no release yet (zero digest)." >&2
		echo "k8s-up: push a release tag and merge its digest first, or run k8s-up.sh --dev" >&2
		exit 1
	fi
	need_secret_file "$ghcr_token_file" "the GHCR read:packages token"
	need_secret_file "$deploy_key_file" "the Argo CD deploy key"
fi

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

# app_exists: Argo CD is installed and has the rootcause Application.
app_exists() {
	kc get crd applications.argoproj.io >/dev/null 2>&1 &&
		kc get application rootcause -n argocd >/dev/null 2>&1
}

# sync_state prints the Application's sync, health and policy.
sync_state() {
	if ! app_exists; then
		echo "argo cd: no rootcause Application (not managed by Argo CD)"
		return
	fi
	kc get application rootcause -n argocd -o jsonpath='argo cd: sync {.status.sync.status} at {.status.sync.revision}, health {.status.health.status}, automated {.spec.syncPolicy.automated}{"\n"}'
}

echo "== images"
images=()
if [[ "$mode" == dev ]]; then
	scripts/build-api-image.sh rootcause/api:local
	images+=(rootcause/api:local)
fi
for dir in exercises/*/; do
	id="$(basename "$dir")"
	version="$(awk '$1 == "version:" { print $2; exit }' "$dir/exercise.yaml")"
	tag="rootcause/$id:v$version"
	docker build -q -t "$tag" "$dir" >/dev/null
	images+=("$tag")
done
k3d image import --cluster "$cluster" "${images[@]}" >/dev/null
echo "imported ${#images[@]} images"

echo "== namespaces and secrets"
kc apply -f deploy/k8s/namespaces.yaml >/dev/null
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
kc create secret generic rootcause-db -n rootcause-system \
	--from-file=password="$tmp/password" --from-file=database-url="$tmp/database-url" \
	--dry-run=client -o yaml | kc apply -f - >/dev/null
rm -f "$tmp/password" "$tmp/database-url"
echo "secret rootcause-db applied"

if [[ "$mode" == gitops ]]; then
	# The pull secret is built in a private file, so the token never reaches
	# a command line.
	(
		umask 077
		auth="$(printf '%s:%s' "$ghcr_user" "$(tr -d '[:space:]' <"$ghcr_token_file")" | base64 -w0)"
		printf '{"auths":{"ghcr.io":{"auth":"%s"}}}' "$auth" >"$tmp/dockerconfig.json"
	)
	kc create secret generic ghcr-pull -n rootcause-system \
		--type=kubernetes.io/dockerconfigjson --from-file=.dockerconfigjson="$tmp/dockerconfig.json" \
		--dry-run=client -o yaml | kc apply -f - >/dev/null
	rm -f "$tmp/dockerconfig.json"
	echo "secret ghcr-pull applied"

	echo "== argo cd $ARGOCD_VERSION"
	mkdir "$tmp/argocd"
	curl -fsSL --retry 3 -o "$tmp/argocd/core-install.yaml" \
		"https://raw.githubusercontent.com/argoproj/argo-cd/$ARGOCD_VERSION/manifests/core-install.yaml"
	if ! echo "$ARGOCD_SHA256  $tmp/argocd/core-install.yaml" | sha256sum --check --status; then
		echo "k8s-up: checksum mismatch for Argo CD $ARGOCD_VERSION core-install.yaml" >&2
		exit 1
	fi
	cp deploy/argocd/install/kustomization.yaml "$tmp/argocd/"
	kc create namespace argocd --dry-run=client -o yaml | kc apply -f - >/dev/null
	# Server-side: the CRDs are too large for the last-applied annotation.
	kc apply --server-side --force-conflicts -k "$tmp/argocd" >/dev/null
	kc rollout status statefulset/argocd-application-controller -n argocd --timeout=600s
	kc rollout status deployment/argocd-repo-server -n argocd --timeout=600s
	kc rollout status deployment/argocd-redis -n argocd --timeout=600s

	kc create secret generic repo-rootcause -n argocd \
		--from-literal=type=git --from-literal=url="$repo_url" \
		--from-file=sshPrivateKey="$deploy_key_file" \
		--dry-run=client -o yaml |
		kc label --local -f - argocd.argoproj.io/secret-type=repository -o yaml |
		kc apply -f - >/dev/null
	echo "secret repo-rootcause applied"
fi

echo "== api-to-apiserver"
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

if [[ "$mode" == dev ]]; then
	echo "== manifests (dev)"
	if app_exists; then
		kc patch application rootcause -n argocd --type json \
			-p '[{"op": "remove", "path": "/spec/syncPolicy/automated"}]' >/dev/null 2>&1 || true
		echo "argo cd automated sync turned off"
	fi
	kc apply -k deploy/k8s-dev
	kc rollout status statefulset/postgres -n rootcause-system --timeout=180s
	kc rollout restart deployment/rootcause-api -n rootcause-system >/dev/null
	kc rollout status deployment/rootcause-api -n rootcause-system --timeout=180s
else
	echo "== argo cd application"
	kc apply -f deploy/argocd/project.yaml -f deploy/argocd/application.yaml
	# Ask for a refresh now instead of at the next poll.
	kc annotate application rootcause -n argocd argocd.argoproj.io/refresh=normal --overwrite >/dev/null
	state=""
	for _ in $(seq 120); do
		state="$(kc get application rootcause -n argocd -o jsonpath='{.status.sync.status}/{.status.health.status}')"
		[[ "$state" == Synced/Healthy ]] && break
		sleep 5
	done
	if [[ "$state" != Synced/Healthy ]]; then
		sync_state
		kc get application rootcause -n argocd -o jsonpath='{range .status.conditions[*]}{.type}: {.message}{"\n"}{end}' >&2
		echo "k8s-up: Argo CD did not reach Synced/Healthy within 10 minutes" >&2
		exit 1
	fi
fi

echo "== state"
sync_state
echo "Dashboard: http://localhost:7300"
