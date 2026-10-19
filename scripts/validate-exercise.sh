#!/usr/bin/env bash
# validate-exercise.sh proves one exercise works end to end for one seed:
#
#   1. The definition is valid (Go loader).
#   2. Unsolved:  break -> check must FAIL  (the fault is real and detected).
#   3. Solved:    break -> solution -> check must PASS (the fix is accepted).
#
# It does this in a real sandbox container, with the spec's resource limits and
# no network. break/check/solution are copied in, run as root, then deleted,
# exactly as the engine will do.
#
# Usage: scripts/validate-exercise.sh <exercise-id> <seed>
set -euo pipefail

id="${1:?usage: validate-exercise.sh <exercise-id> <seed>}"
seed="${2:?usage: validate-exercise.sh <exercise-id> <seed>}"

root="$(cd "$(dirname "$0")/.." && pwd)"
exdir="$root/exercises/$id"
[ -d "$exdir" ] || { echo "no such exercise: exercises/$id" >&2; exit 2; }

image="rootcause-ex-${id}:test"
container="rc-validate-${id}-${seed}-$$"

cleanup() { docker rm -f "$container" >/dev/null 2>&1 || true; }
trap cleanup EXIT

note() { printf '\n== %s\n' "$*"; }

# --- 1. validate the definition, and get this seed's variant values ----------
note "validating definition"
go run "$root/cmd/rootcause" validate "$exdir"

note "resolving variant for seed $seed"
varlines="$(go run "$root/cmd/rootcause" vars --seed "$seed" "$exdir")"
echo "$varlines"

# Turn the RC_VAR_NAME='value' lines into docker -e NAME=value arguments.
env_args=( -e "RC_SEED=$seed" )
while IFS= read -r line; do
    [ -z "$line" ] && continue
    name="${line%%=*}"
    val="${line#*=}"; val="${val#\'}"; val="${val%\'}"   # strip the single quotes
    env_args+=( -e "$name=$val" )
done <<< "$varlines"

# --- helpers -----------------------------------------------------------------
start_container() {
    docker rm -f "$container" >/dev/null 2>&1 || true
    docker run -d --rm \
        --network none --memory 256m --cpus 0.5 \
        "${env_args[@]}" --name "$container" "$image" >/dev/null
    # give the entrypoint a moment to render config and start nginx
    sleep 1
}

# run_script <script> : copy it in, run as root, capture stdout+exit, delete it.
run_script() {
    local script="$1"
    docker cp "$exdir/$script" "$container:/tmp/$script"
    docker exec "$container" bash -c \
        "chmod +x /tmp/$script && /tmp/$script; rc=\$?; rm -f /tmp/$script; exit \$rc"
}

# --- 2. build the image ------------------------------------------------------
note "building image"
docker build -t "$image" "$exdir" >/dev/null
echo "built $image"

# --- 3. unsolved: break then check must FAIL ---------------------------------
note "scenario A (unsolved): break, then check must FAIL"
start_container
echo "break:    $(run_script break.sh)"
set +e
out="$(run_script check.sh)"; rc=$?
set -e
echo "check:    $out (exit $rc)"
if [ "$rc" -eq 0 ]; then
    echo "FAIL: check passed on the unsolved machine; the fault is not detected." >&2
    exit 1
fi
echo "ok: check correctly failed while broken"

# --- 4. solved: break, solution, then check must PASS ------------------------
note "scenario B (solved): break, solution, then check must PASS"
start_container
echo "break:    $(run_script break.sh)"
run_script solution.sh >/dev/null 2>&1
set +e
out="$(run_script check.sh)"; rc=$?
set -e
echo "check:    $out (exit $rc)"
if [ "$rc" -ne 0 ]; then
    echo "FAIL: check did not pass after the reference solution." >&2
    exit 1
fi
echo "ok: check correctly passed after the solution"

note "PASS: $id seed $seed"
