# RootCause

Hands-on IT troubleshooting practice. Each exercise is a broken Linux machine with a planted fault, delivered as a helpdesk ticket. The learner finds and fixes the fault in a terminal, an automated check grades the result, and the next exercise adapts to the learner's weak spots.

**Status:** in development.

See [`docs/SPEC.md`](docs/SPEC.md) for the full specification.

## Local setup

RootCause runs in WSL2 or Linux with Docker Engine and Go. Each exercise runs in a container where you are root, so root in the container must not be root on your machine. Docker must use user namespace remapping or rootless mode. The CLI refuses to start sessions otherwise.

1. Create `/etc/docker/daemon.json` with `{"userns-remap": "default"}`, then run `sudo systemctl restart docker`. Docker then uses a separate storage area, so images you pulled before are pulled again.
2. Check that `docker info --format '{{json .SecurityOptions}}'` shows `name=userns`.
3. Copy `deploy/compose/.env.example` to `deploy/compose/.env` and set a password.
4. Run `go run ./cmd/rootcause init`, then `go run ./cmd/rootcause list`.

## License

PolyForm Noncommercial 1.0.0. See [`LICENSE`](LICENSE).
