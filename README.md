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

### Kubernetes (v2)

The same sessions can run in a local k3d cluster (spec section 8.2): the API and dashboard, Postgres and dbt in the namespace `rootcause-system`, one pod per session in `sandboxes`, and the terminal in the browser.

1. Run `scripts/install-k8s-tools.sh`. It puts pinned, checksum-verified `k3d` and `kubectl` in `~/.local/bin`, without sudo.
2. Run `scripts/k8s-up.sh`. It creates the cluster (1 server, 2 agents), builds and imports the images, generates the database password into git-ignored `deploy/k8s/.env`, and deploys everything. Re-run it after a change.
3. Open http://localhost:7300. Start an exercise and work in the terminal on the exercise page.
4. Run `scripts/k8s-prove.sh` to check the isolation: the API's permissions, the network policies, and one full session through the dashboard. That session plays on a separate database, `rootcause_cluster_test`, through a second API pod, and leaves the real one untouched.
5. Run `scripts/k8s-down.sh` to stop the cluster and free memory; its data stays. `scripts/k8s-dbt.sh` rebuilds the statistics at once instead of waiting for the 10-minute CronJob.

`RC_RUNTIME=kubernetes` points the CLI at the cluster, for example `RC_RUNTIME=kubernetes scripts/validate-exercise.sh perm-01-shared-folder 1`. svc-02 boots systemd, which does not run in a pod, so it is refused there (spec section 13.3).

## Tests

Unit tests need nothing: `go test ./...`. Integration tests need a database whose name ends in `_test`, and Docker with userns-remap:

```bash
docker exec rootcause-postgres-1 createdb -U rootcause rootcause_test
export RC_TEST_DATABASE_URL="postgres://rootcause:<password>@127.0.0.1:5432/rootcause_test?sslmode=disable"
go test -count=1 ./...
```

- A test skips only when `RC_TEST_DATABASE_URL` is unset, or when Docker is missing. If the variable is set but the database is unreachable or not a `_test` database, the test fails.
- `RC_REQUIRE_DOCKER=1` makes a missing Docker fail the tests too. CI sets it.
- Tests that run dbt create and drop their own database (for example `rootcause_selector_test`), because a dbt build reads every row in its database.
- `RC_DBT_STRESS=1 go test -run TestBuildStress ./internal/dbt/` kills dbt builds at random points and checks that the next build succeeds. `RC_DBT_STRESS_ROUNDS` sets the rounds (default 10).
- A failed dbt build saves its full `dbt.log` under `~/.cache/rootcause/dbt-logs/` and prints the path. The newest 20 are kept.
- `TestClusterSession` plays a session against the deployed cluster and runs only with `RC_CLUSTER_URL` set; `scripts/k8s-prove.sh` sets it.

## CI

GitHub Actions runs four workflows (spec section 14). Every action is pinned by commit SHA, and every job gets read-only access to the repository and nothing else.

| Workflow | Check name | Does |
|---|---|---|
| `ci.yml` | `ci-checks`, `ci-test` | gofmt, go vet, golangci-lint, shellcheck, i18n parity, the dashboard build, and every Go test with Postgres and Docker. Fails if any test is skipped |
| `exercises.yml` | `exercises` | Validates the changed exercises with seeds 1, 2 and 3. All of them on tags `v*` and on manual runs |
| `dbt.yml` | `dbt` | dbt build against a Postgres service container, when the dbt project or its inputs changed |
| `commit-policy.yml` | `commit-policy` | Rejects Co-authored-by trailers, tool attribution lines, author emails other than the owner's, and commits GitHub does not show as Verified |

Run the same commit check locally with `scripts/check-commit-policy.sh origin/main HEAD`.

### When the repo goes public

On the free plan, branch protection does not apply to private repositories, so a red check does not block the merge button. The rule until then: never merge a PR with a red check. When the repo goes public, add a ruleset under Settings, Rules, Rulesets:

- Target: the default branch (`main`). Enforcement: Active. No bypass list.
- Restrict deletions, and block force pushes.
- Require a pull request before merging, with merge commits only (squash and rebase drop the SSH signatures).
- Require signed commits.
- Require status checks to pass, with the branch up to date: `ci-checks`, `ci-test`, `exercises`, `dbt`, `commit-policy`. Every workflow starts on every PR and decides inside the job what to run, so none of these checks stays pending.

Known gap to close when the ruleset goes on: the workflows, `scripts/check-commit-policy.sh` and `.githooks/commit-msg` run from the PR's own commits, so a PR could weaken the checks that judge it. Before the repo goes public, make `commit-policy.yml` check out the base branch's copy of the script and hook, and require review of changes under `.github/`, `scripts/` and `.githooks/`.

## License

PolyForm Noncommercial 1.0.0. See [`LICENSE`](LICENSE).
