# ADR 0013 · The web terminal goes through the Kubernetes exec API

Accepted on 2026-10-27.

## Context

Kubernetes mode shows the terminal in the browser. The first plan was to run ttyd inside the session pod.

## Decision

- The terminal is xterm.js in the dashboard, on a WebSocket to the API. The API runs `bash` in the session pod through the Kubernetes exec API.
- The WebSocket carries the same token as the API, as a subprotocol. The Origin must equal the Host, and the socket closes when the session ends.

## Alternatives considered

ttyd in each pod. It needs a listening port and a NetworkPolicy that opens every sandbox to `rootcause-system`, plus a pinned binary in every image.

## Consequences

- Nothing in a sandbox listens on the network, so sandboxes stay closed to incoming traffic.
- The exercise images need no extra binary.
- `rootcause shell` is not available in Kubernetes mode, where the browser is the terminal.
