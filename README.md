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
5. Run `go run ./cmd/rootcause next` to start the recommended exercise. Statistics are built with dbt in a container (the first run pulls the image).

### Dashboard

The dashboard is a React app in `web/`, built into the binary. Building it needs Node 24 (see `web/.nvmrc`), and only at build time.

1. Run `scripts/build-web.sh`. It runs `npm ci` from the committed lockfile, a type check, and `vite build` into `web/dist`.
2. Build the binary with the dashboard embedded: `go build -tags webdist -o rootcause ./cmd/rootcause`.
3. Run `./rootcause serve` and open http://localhost:7300 in a browser on Windows. Stop it with Ctrl-C.

The server listens on 127.0.0.1 only. WSL2 forwards `localhost` from Windows to it, so no `.wslconfig` change is needed. Work in the sandbox with `./rootcause shell` in a WSL terminal next to the browser. The dashboard follows the CLI: hints, checks and results from either side show up in both. A binary built without `-tags webdist` still runs `serve`, but shows a page that says how to build the dashboard.

Exercises that boot systemd (such as svc-02) need a host with cgroup v2. WSL2 uses cgroup v2 by default. Check with `docker info --format '{{.CgroupVersion}}'`, which must print `2`.

## License

PolyForm Noncommercial 1.0.0. See [`LICENSE`](LICENSE).
