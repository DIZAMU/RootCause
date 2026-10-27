#!/usr/bin/env bash
# Proves the Phase 7 done-when criteria on the running cluster (spec section
# 18), after scripts/k8s-up.sh:
#
#   1. The API's service account can create, read, exec into and delete pods
#      in sandboxes, and nothing else anywhere (a kubectl auth can-i matrix).
#   2. A sandbox pod cannot reach the internet, cluster DNS, the Kubernetes
#      API, rootcause-system or another sandbox; a check runner reaches its
#      internal sandbox and nothing else. A control pod in a namespace without
#      policies reaches the same addresses, so the blocks come from the
#      NetworkPolicies, not from the probes.
#   3. A session runs fully in the cluster from the browser's side: the
#      dashboard's API and web terminal (TestClusterSession), with the network
#      probes repeated from the real session pod's terminal. It plays on its
#      own database, rootcause_cluster_test, through a second API pod, and the
#      real database must be unchanged afterwards (working rule 10).
#   4. Nothing of the cluster listens beyond the host's 127.0.0.1: every port
#      the k3d node containers publish (the dashboard and the Kubernetes API)
#      is bound to 127.0.0.1.
#
# The probe pods, the rootcause-probe namespace, the second API pod, its
# Secret and the test database are removed at the end.
# Exit 0 only if every check passes.
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
cd "$root"
context=k3d-rootcause
kc() { kubectl --context "$context" "$@"; }
failures=0
pass() { echo "PASS  $*"; }
fail() { echo "FAIL  $*"; failures=$((failures + 1)); }

echo "== 1. RBAC: system:serviceaccount:rootcause-system:rootcause-api"
sa=system:serviceaccount:rootcause-system:rootcause-api
# verb resource[/subresource] namespace(-: cluster scope) expected
matrix="
create pods sandboxes yes
get pods sandboxes yes
list pods sandboxes yes
delete pods sandboxes yes
create pods/exec sandboxes yes
get pods/exec sandboxes yes
get pods/log sandboxes yes
update pods sandboxes no
patch pods sandboxes no
watch pods sandboxes no
deletecollection pods sandboxes no
patch pods/ephemeralcontainers sandboxes no
create pods/attach sandboxes no
create pods/portforward sandboxes no
create pods/eviction sandboxes no
create services sandboxes no
create configmaps sandboxes no
get secrets sandboxes no
create networkpolicies.networking.k8s.io sandboxes no
delete networkpolicies.networking.k8s.io sandboxes no
update resourcequotas sandboxes no
delete limitranges sandboxes no
create deployments.apps sandboxes no
create jobs.batch sandboxes no
create rolebindings.rbac.authorization.k8s.io sandboxes no
create serviceaccounts/token sandboxes no
get pods rootcause-system no
create pods rootcause-system no
create pods/exec rootcause-system no
get secrets rootcause-system no
create jobs.batch rootcause-system no
patch deployments.apps rootcause-system no
delete networkpolicies.networking.k8s.io rootcause-system no
create serviceaccounts/token rootcause-system no
create pods default no
get pods kube-system no
get secrets kube-system no
get nodes - no
list namespaces - no
create namespaces - no
create clusterrolebindings.rbac.authorization.k8s.io - no
delete validatingadmissionpolicies.admissionregistration.k8s.io - no
patch validatingadmissionpolicybindings.admissionregistration.k8s.io - no
impersonate serviceaccounts - no
impersonate users - no
* * - no
"
while read -r verb resource ns want; do
	[[ -z "$verb" ]] && continue
	# kubectl reads "pods/exec" as plain "pods"; a subresource must be
	# passed with --subresource.
	args=("$verb" "${resource%%/*}")
	[[ "$resource" == */* ]] && args+=(--subresource "${resource#*/}")
	if [[ "$ns" == - ]]; then
		args+=(--all-namespaces)
		where="cluster-wide"
	else
		args+=(-n "$ns")
		where="in $ns"
	fi
	got="$(kc auth can-i "${args[@]}" --as="$sa" 2>/dev/null || true)"
	if [[ "$got" == "$want" ]]; then
		pass "can-i $verb $resource $where: $got"
	else
		fail "can-i $verb $resource $where: $got, want $want"
	fi
done <<<"$matrix"

echo "== 2. network"
cleanup() {
	kc delete pod -n sandboxes probe-target probe-sandbox probe-runner --ignore-not-found --grace-period=0 --wait=false >/dev/null 2>&1 || true
	kc delete namespace rootcause-probe --ignore-not-found --wait=false >/dev/null 2>&1 || true
}
trap cleanup EXIT
cleanup
kc wait --for=delete pod -n sandboxes probe-target probe-sandbox probe-runner --timeout=60s >/dev/null 2>&1 || true
kc wait --for=delete namespace rootcause-probe --timeout=120s >/dev/null 2>&1 || true

# probe_pod <namespace> <name> <labels as k: v lines> <command>: a locked-down
# pod from the log-01 image, which has bash, timeout and python3.
probe_pod() {
	kc apply -f - >/dev/null <<YAML
apiVersion: v1
kind: Pod
metadata:
  name: $2
  namespace: $1
  labels:
$3
spec:
  hostUsers: false
  automountServiceAccountToken: false
  enableServiceLinks: false
  terminationGracePeriodSeconds: 0
  securityContext:
    seccompProfile:
      type: RuntimeDefault
  containers:
    - name: probe
      image: rootcause/log-01-crash-on-start:v1
      imagePullPolicy: Never
      command: ["bash", "-c", "$4"]
      resources:
        limits: {cpu: 200m, memory: 64Mi}
        requests: {cpu: 200m, memory: 64Mi}
      securityContext:
        allowPrivilegeEscalation: false
        capabilities:
          drop: [ALL]
YAML
}
kc create namespace rootcause-probe >/dev/null
probe_pod sandboxes probe-target "    rootcause.sandbox: \"true\"
    rootcause.network: internal" "python3 -m http.server 8080"
probe_pod sandboxes probe-sandbox "    rootcause.sandbox: \"true\"
    rootcause.network: internal" "sleep infinity"
probe_pod sandboxes probe-runner "    rootcause.sandbox: \"true\"
    rootcause.network: internal
    rootcause.runner: \"true\"" "sleep infinity"
probe_pod rootcause-probe probe-control "    app: probe" "sleep infinity"
kc wait --for=condition=Ready pod -n sandboxes probe-target probe-sandbox probe-runner --timeout=120s >/dev/null
kc wait --for=condition=Ready pod -n rootcause-probe probe-control --timeout=120s >/dev/null

ip() { kc get "$@" -o jsonpath='{.status.podIP}{.spec.clusterIP}'; }
target_ip="$(ip pod -n sandboxes probe-target)"
api_pod_ip="$(kc get pod -n rootcause-system -l app=rootcause-api -o jsonpath='{.items[0].status.podIP}')"
pg_pod_ip="$(ip pod -n rootcause-system postgres-0)"
api_svc_ip="$(ip svc -n rootcause-system rootcause-api)"
pg_svc_ip="$(ip svc -n rootcause-system postgres)"
dns_ip="$(ip svc -n kube-system kube-dns)"
kube_ip="$(ip svc -n default kubernetes)"
# Every address a sandbox must not reach, as name=ip:port.
targets="internet=1.1.1.1:443 cluster-dns=$dns_ip:53 kube-api=$kube_ip:443 api-service=$api_svc_ip:7300 api-pod=$api_pod_ip:7300 postgres-service=$pg_svc_ip:5432 postgres-pod=$pg_pod_ip:5432"

# reach <namespace> <pod> <ip:port>: exit 0 if a TCP connection opens.
reach() {
	kc exec -n "$1" "$2" -- timeout 3 bash -c "exec 3<>/dev/tcp/${3%:*}/${3#*:}" >/dev/null 2>&1
}
# expect <namespace> <pod> <name=ip:port> <open|closed>
expect() {
	local name="${3%%=*}" addr="${3#*=}" got=closed
	reach "$1" "$2" "$addr" && got=open
	if [[ "$got" == "$4" ]]; then
		pass "$2 -> $name ($addr): $got"
	else
		fail "$2 -> $name ($addr): $got, want $4"
	fi
}
for t in $targets "other-sandbox=$target_ip:8080"; do
	expect sandboxes probe-sandbox "$t" closed
done
for t in $targets; do
	expect sandboxes probe-runner "$t" closed
done
expect sandboxes probe-runner "internal-sandbox=$target_ip:8080" open
# The control pod has no policy of its own, so it shows the probes work where
# nothing blocks them; rootcause-system and sandboxes still refuse it where
# their policies say so.
expect rootcause-probe probe-control "internet=1.1.1.1:443" open
expect rootcause-probe probe-control "cluster-dns=$dns_ip:53" open
expect rootcause-probe probe-control "api-service=$api_svc_ip:7300" open
expect rootcause-probe probe-control "postgres-service=$pg_svc_ip:5432" closed
expect rootcause-probe probe-control "sandbox=$target_ip:8080" closed

echo "== 3. a session from the browser's side"
# The session plays on its own database, never on the cluster's real one
# (rootcause): a second API pod, instance "prove", copied from the running
# API pod, serves rootcause_cluster_test on port 7301 through kubectl
# port-forward. It, its Secret and the database are removed at the end, and
# the real database's session count must not change.
testdb=rootcause_cluster_test
psql() { kc exec -n rootcause-system postgres-0 -- psql -U rootcause -d "$1" -tAc "$2"; }
forward_pid=""
secretdir="$(mktemp -d)"
cleanup_prove() {
	if [[ -n "$forward_pid" ]]; then kill "$forward_pid" 2>/dev/null || true; fi
	kc delete pod -n rootcause-system rootcause-api-prove --ignore-not-found --grace-period=0 --wait=true >/dev/null 2>&1 || true
	kc delete secret -n rootcause-system rootcause-db-prove --ignore-not-found >/dev/null 2>&1 || true
	kc exec -n rootcause-system postgres-0 -- dropdb -U rootcause --if-exists --force "$testdb" >/dev/null 2>&1 || true
	rm -rf "$secretdir"
}
trap 'cleanup_prove; cleanup' EXIT
cleanup_prove
secretdir="$(mktemp -d)"
real_before="$(psql rootcause "SELECT count(*) FROM session")"
kc exec -n rootcause-system postgres-0 -- createdb -U rootcause "$testdb"
(
	umask 077
	password="$(sed -n 's/^POSTGRES_PASSWORD=//p' "$root/deploy/k8s/.env")"
	[[ -n "$password" ]] || { echo "k8s-prove: no POSTGRES_PASSWORD in deploy/k8s/.env" >&2; exit 1; }
	printf 'postgres://rootcause:%s@postgres:5432/%s?sslmode=disable' "$password" "$testdb" >"$secretdir/database-url"
)
kc create secret generic rootcause-db-prove -n rootcause-system \
	--from-file=database-url="$secretdir/database-url" >/dev/null
rm -f "$secretdir/database-url"
# The running API pod with the test database, its own port and its own
# instance label, so the Service never sends the dashboard to it.
kc get pod -n rootcause-system -l app=rootcause-api,rootcause.instance=main -o json | python3 -c '
import json, sys
src = json.load(sys.stdin)["items"][0]["spec"]
c = src["containers"][0]
c["args"] = ["serve", "--listen-all", "--port", "7301"]
c["ports"] = [{"containerPort": 7301}]
c["readinessProbe"] = {"tcpSocket": {"port": 7301}, "periodSeconds": 2}
for e in c["env"]:
    if e["name"] == "RC_DATABASE_URL":
        e["valueFrom"]["secretKeyRef"]["name"] = "rootcause-db-prove"
c["volumeMounts"] = [m for m in c["volumeMounts"] if not m["name"].startswith("kube-api-access")]
spec = {k: v for k, v in src.items() if k not in ("nodeName", "tolerations", "priority", "preemptionPolicy")}
spec["volumes"] = [v for v in src["volumes"] if not v["name"].startswith("kube-api-access")]
json.dump({"apiVersion": "v1", "kind": "Pod",
           "metadata": {"name": "rootcause-api-prove", "namespace": "rootcause-system",
                        "labels": {"app": "rootcause-api", "rootcause.instance": "prove"}},
           "spec": spec}, sys.stdout)
' | kc apply -f - >/dev/null
kc wait --for=condition=Ready pod -n rootcause-system rootcause-api-prove --timeout=120s >/dev/null
kubectl --context "$context" port-forward -n rootcause-system pod/rootcause-api-prove 7301:7301 >/dev/null 2>&1 &
forward_pid=$!
for _ in $(seq 50); do
	curl -fs -o /dev/null http://localhost:7301/ && break
	sleep 0.2
done
probes="$(tr ' ' ',' <<<"$targets"),other-sandbox=$target_ip:8080"
log="$(mktemp)"
rc=0
RC_CLUSTER_URL=http://localhost:7301 RC_CLUSTER_DATABASE="$testdb" RC_CLUSTER_PROBES="$probes" \
	go test -count=1 -v -run '^TestClusterSession$' ./internal/server/ >"$log" 2>&1 || rc=$?
grep -E '^\s+cluster_test|^(--- |ok|FAIL)' "$log" || true
if [[ "$rc" -eq 0 ]] && grep -q -- '--- PASS: TestClusterSession' "$log"; then
	pass "TestClusterSession on $testdb"
else
	fail "TestClusterSession (exit $rc)"
fi
rm -f "$log"
played="$(psql "$testdb" "SELECT count(*) FROM session")"
real_after="$(psql rootcause "SELECT count(*) FROM session")"
if [[ "$played" -ge 1 && "$real_before" == "$real_after" ]]; then
	pass "the session went to $testdb ($played session); rootcause unchanged ($real_after sessions)"
else
	fail "sessions: $testdb has $played, rootcause went from $real_before to $real_after"
fi

echo "== 4. published ports"
# docker ps prints each published port as "IP:hostport->port/proto".
published="$(docker ps --filter "label=k3d.cluster=rootcause" --format '{{.Names}} {{.Ports}}')"
# The address may be IPv6 in brackets ([::]:port); "]" goes first in the set.
bindings="$(grep -oE '[][0-9a-f.:]+:[0-9]+->[0-9]+/[a-z]+' <<<"$published" || true)"
if [[ -z "$bindings" ]]; then
	fail "no published ports found for the k3d cluster"
fi
while read -r b; do
	[[ -n "$b" ]] || continue
	if [[ "$b" == 127.0.0.1:* ]]; then
		pass "published on the host's loopback only: $b"
	else
		fail "published beyond 127.0.0.1: $b (deploy/k8s/k3d.yaml binds 127.0.0.1 for a new cluster; recreate it)"
	fi
done <<<"$bindings"

echo
if [[ "$failures" -gt 0 ]]; then
	echo "k8s-prove: $failures check(s) failed"
	exit 1
fi
echo "k8s-prove: every check passed"
