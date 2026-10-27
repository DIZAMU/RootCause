#!/usr/bin/env bash
# Fails if a script passes a credential to a command line, where any process
# on the machine can read it (ps, /proc/<pid>/cmdline). It looks for curl-style
# user and header options whose value is built from a command substitution or
# from a variable named like a token or password:
#
#   -u "user:$(cat token-file)"      --user "x:$GITHUB_TOKEN"
#   -H "Authorization: Bearer $1"    --header "Authorization: token $t"
#
# Pass such values on stdin instead (curl --config -).
#
# Usage: scripts/ci-check-credential-args.sh [file...]   (default: the other scripts/*.sh)
#        scripts/ci-check-credential-args.sh --self-test
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
cd "$root"

pattern='(^|[[:space:]])(-u|--user|-H|--header)[[:space:]]+"([^"]*\$\(|[^"]*\$\{?[A-Za-z_]*([Tt][Oo][Kk][Ee][Nn]|[Pp][Aa][Ss][Ss][Ww][Oo][Rr][Dd])|[Aa]uthorization:)'

if [[ "${1:-}" == --self-test ]]; then
	# The line that once put the owner's token on curl's command line, and
	# the forms the pattern must also catch, then two it must leave alone.
	# They are literal text, never expanded.
	# shellcheck disable=SC2016
	bad=(
		'curl -fsS -u "DIZAMU:$(tr -d "[:space:]" < ~/.config/rootcause/ghcr-token)" https://ghcr.io/token'
		'curl -sS -u "${GITHUB_ACTOR:-x}:$GITHUB_TOKEN" https://ghcr.io/token'
		'curl -fsS -H "Authorization: Bearer $1" https://ghcr.io/v2/'
		'curl --header "Authorization: token $t" https://api.github.com/'
	)
	# shellcheck disable=SC2016
	good=(
		'curl -fsSL --retry 3 -o "$tmp/$name" "$url"'
		'curl -sS -I -H "Accept: $accept" "https://$registry/v2/"'
	)
	for line in "${bad[@]}"; do
		grep -qE -- "$pattern" <<<"$line" || { echo "self-test: not caught: $line" >&2; exit 1; }
	done
	for line in "${good[@]}"; do
		if grep -qE -- "$pattern" <<<"$line"; then echo "self-test: false alarm: $line" >&2; exit 1; fi
	done
	echo "self-test: ${#bad[@]} caught, ${#good[@]} left alone"
	exit 0
fi

files=("$@")
if [[ ${#files[@]} -eq 0 ]]; then
	# Every script but this one, whose comments and self-test hold the
	# patterns on purpose.
	for f in scripts/*.sh; do
		[[ "$f" == scripts/ci-check-credential-args.sh ]] || files+=("$f")
	done
fi
if hits="$(grep -nHE -- "$pattern" "${files[@]}")"; then
	echo "credential on a command line (pass it on stdin, e.g. curl --config -):" >&2
	echo "$hits" >&2
	exit 1
fi
echo "no credential arguments in ${#files[@]} files"
