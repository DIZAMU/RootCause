# ADR 0002 · The dashboard is embedded in the binary and local only

Accepted on 2026-10-27.

## Context

The learner wants a browser view next to the terminal. Another website open in the same browser must not be able to drive the API.

## Decision

- The dashboard is React and TypeScript built with Vite, and embedded with `go build -tags webdist` (ARCHITECTURE.md section 11).
- `rootcause serve` binds to 127.0.0.1 only. It refuses any Host header other than `127.0.0.1:PORT` or `localhost:PORT`, which blocks DNS rebinding. Every `/api` request must carry a token made per server process, and only the served `index.html` contains it. The server sends no CORS headers and forbids framing.
- In the cluster the same server runs with `--listen-all`, which is refused unless `RC_RUNTIME=kubernetes`. k3d publishes it on the host's 127.0.0.1 only.
- The dashboard follows the CLI by polling. The two front ends share only the database.

## Consequences

- Node is needed only to build the dashboard. `go build` without the tag still works and serves a page that explains how to build it.
- The Host check and the token each stop a different attack, and neither is enough alone.
