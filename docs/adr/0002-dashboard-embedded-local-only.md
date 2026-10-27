# ADR 0002: Dashboard embedded in the binary, local only

Status: Accepted (Phase 5). Recorded 2026-10-27.

## Context

The learner wants a browser view next to the terminal. Another website open in the same browser must not be able to drive the API.

## Decision

- The dashboard is React and TypeScript built with Vite, embedded with `go build -tags webdist` (spec section 11).
- `rootcause serve` binds to 127.0.0.1 only. It refuses any Host header other than `127.0.0.1:PORT` or `localhost:PORT` (DNS rebinding), and every `/api` request must carry a per-process token that only the served `index.html` contains. No CORS headers; framing is forbidden.
- In the cluster the same server runs with `--listen-all`, which is refused unless `RC_RUNTIME=kubernetes`; k3d publishes it on the host's 127.0.0.1 only.
- The dashboard follows the CLI by polling; the two front ends share only the database.

## Consequences

- Node is needed only to build the dashboard; `go build` without the tag still works and serves a page that says how to build it.
- The Host check and the token each stop a different attack, and neither is enough alone.
