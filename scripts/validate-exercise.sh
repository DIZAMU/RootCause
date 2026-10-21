#!/usr/bin/env bash
# validate-exercise.sh proves one exercise works end to end for one seed:
#
#   1. The definition is valid (Go loader).
#   2. Unsolved:  setup -> break -> check must FAIL (exit 1), and no exercise
#                 script or RC_ value is left visible in the sandbox.
#   3. Solved:    setup -> break -> solution -> check must PASS (exit 0).
#
# Steps 2 and 3 run in the engine's own code (rootcause selftest), so they use
# exactly the sandbox flags and script handling a real session uses.
#
# Usage: scripts/validate-exercise.sh <exercise-id> <seed>
set -euo pipefail

id="${1:?usage: validate-exercise.sh <exercise-id> <seed>}"
seed="${2:?usage: validate-exercise.sh <exercise-id> <seed>}"

root="$(cd "$(dirname "$0")/.." && pwd)"
exdir="$root/exercises/$id"
[ -d "$exdir" ] || { echo "no such exercise: exercises/$id" >&2; exit 2; }

echo "== validating definition"
go run "$root/cmd/rootcause" validate "$exdir"

go run "$root/cmd/rootcause" selftest --seed "$seed" "$exdir"
