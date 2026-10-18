# RootCause

Hands-on IT troubleshooting practice. Each exercise is a broken Linux machine with a planted fault, delivered as a helpdesk ticket. The learner finds and fixes the fault in a terminal, an automated check grades the result, and the next exercise adapts to the learner's weak spots.

| | |
|---|---|
| Owner and author | DIZAMU |
| Repository | github.com/DIZAMU/RootCause |
| License | PolyForm Noncommercial 1.0.0 |
| Visibility | Private. Changes only by owner decision |
| Spec status | v1 draft |

The product and repository are named RootCause. The CLI command and binary stay lowercase: `rootcause`. The Go module path is `github.com/DIZAMU/RootCause`.

## 1. Purpose

### 1.1 Problem

New IT and operations staff learn troubleshooting on production systems or not at all. Courses teach commands, but few let you practice fixing a broken machine safely, repeatedly, in both English and Norwegian.

### 1.2 Goals

- Proof project for IT and system engineer roles: containers, Kubernetes, CI/CD, a data pipeline and security design in one working system.
- Daily practice tool for the owner.
- Usable by others later, under the noncommercial license.

### 1.3 Non-goals for v1

- Public hosting or multiple users
- Accounts or login
- Windows or Active Directory exercises
- Mobile layout
- Commercial use

## 2. Locked decisions

| Area | Decision |
|---|---|
| Engine language | Go: CLI, local server, grading, sandbox control |
| Dashboard | React + TypeScript (Vite), built and embedded in the Go binary |
| Database | PostgreSQL, run with Docker Compose in v1 |
| Transformations | dbt Core with the Postgres adapter, run in a container |
| Fault and check scripts | Bash |
| Sandboxes v1 | Docker Engine inside WSL2 |
| Sandboxes v2 | Kubernetes with k3d inside WSL2 |
| CI/CD | GitHub Actions. GitOps with Argo CD in v2 |
| Interface v1 | CLI plus local web dashboard |
| Accounts v1 | None. Single local profile |
| Languages | English and Norwegian from day one |
| Exercise framing | Helpdesk tickets |
| Scoring | 1 to 3 stars based on hints used |
| Time | Recorded, never scored |
| Adaptation | Per-topic levels with a 2 up / 2 down rule. Elo considered later |
| Randomized variants | Designed in from v1, implemented in at least 3 exercises |
| Commit style | Conventional Commits, SSH-signed |

## 3. Concepts

- **Exercise:** a folder defining a healthy machine, a fault, a check, a reference solution and texts.
- **Variant:** one concrete version of an exercise, chosen by a seed. Same fault type and difficulty, different details (file names, ports, users, ticket sender).
- **Session:** one attempt at one variant in one sandbox, from start until solved, given up or expired.
- **Sandbox:** the isolated container or pod a session runs in. Disposable.
- **Check:** the automated grader. Tests the end state, never the commands typed.
- **Hint tiers:** hint 1 says where to look, hint 2 which tool to use, hint 3 nearly gives the answer.
- **Walkthrough:** the explained solution, shown after solving or giving up.
- **Topic level:** the learner's level per topic, 1 to 3 in v1.

## 4. Exercise format

### 4.1 Folder layout

```
exercises/svc-01-intranet-config/
├── exercise.yaml      metadata and runtime settings
├── Dockerfile         builds the healthy machine
├── break.sh           plants the fault at session start
├── check.sh           grades the end state
├── solution.sh        reference fix, used only by CI
└── i18n/
    ├── en.yaml        ticket, hints, walkthrough, check feedback
    └── no.yaml
```

### 4.2 exercise.yaml

```yaml
id: svc-01-intranet-config
version: 1
topic: services            # permissions | services | logs | networking | disk
difficulty: 1              # 1 to 3 in v1
estimated_minutes: 10
network: none              # none | internal
resources:
  memory: 256Mi
  cpus: 0.5
session_ttl_minutes: 60
check:
  runs_from: sandbox       # sandbox | runner
  before: none             # none | restart
variants:
  site_name: [intranet, wiki, portal]
  port: [8080, 8081, 8088]
```

### 4.3 Script contracts

**break.sh**
- Receives `RC_SEED` and every variant value as `RC_VAR_<NAME>` environment variables.
- Deterministic: same seed, same fault.
- Copied into the sandbox at session start, run as root, then deleted. Leaves no trace of itself.
- May print derived values as one line of JSON (example: `{"line": 14}`). These are stored with the session and fill placeholders in hints and the walkthrough.

**check.sh**
- Exit 0: pass. Exit 1: fail. Exit 2: the check could not run (infrastructure error, never counted against the learner).
- Prints one line of JSON: `{"passed": false, "feedback": "site_unreachable"}`. The feedback key maps to a text in i18n that describes the symptom, never the fix.
- Accepts every valid fix and rejects shortcuts. Each check has at least one anti-shortcut assertion. Example: the config is valid and the original site content is served, so replacing the config with a default one fails.
- Copied in at check time and deleted afterwards, so the criteria can't be read during the session.

**solution.sh**
- The reference fix. Never copied into a sandbox image or session. Used only by CI.

### 4.4 Texts (i18n/en.yaml)

```yaml
ticket:
  title: "The intranet is down"
  from: "{sender_name}, {department}"
  body: |
    Hi, since this morning nobody in {department} can open {site_name}.
    Someone changed something yesterday. Can you take a look?
hints:
  - "Check whether the web server accepts its own configuration."
  - "nginx has a built-in config test: nginx -t"
  - "Line {line} of the config file has a syntax error."
walkthrough: |
  Explains the cause, how to find it, the fix, and why the fix works.
feedback:
  site_unreachable: "The site still doesn't respond."
```

Placeholders are filled from variant values and derived values. Both language files must define the same keys. CI enforces this.

## 5. v1 exercise catalog

| ID | Topic | Ticket | Fault (hidden) | Check passes when | Anti-shortcut |
|---|---|---|---|---|---|
| perm-01-shared-folder | permissions | Økonomi can't save to the shared folder | Group write missing or wrong group owner on the share | A group member can create and edit a file | Share is not world-writable |
| perm-02-backup-denied | permissions | The nightly backup fails with "permission denied" | Script lacks the execute bit, or the backup target isn't writable by the backup user | Backup runs as the backup user and produces an archive | Script is not world-writable, backup user is not root |
| svc-01-intranet-config | services | The intranet went down after a config change | Syntax error in the nginx config | Config test passes and the original page is served | Default page or empty config fails |
| svc-02-dies-on-reboot | services | The app works until the server reboots | Service not set to start at boot | App is running after a sandbox restart | Starting it by hand fails, since the check restarts first |
| log-01-crash-on-start | logs | The app crashes on start | Wrong path or missing value in the app's config, visible in its log | App runs and its health endpoint returns OK | Health endpoint must report the correct config loaded |
| net-01-no-names | networking | The server reaches IP addresses but not names | Name resolution misconfigured (resolver address or lookup order) | Internal host names resolve to the expected address | Hard-coding the answer in the hosts file fails |
| net-02-outside-only | networking | The site works on the server but not from outside | Service listens only on the loopback address | A request from outside the sandbox succeeds | Check runs from the runner, not inside |
| disk-01-stopped-saving | disk | The server stopped saving data | Data volume full because of a large hidden file | Free space above threshold and the app writes again | Required data files still exist |

Notes:
- Sandboxes have no internet access, so net-01 runs its own small DNS server (dnsmasq) inside the sandbox with an internal zone.
- disk-01 mounts a size-limited tmpfs as the data volume.
- svc-02 depends on OQ-1. Fallback: replace it with a cron exercise, "The nightly report never arrives".
- Randomized variants in v1: svc-01, perm-01 and net-02 at minimum.

## 6. Grading and fairness

| Result | Stars |
|---|---|
| Solved, no hints | 3 |
| Solved with hint 1 or 2 | 2 |
| Solved after hint 3 | 1 |
| Gave up | 0, walkthrough shown |

- Time is recorded for statistics, never used in scores or levels.
- Unlimited checks. Failed checks don't cost stars.
- Reset gives a fresh copy of the same variant with no penalty. Revealed hints stay revealed.
- Give up becomes available once hint 3 is revealed.
- Infrastructure errors (check exit 2, sandbox crash) void the session. It doesn't count.
- An expired session counts as a fail only if at least one check was run.

## 7. Adaptation

### 7.1 Topic levels (engine, applied instantly)

- Every topic starts at level 1. Range 1 to 3 in v1.
- Level up: 2 consecutive solves in the topic with at least 2 stars, at the current level.
- Level down: 2 consecutive fails in the topic (gave up, or expired after a check).
- Each change is logged as a `level_changed` event.

### 7.2 Next exercise (reads dbt marts)

1. Review first: an exercise failed or solved with 1 star at least 3 days ago returns as a new variant.
2. Otherwise the weakest topic: lowest level, ties broken by least recently practiced.
3. Within the topic: difficulty closest to the topic level, unseen first, then least recently attempted.
4. Never the same exercise twice in a row.

### 7.3 Calibration

dbt computes solve rate, hint use and duration per exercise. An exercise is flagged when difficulty 1 has a solve rate under 50%, or difficulty 3 has a solve rate over 90%, once it has at least 10 sessions. With one user in v1, flags are informational.

### 7.4 Later

Elo-style ratings for learners and exercises once the catalog and user count justify it.

## 8. Architecture

### 8.1 v1: local

```
Windows 11 laptop
└── WSL2 (Ubuntu)
    ├── rootcause (Go binary)
    │   ├── CLI
    │   ├── local server: API + embedded dashboard, 127.0.0.1 only
    │   └── Docker runtime (Docker Engine API)
    └── Docker Engine
        ├── postgres (Compose)
        ├── dbt (container, run on demand)
        └── sandbox containers, one per session

Browser on Windows → http://localhost:7300
```

In v1 the terminal is the learner's own: `rootcause shell` opens a shell inside the sandbox.

### 8.2 v2: local cluster

```
WSL2
└── k3d cluster: 1 server, 2 agents
    ├── namespace rootcause-system
    │   ├── api + dashboard (Deployment)
    │   ├── postgres (StatefulSet)
    │   ├── dbt (on-demand Job + CronJob)
    │   └── argocd
    └── namespace sandboxes
        └── one pod per session: exercise image + ttyd web terminal
```

- Reached from Windows through k3d port mapping to localhost. Tailscale only if remote access is needed.
- The browser terminal is ttyd inside the session pod, proxied through the API. Sandboxes are never exposed directly.

### 8.3 Runtime abstraction

All sandbox control goes through one interface, so v2 swaps the implementation, not the callers.

```go
type Runtime interface {
    Create(ctx context.Context, spec SandboxSpec) (Sandbox, error)
    Exec(ctx context.Context, id string, cmd []string, stdin io.Reader) (ExecResult, error)
    CopyIn(ctx context.Context, id string, path string, data []byte) error
    Restart(ctx context.Context, id string) error
    Destroy(ctx context.Context, id string) error
    List(ctx context.Context) ([]Sandbox, error)
}
```

Implementations: `internal/runtime/docker` (v1) and `internal/runtime/kubernetes` (v2, client-go).

### 8.4 Session lifecycle

1. The selector picks an exercise and a seed.
2. The runtime creates the sandbox from the exercise image with its resource limits and network mode.
3. break.sh is copied in, run with the seed and variant values, then deleted. Derived values are stored with the session.
4. The ticket is shown in the CLI and the dashboard.
5. The learner works. Hints, checks and resets are logged as events.
6. On check: optional restart, then check.sh runs in the sandbox or from the runner, and the result is logged.
7. On solve or give up: walkthrough shown, stars and level rule applied, sandbox destroyed, dbt build triggered.
8. A reaper destroys sandboxes past their TTL.

## 9. Data

### 9.1 Source tables (written by the engine)

- `learner`: one row in v1.
- `exercise`: synced from exercise.yaml on startup (id, version, topic, difficulty).
- `session`: id, learner, exercise id and version, seed, variant values, derived values, status, sandbox id, started_at, ended_at, expires_at.
- `event`: id, occurred_at, session_id, type, payload (jsonb).

Event types: `session_started`, `hint_revealed`, `check_run`, `reset`, `gave_up`, `solved`, `expired`, `voided`, `level_changed`.

Migrations are versioned SQL files in `internal/store/migrations`, applied on startup.

### 9.2 dbt models

| Layer | Model | Purpose |
|---|---|---|
| staging | stg_events, stg_sessions | Typed, cleaned source data |
| intermediate | int_session_outcomes | One row per finished session: outcome, stars, hints, checks, duration |
| marts | mart_topic_levels | Current level per topic, from level_changed events |
| marts | mart_review_queue | Exercises due for review |
| marts | mart_exercise_stats | Solve rate, hint use, duration, calibration flag |
| marts | mart_progress | Per-topic history for the dashboard |

Tests: unique and not-null keys, accepted values for event types and outcomes, relationships between sessions and events.

### 9.3 When dbt runs

- v1: after each finished session, and on `rootcause stats --refresh`.
- v2: an on-demand Job after each session, plus a CronJob every 15 minutes as backstop.
- If marts are missing or stale, the selector falls back to topic levels alone.

## 10. CLI (v1)

| Command | Does |
|---|---|
| `rootcause init` | Starts Postgres, runs migrations, creates the local profile |
| `rootcause next` | Starts the recommended next exercise |
| `rootcause start <id>` | Starts a specific exercise |
| `rootcause shell` | Opens a shell in the active sandbox |
| `rootcause ticket` | Shows the active ticket again |
| `rootcause hint` | Reveals the next hint tier |
| `rootcause check` | Grades the active session |
| `rootcause reset` | Fresh copy of the same variant |
| `rootcause giveup` | Ends the session and shows the walkthrough (after hint 3) |
| `rootcause status` | Topic levels, active session, review queue |
| `rootcause list` | All exercises with best stars |
| `rootcause serve` | Starts the local dashboard |
| `rootcause lang en\|no` | Sets the interface language |

## 11. Dashboard (v1)

- Home: recommended next exercise, topic levels, review queue.
- Exercise view: ticket, hint buttons, check, reset, give up, result and walkthrough. Shows the `rootcause shell` command (v2 embeds the terminal).
- History: past sessions with stars and outcome.
- Exercise list with best stars per exercise.
- EN/NO toggle.
- Own visual identity, light and dark mode, desktop-first.

## 12. Languages

- UI strings live in `locales/en.json` and `locales/no.json`. No user-facing text hard-coded in Go or React.
- Exercise texts live in `exercises/<id>/i18n/{en,no}.yaml`.
- CI fails if a key exists in one language and not the other.
- Code, comments, commit messages and docs are in English.

## 13. Security model

### 13.1 v1 (Docker)

- Sandboxes never get the Docker socket, the host network or privileged mode.
- `--cap-drop ALL`, then only the capabilities an exercise needs.
- Memory, CPU and process-count limits on every sandbox.
- Network `none`, or an internal network with no outside route.
- The learner is root inside the sandbox, so container root must not map to host root: user namespace remapping or rootless Docker (OQ-3).
- The local server binds to 127.0.0.1 only.

### 13.2 v2 (Kubernetes) adds

- The API ServiceAccount may only create, read, exec into and delete pods in `sandboxes`. Nothing else.
- `sandboxes` has a default-deny NetworkPolicy, a ResourceQuota and a LimitRange.
- Session pods: no service account token mounted, seccomp `RuntimeDefault`, no hostPath volumes.
- Secrets never enter the repo. Local secrets live in git-ignored `.env` files. Cluster secrets are created outside git or sealed (decided in Phase 7).

## 14. CI/CD

| Workflow | Runs on | Does |
|---|---|---|
| `ci.yml` | Every push and PR | go vet, golangci-lint, go test, shellcheck on all scripts, frontend type check and build, i18n key parity |
| `exercises.yml` | Changes under `exercises/` | For each changed exercise and 3 seeds: build the image; break then check must fail; break, solution, check must pass |
| `dbt.yml` | Changes under `dbt/` | dbt build against a Postgres service container with seed data |
| `commit-policy.yml` | Every PR | Rejects Co-authored-by trailers, tool attribution lines, author emails other than the owner's, and unsigned commits |
| `release.yml` (v2) | Tags on main | Builds and pushes images to GHCR, updates image tags in `deploy/k8s`. Argo CD syncs |

- Private repo on the free plan means limited Actions minutes. Only changed exercises are validated per PR. Full validation runs on tags.
- GHCR images are private while the repo is private, so the cluster needs an image pull secret.
- Images target linux/amd64 only, until something needs ARM.

## 15. Git and authorship

- Author name `DIZAMU`, email the GitHub noreply address.
- Every commit is made by the owner after reviewing the diff. No tool commits or pushes to this repository.
- Conventional Commits: `feat:`, `fix:`, `docs:`, `test:`, `refactor:`, `ci:`, `chore:`.
- No `Co-authored-by` trailers. No generator or tool attribution lines in commits, PRs, code comments or docs.
- Commits are signed with an SSH key (`gpg.format ssh`), so GitHub shows Verified.
- Enforcement: `.githooks/commit-msg` (enabled with `git config core.hooksPath .githooks`) rejects violating messages, and `commit-policy.yml` checks again in CI.
- Tool-specific local config stays out of the repo through the owner's global gitignore.
- Short-lived feature branches, merged to `main` through a PR so CI runs first.

## 16. Rules for anyone or any tool writing code here

1. Work one phase at a time, in order. A phase is done when its acceptance criteria pass.
2. Never commit or push. Leave changes for the owner to review and commit.
3. Every change comes with a short plain-language explanation: what changed, why, and how to verify it.
4. Ask before adding a dependency. State why, and the alternative considered.
5. Never write secrets, tokens or kubeconfig files to the repo.
6. Sandboxes are never privileged, never get the Docker socket, never use the host network.
7. `solution.sh` never goes into an image or a session.
8. Every exercise change ships with passing CI validation.
9. Docs and UI text: plain, short sentences. No em dashes.

## 17. Development environment

- Windows 11 laptop: 4 cores / 8 threads, x64, 16 GB RAM (not upgradeable), SSD.
- All work happens inside WSL2 (Ubuntu). The repo lives under `~/`, never under `/mnt/c`.
- `.wslconfig` caps WSL2 memory at 8 GB.
- Docker Engine installed directly in WSL2, not Docker Desktop.
- Toolchain: Go (latest stable), Node LTS (build time only, for the dashboard). dbt runs in a container.
- v1 resource budget: at most 2 concurrent sandboxes, default 256 MiB and 0.5 CPU each.
- v2 resource budget: k3d with 1 server and 2 agents. If memory is tight, measure Argo CD first (OQ-4).

## 18. Phases and acceptance criteria

### Phase 0: Environment and repo
- WSL2, Docker Engine, Go and Node installed. `.wslconfig` set.
- Private repo `DIZAMU/RootCause` created with a README stub and this spec in `docs/`.
- LICENSE contains the full PolyForm Noncommercial 1.0.0 text, with this as its first line:
  `Required Notice: Copyright (c) 2026 DIZAMU (https://github.com/DIZAMU/RootCause)`
- Git identity, SSH signing and `.githooks/commit-msg` active.

Done when: a signed commit shows Verified on GitHub, and the hook rejects a test message containing a Co-authored-by trailer.

### Phase 1: Exercise format and first exercise
- exercise.yaml schema and a Go loader with validation.
- svc-01 complete in both languages.
- `scripts/validate-exercise.sh <id> <seed>`: break then check fails; break, solution, check passes.

Done when: validation passes locally for 3 seeds.

### Phase 2: CLI core on Docker
- Runtime interface and Docker implementation.
- `init`, `start`, `shell`, `ticket`, `hint`, `check`, `reset`, `giveup`, `status`, `list`.
- Postgres via Compose, migrations, events written.
- Stars, the topic-level rule and the session reaper.
- OQ-3 resolved for v1.

Done when: svc-01 can be played end to end from the CLI, and every action appears as an event.

### Phase 3: Full v1 catalog
- The remaining 7 exercises. Variants in svc-01, perm-01 and net-02 at minimum.
- OQ-1 and OQ-2 resolved.

Done when: all 8 exercises pass validation for 3 seeds each.

### Phase 4: dbt and adaptive selection
- dbt project with the models and tests from section 9.2.
- `rootcause next` uses the marts, with fallback.

Done when: dbt build passes, and after a scripted series of sessions the selector picks the expected exercises.

### Phase 5: Dashboard
- React + TypeScript app in `web/`, embedded in the binary and served by `rootcause serve`.
- The views from section 11, with the EN/NO toggle.

Done when: a full session can be followed in the dashboard alongside the CLI, in both languages.

### Phase 6: CI/CD
- The workflows from section 14, except `release.yml`.

Done when: all workflows pass on main, and a PR with a broken exercise or a Co-authored-by trailer is rejected.

**v1 complete.** Public readiness is reviewed against section 19. The repo stays private until the owner decides.

### Phase 7: Kubernetes runtime
- k3d cluster, namespaces, RBAC, NetworkPolicy and quotas.
- Kubernetes runtime implementation. API and dashboard run in the cluster, with the ttyd terminal in the dashboard.

Done when: a session runs fully in the cluster from the browser, a sandbox pod can't reach the internet or `rootcause-system`, and the API ServiceAccount can't touch anything outside `sandboxes`.

### Phase 8: GitOps
- `release.yml`, GHCR images, Argo CD syncing `deploy/k8s`.

Done when: a merged change reaches the cluster without manual kubectl.

### Phase 9: Hardening and presentation
- OQ-3 resolved for v2, and the security model reviewed against section 13.
- README with an architecture diagram and a 60-second demo recording.
- Architecture decision records in `docs/adr/` for each locked decision.

## 19. Public readiness checklist

The owner decides when the repo goes public. Ready means:

- Phases 0 to 6 complete, CI green.
- README explains the purpose and architecture, and how to run it locally in under 10 minutes.
- Demo recording in the README.
- Git history scanned for secrets, none found.
- LICENSE present, first line reading `Required Notice: Copyright (c) 2026 DIZAMU (https://github.com/DIZAMU/RootCause)`.

## 20. Open questions

| ID | Question | Options | Decide by |
|---|---|---|---|
| OQ-1 | Service exercises usually rely on systemd, which often needs extra container privileges | a) systemd in the container, if it runs unprivileged on this setup; b) a lighter service manager in the image; c) replace svc-02 with a cron exercise | Phase 3 |
| OQ-2 | Docker manages resolv.conf inside containers | Verify edits persist through a session. Otherwise the fault lives in lookup order or the local DNS server config | Phase 3 |
| OQ-3 | The learner is root inside the sandbox | userns-remap or rootless Docker in v1; gVisor or user namespaces in v2 | Phase 2 (v1), Phase 9 (v2) |
| OQ-4 | Argo CD memory use on a 16 GB laptop next to the cluster | Argo CD, or Flux if too heavy | Phase 8 |
| OQ-5 | Public live demo | Recording only, or Oracle Cloud free tier (ARM, idle reclaim risk) | After v1 |
| OQ-6 | Elo ratings | Keep 2 up / 2 down, or switch | When the catalog passes about 30 exercises |

## 21. Repository layout

```
RootCause/
├── cmd/rootcause/            CLI entry point
├── internal/
│   ├── cli/
│   ├── exercise/             loader, schema validation
│   ├── runtime/
│   │   ├── docker/
│   │   └── kubernetes/
│   ├── session/              lifecycle, reaper
│   ├── grading/              stars, level rule
│   ├── selector/             next-exercise logic
│   ├── store/                Postgres access, migrations
│   ├── server/               HTTP API, embeds web/dist
│   └── i18n/
├── web/                      React + TypeScript (Vite)
├── exercises/<id>/
├── dbt/
├── deploy/
│   ├── compose/              v1: Postgres, dbt
│   └── k8s/                  v2: manifests
├── locales/                  en.json, no.json
├── scripts/                  validate-exercise.sh
├── .githooks/                commit-msg
├── .github/workflows/
├── docs/
│   ├── SPEC.md
│   └── adr/
├── LICENSE
└── README.md
```

## 22. Out of scope for now

- Windows and Active Directory exercises (later, as VM-based sandboxes)
- Accounts, multiple users, public hosting
- Proxmox or physical homelab hardware
- Mobile app
