#!/usr/bin/env bash
# Checks the dashboard in a real browser (spec section 11): accessibility
# (axe-core, WCAG 2.2 AA rules), layout shift on data arrival, keyboard rules
# (shortcuts never reach the terminal or a field, the palette is a modal),
# the Tab order, terminal typing latency, the CSP header, and screenshots.
# A developer tool for this laptop, not CI: it runs Windows' Chrome headless
# with a throwaway profile, driven over the DevTools protocol on Windows'
# 127.0.0.1 by Windows' Node. scripts/ui/*.mjs use Node's built-ins only.
#
# Usage: scripts/ui-check.sh <base-url> <active-session-id> <finished-session-id> [out-dir]
#
# The server at base-url must use a test database: the keyboard check runs
# a real check on the active session. For the terminal checks it runs with
# RC_RUNTIME=kubernetes. Exit 0 only if every check passes.
set -euo pipefail

[[ $# -ge 3 && $# -le 4 ]] || { echo "usage: ui-check.sh <base-url> <active-session-id> <finished-session-id> [out-dir]" >&2; exit 2; }
base="${1%/}"
active="$2"
finished="$3"
root="$(cd "$(dirname "$0")/.." && pwd)"
out="${4:-$(mktemp -d)}"
mkdir -p "$out"

# axe-core, pinned: never in web/package.json, the bundle or the repo.
AXE_VERSION=4.14.0
AXE_SHA256=20c09fe157a8a34a30e241aaa1fcdade657734f08ab379ecfbeb7d45cc46e878
node="${RC_UI_NODE:-/mnt/c/Program Files/nodejs/node.exe}"
chrome="${RC_UI_CHROME:-/mnt/c/Program Files/Google/Chrome/Application/chrome.exe}"
port="${RC_UI_CDP_PORT:-9333}"

for tool in "$node" "$chrome"; do
	[[ -x "$tool" ]] || { echo "ui-check: not found: $tool (set RC_UI_NODE / RC_UI_CHROME)" >&2; exit 1; }
done
[[ -n "${WSL_DISTRO_NAME:-}" ]] || { echo "ui-check: runs in WSL, with Windows' Node and Chrome" >&2; exit 1; }
# winpath turns a WSL path into the UNC path Windows' Node can read.
winpath() { printf '%s\n' "\\\\wsl.localhost\\${WSL_DISTRO_NAME}${1//\//\\}"; }
win() { (cd /tmp && "$node" "$@") | tr -d '\r'; }

axe="$HOME/.cache/rootcause/axe-core-$AXE_VERSION.min.js"
if ! echo "$AXE_SHA256  $axe" | sha256sum --check --status 2>/dev/null; then
	mkdir -p "$(dirname "$axe")"
	tmp="$(mktemp -d)"
	curl -fsSL --retry 3 -o "$tmp/axe.tgz" "https://registry.npmjs.org/axe-core/-/axe-core-$AXE_VERSION.tgz"
	tar -xzf "$tmp/axe.tgz" -C "$tmp" package/axe.min.js
	echo "$AXE_SHA256  $tmp/package/axe.min.js" | sha256sum --check --status || { echo "ui-check: axe-core checksum mismatch" >&2; exit 1; }
	mv "$tmp/package/axe.min.js" "$axe"
	rm -rf "$tmp"
fi
cp "$axe" "$out/axe.min.js"

# A throwaway headless Chrome; closed again on exit.
profile="$(cd /tmp && cmd.exe /c 'echo %TEMP%' 2>/dev/null | tr -d '\r')\\rootcause-ui-check"
"$chrome" --headless=new --remote-debugging-port="$port" --remote-debugging-address=127.0.0.1 \
	--user-data-dir="$profile" --no-first-run --no-default-browser-check --disable-extensions --hide-scrollbars about:blank >/dev/null 2>&1 &
cdp() { win -e "fetch('http://127.0.0.1:$port/json/version').then(r => r.json()).then(j => { $1 }).catch(() => process.exit(1))"; }
close_chrome() {
	cdp "const ws = new WebSocket(j.webSocketDebuggerUrl); ws.onopen = () => { ws.send(JSON.stringify({ id: 1, method: 'Browser.close' })); setTimeout(() => process.exit(0), 300); };" >/dev/null 2>&1 || true
}
trap close_chrome EXIT
for i in $(seq 40); do
	cdp "console.log(j.Browser)" >/dev/null 2>&1 && break
	[[ "$i" -lt 40 ]] || { echo "ui-check: Chrome did not start" >&2; exit 1; }
	sleep 0.5
done

# setlang switches the learner's language through the API, with the token
# from the served page on curl's stdin, never on its command line.
token="$(curl -fsS "$base/" | sed -n 's/.*name="rootcause-token" content="\([0-9a-f]*\)".*/\1/p')"
[[ -n "$token" ]] || { echo "ui-check: no token in $base/ (is the dashboard built in?)" >&2; exit 1; }
setlang() {
	curl -fsS -o /dev/null --config - -X PUT -H "Content-Type: application/json" --data "{\"lang\":\"$1\"}" "$base/api/lang" <<<"header = \"X-RootCause-Token: $token\""
}

failures=0
report() { # report <log>: show it, count its FAIL lines
	cat "$1"
	failures=$((failures + $(grep -c '^FAIL' "$1" || true)))
}

echo "== CSP"
csp="$(curl -fsSI "$base/" | sed -n 's/^[Cc]ontent-[Ss]ecurity-[Pp]olicy: //p' | tr -d '\r')"
# The constant is one string split over two lines: its line and the next.
want="$(awk '/^const ContentSecurityPolicy = / { getline next_line; print $0 next_line; exit }' "$root/internal/server/server.go" |
	sed 's/^const ContentSecurityPolicy = "//; s/" +[[:space:]]*"//; s/"$//')"
if [[ "$csp" == "$want" ]]; then echo "PASS  CSP is the server's constant: $csp"; else echo "FAIL  CSP '$csp', the constant says '$want'"; failures=$((failures + 1)); fi

pages="inbox=#/ exercises=#/exercises history=#/history stats=#/stats session=#/session/$active finished=#/session/$finished"
plan() { # plan <kind> <lang> <scheme>
	python3 - "$1" "$2" "$3" "$base" "$pages" <<'PY'
import json, sys
kind, lang, scheme, base, pages = sys.argv[1:6]
# 640 x 400 is a 1280 x 800 window at 200% zoom.
sizes = [(1280, 800), (640, 400), (390, 844), (320, 700)]
items = []
for p in pages.split():
    name, route = p.split("=", 1)
    for w, h in sizes:
        items.append({"name": f"{name}-{lang}-{scheme}-{w}", "url": f"{base}/{route}", "width": w, "height": h,
                      "scheme": scheme, "wait": 4000, "waitFor": "main", "fullPage": True})
    if kind == "audit" and lang == "en":
        items.append({"name": f"{name}-{lang}-{scheme}-1280-reduced", "url": f"{base}/{route}", "width": 1280,
                      "height": 800, "scheme": scheme, "motion": "reduce", "wait": 4000})
print(json.dumps(items))
PY
}

for combo in "en light" "no dark"; do
	read -r lang scheme <<<"$combo"
	setlang "$lang"
	echo "== audit: axe-core $AXE_VERSION and CLS, $lang $scheme"
	plan audit "$lang" "$scheme" >"$out/audit-$lang.json"
	win "$(winpath "$root/scripts/ui/audit.mjs")" "$port" "$(winpath "$out/axe.min.js")" "$(winpath "$out/audit-$lang.json")" >"$out/audit-$lang.log" || true
	report "$out/audit-$lang.log"
	echo "== screenshots, $lang $scheme"
	mkdir -p "$out/shots"
	plan shots "$lang" "$scheme" >"$out/shots-$lang.json"
	win "$(winpath "$root/scripts/ui/shots.mjs")" "$port" "$(winpath "$out/shots")" "$(winpath "$out/shots-$lang.json")" | tee "$out/shots-$lang.log"
	if grep -q '"horizontalOverflow":true\|consoleErrors":\[\[' "$out/shots-$lang.log"; then
		echo "FAIL  console errors or horizontal overflow (see above)"
		failures=$((failures + 1))
	else
		echo "PASS  no console errors and no horizontal overflow at 1280, 640 (200% zoom), 390 and 320"
	fi
done
setlang en

echo "== keyboard: shortcuts, palette, terminal"
win "$(winpath "$root/scripts/ui/keys.mjs")" "$port" "$base/#/session/$active" >"$out/keys.log" || true
report "$out/keys.log"

echo "== Tab order"
win "$(winpath "$root/scripts/ui/tabwalk.mjs")" "$port" "$base/#/session/$active" 18 | tee "$out/tabwalk.log"
if grep -v ' body ' "$out/tabwalk.log" | grep -q 'NO-OUTLINE'; then
	echo "FAIL  a focus stop has no visible outline"
	failures=$((failures + 1))
else
	echo "PASS  every focus stop shows its outline, and Esc then Tab leaves the terminal"
fi

echo "== terminal"
win "$(winpath "$root/scripts/ui/latency.mjs")" "$port" "$base/#/session/$active" 50 stability | tee "$out/latency.log"
if grep -q 'same terminal element after .*: true' "$out/latency.log" && grep -q '50/50 echoed' "$out/latency.log"; then
	echo "PASS  every keystroke echoed, and the terminal was never remounted"
else
	echo "FAIL  terminal latency or stability (see above)"
	failures=$((failures + 1))
fi

echo
echo "screenshots and logs: $out"
if [[ "$failures" -gt 0 ]]; then
	echo "ui-check: $failures check(s) failed"
	exit 1
fi
echo "ui-check: every check passed"
