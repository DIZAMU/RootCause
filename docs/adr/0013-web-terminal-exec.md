# ADR 0013: Web terminal through the Kubernetes exec API

Status: Accepted (Phase 7). Recorded 2026-10-27.

## Context

v2 shows the terminal in the browser. The spec first named ttyd inside the session pod.

## Decision

- The terminal is xterm.js in the dashboard, on a WebSocket to the API. The API runs `bash` in the session pod through the Kubernetes exec API.
- The WebSocket carries the same token as the API (as a subprotocol), the Origin must equal the Host, and the socket closes when the session ends.

## Alternatives considered

ttyd in each pod: a listening port and a NetworkPolicy opening every sandbox to `rootcause-system`, plus a pinned binary in every image.

## Consequences

- Nothing in a sandbox listens on the network, so sandboxes stay closed to ingress.
- No extra binary in the exercise images.
- `rootcause shell` is not available on Kubernetes; the browser is the terminal there.
