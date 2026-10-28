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
├── Dockerfile         builds the base machine (no exercise scripts inside)
├── setup.sh           optional: builds the healthy state at session start
├── break.sh           plants the fault at session start
├── check.sh           grades the end state
├── solution.sh        reference fix, used only by CI
└── i18n/
    ├── en.yaml        ticket, hints, walkthrough, check feedback
    └── no.yaml
```

Other files may sit beside these when the Dockerfile copies them into the image, such as an app the machine runs (`app/`). Exercise scripts are never copied into the image.

### 4.2 exercise.yaml

```yaml
id: svc-01-intranet-config
version: 1
topic: services            # permissions | services | logs | networking | disk
difficulty: 1              # 1 to 3 in v1
estimated_minutes: 10
network: none              # none | internal
init: none                 # none | systemd (optional, default none)
resources:
  memory: 256Mi
  cpus: 0.5
tmpfs:                     # optional size-limited in-memory volumes
  - path: /srv/data
    size: 32Mi
session_ttl_minutes: 60
check:
  runs_from: sandbox       # sandbox | runner
  before: none             # none | restart
capabilities: [SETUID, SETGID, CHOWN]   # added back after --cap-drop ALL; only from the loader's allowlist
variants:
  site_name: [intranet, wiki, portal]
  port: [8080, 8081, 8088]
```

- `init: systemd` boots systemd as PID 1, for exercises about services at boot. The image runs systemd as its entrypoint. The sandbox gets in-memory `/run` and `/run/lock` instead of Docker's init, since systemd reaps processes itself. A new sandbox counts as ready only when `systemctl is-system-running` reports `running`. After the check's restart, `degraded` also counts, so the check grades what the learner left.
- `tmpfs` volumes count toward `resources.memory`. Their total must stay below it.
- `runs_from: runner` needs `network: internal` and cannot be combined with `init: systemd`.

### 4.3 Script contracts

**setup.sh** (optional)
- Builds the healthy state from the variant values, for example rendering a config and starting a service.
- Receives the same environment variables as break.sh. Copied in, run as root, then deleted, before break.sh.
- Exists so the healthy state is never baked into the image, where the learner could read it or re-run it as a shortcut.

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
- With `runs_from: runner`, it runs in a separate runner container instead: a fresh copy of the exercise image on the internal network, with no added capabilities, destroyed after the check. `RC_SANDBOX_HOST` holds the sandbox's host name.

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

1. Review first: an exercise returns when its latest counted session (solved, gave up, or expired after a check) was a fail or a 1-star solve that ended at least 72 hours ago. The oldest due exercise goes first. A later solve with 2 or more stars takes it out of the queue. It returns as a new variant: the first of up to 20 random seeds whose variant values differ from the last session's.
2. Otherwise the weakest topic: lowest level, ties broken by least recently practiced (never practiced first), then by topic name.
3. Within the topic: difficulty closest to the topic level, unseen first, then least recently attempted.
4. Never the same exercise twice in a row. This rule wins over rule 1.

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
    │   └── dbt (CronJob every 10 minutes; a Job on demand by script)
    ├── namespace sandboxes
    │   └── one pod per session: the exercise image, nothing added
    └── namespace argocd
        └── Argo CD core: application controller, repo server, redis
```

- Reached from Windows through k3d port mapping to `127.0.0.1:7300` on the host only. Tailscale only if remote access is needed.
- The browser terminal is xterm.js in the dashboard, on a WebSocket to the API. The API runs `bash` in the session pod through the Kubernetes exec API (the v1 `docker exec` model), so nothing in a sandbox listens on the network and sandboxes stay closed to ingress. Phase 7 chose this over ttyd in the pod, which would have needed a listening port and a NetworkPolicy opening every sandbox to `rootcause-system`, plus an extra binary in every exercise image.
- `scripts/k8s-up.sh` creates the cluster, builds the exercise images and imports them into the cluster's nodes (`k3d image import`; sandbox pods never pull), creates the namespaces and the Secrets, installs Argo CD and applies `deploy/argocd`. Argo CD then syncs `deploy/k8s` from `main`, and the API runs the release pinned there by digest from GHCR (section 14). `scripts/k8s-up.sh --dev` instead builds the API image from the working tree and applies `deploy/k8s-dev`, after turning Argo CD's automated sync off. `scripts/k8s-prove.sh` proves the Phase 7 criteria and `scripts/k8s-gitops-prove.sh` the Phase 8 ones. The CLI uses the cluster with `RC_RUNTIME=kubernetes`; `rootcause selftest` then validates exercises in sandbox pods.
- Argo CD is the core install (no UI, no SSO, no notifications, no ApplicationSet controller) at a pinned version, with its install manifest checked against a pinned SHA-256 and its images pinned by digest. Its one Application belongs to the AppProject `rootcause` (section 13.2).

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

Implementations: `internal/runtime/docker` (v1) and `internal/runtime/kubernetes` (v2, client-go). `RC_RUNTIME` (`docker`, the default, or `kubernetes`) selects one. Optional capabilities sit beside the interface: `Sheller` (v1 `rootcause shell`), `Terminal` (v2 web terminal), `Diagnoser` and `SystemdChecker`.

The Kubernetes runtime makes each sandbox a pod in `sandboxes`. `Create` returns once the container runs, as `docker run -d` does. `Exec` and `CopyIn` go through the exec API (`CopyIn` streams the file to `cat`). `Restart` is not supported (a restarted container starts again from its image); the only exercise that restarts before its check boots systemd, which the runtime refuses first. Pods on the internal network get a hosts entry for every other internal pod, so a check runner reaches its sandbox by name as on docker's internal network.

### 8.4 Session lifecycle

1. The selector picks an exercise and a seed.
2. The runtime creates the sandbox from the exercise image with its resource limits and network mode.
3. setup.sh (if present) and then break.sh are copied in, run with the seed and variant values, then deleted. The values are passed only to these commands, never to the sandbox environment. Derived values are stored with the session.
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
| staging | stg_events, stg_sessions, stg_exercises | Typed, cleaned source data |
| intermediate | int_session_outcomes | One row per finished session: outcome, stars, hints, checks, duration |
| marts | mart_topic_levels | Current level per learner and topic, from level_changed events, and when the topic was last practiced |
| marts | mart_review_queue | Review candidates per learner (rule 1 of section 7.2), with the time each is due |
| marts | mart_exercise_stats | Solve rate, hint use, duration, calibration flag |
| marts | mart_progress | Per learner, topic and day: sessions, solves, average stars, level at the end of the day |
| marts | mart_build_info | One row: when the latest build ran. The dashboard shows it, so statistics that lag behind are visibly older |

dbt reads the source tables in schema `public` and writes every model to schema `analytics`, so a build never touches the engine's tables. Outcomes in `int_session_outcomes` are `solved`, `gave_up`, `expired_fail` (expired after a check), `expired_skip` (expired with no check) and `voided`. Only the first three count in the statistics and the review queue. The engine stays the authority for levels (section 7.1). `mart_topic_levels` replays its `level_changed` events, and a test asserts that the two agree.

Tests: unique and not-null keys, accepted values for event types, statuses and outcomes, relationships between sessions and events. Singular tests assert one level row per learner and topic, and one review row per learner and exercise.

The project lives in `dbt/` and is embedded in the binary. `internal/dbt` extracts it into a fresh temporary directory for each build, so a build never reads the working tree, and runs `dbt build` in the pinned `dbt-postgres` image with `docker run`. The container joins the compose network and reaches Postgres by service name. It runs with no capabilities, a read-only root filesystem, the project mounted read-only, and targets and logs in `/tmp`. The connection comes from the engine's database URL, and the password reaches the container only through its environment. dbt runs only against the compose Postgres, never over the host network. For tests and CI only, `RC_DBT_NETWORK` and `RC_DBT_HOST` name another Postgres container's network and host, such as a CI service container.

Every dbt container is labelled with its database. Before a build, containers left by an earlier build of the same database are removed. A build passes only when dbt exits 0 and runs exactly as many nodes as the project defines, so a run that silently skips models or tests fails. A failed build keeps its full `dbt.log` under the user cache directory and prints the path.

### 9.3 When dbt runs

- v1: after each solve and give up (after the result is shown, with a 120-second limit), on `rootcause stats --refresh`, and in `rootcause next` when the marts are stale. Expiry by the reaper does not run dbt; the next `rootcause next` catches up. In the dashboard, a solve or give up returns at once and dbt runs in the background, as does a rebuild when the home view finds the marts stale; starting the recommended exercise from the dashboard rebuilds stale marts first, like `rootcause next`. A Postgres advisory lock lets only one build run at a time, across the CLI and the dashboard.
- v2: a CronJob every 10 minutes (`concurrencyPolicy: Forbid`). The API's service account may not start Jobs (section 13.2), so the dashboard shows when the marts were last built instead. `scripts/k8s-dbt.sh` runs a build at once. An init container writes the project embedded in the binary (`rootcause dbt-project`), so the cluster builds what the binary ships. The API runs with `RC_DBT_MODE=cronjob` and never starts dbt itself.
- The marts are stale when they are missing, or when the learner's latest finished session is newer than the latest one in `int_session_outcomes`. `rootcause next` then rebuilds them once. If they are still stale, or dbt fails, the selector falls back to topic levels alone: rules 2 to 4 of section 7.2 from the engine's levels and the source tables, without the review queue. A failed build never changes a session's result.

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
| `rootcause stats [--refresh]` | Statistics per exercise from the marts, with calibration flags. `--refresh` rebuilds the marts first |
| `rootcause serve [--port N] [--listen-all]` | Starts the local dashboard on 127.0.0.1, port 7300 by default, until Ctrl-C. Reaps expired sessions every minute while it runs. `--listen-all` is for the cluster deployment only and is refused unless `RC_RUNTIME=kubernetes` |
| `rootcause lang en\|no` | Sets the interface language |

## 11. Dashboard

Rebuilt in Phase 10 as a calm workbench: flat panels divided by hairlines, nearly square corners, Inter for text and JetBrains Mono for ids, commands and the terminal, a teal accent, light and dark (following the system unless switched).

- Sidebar: Inbox, Exercises, History, Stats and the session in progress; the five topic level rings; the EN/NO and theme switches; the keyboard shortcuts. It collapses to icons (remembered per browser), and becomes a top bar on a phone.
- Inbox (home): the tickets as a helpdesk inbox, one row each with title and sender, topic, difficulty, a status tag (next, due, new, tried, solved with best stars) and the exercise id. The recommended ticket is pinned first, with why it was picked: the rule, the last result and date, and what would change that. A session in progress is pinned above everything. A first visit shows one short tip, which can be hidden.
- Session start: the engine's real steps (build the image, start the machine, set up, plant the fault) tick off as they happen. Only the exercise id and the step reach the browser.
- Session: the ticket and hints on the left, the work on the right: a check bar (Run check, Show the next hint with the stars it leaves, Reset, an optional elapsed-time timer, and Give up behind a menu, each destructive action confirmed in place), the last check's result in one line, the terminal (v2) or the `rootcause shell` command (v1), and a check log. A solve plays the app's one expressive animation, under 1.2 s, skippable, never taking focus from the terminal. The terminal closes when its session ends.
- Exercises: the catalog by topic with difficulty, best stars and when it was last played.
- History: finished sessions with outcome, stars, hints, checks and time, filtered by topic and outcome.
- Stats: the topic rings with progress toward the next level, solves per day (one-axis column chart with a data table), and per exercise the sessions, solve rate, hints, time and calibration flag, with "rebuild now" where the server builds the marts itself.
- Keyboard: Ctrl+K opens a command palette; Ctrl+Enter runs the check, from the terminal too; `g` then `i`, `e`, `h` or `s` goes to a page, `[` folds the sidebar and `?` lists the shortcuts. The single-key ones can be switched off and never fire while typing in a field, a dialog or the terminal. The terminal keeps Tab for completion; Esc, then Tab, leaves it.
- EN/NO switch. It sets the profile language, the same one `rootcause lang` sets.
- Accessibility is a floor: WCAG 2.2 AA, every task by keyboard alone, reduced motion respected, layouts that reflow at 320 px and at 200% zoom.

Implementation:

- The app (Vite, React, TypeScript, plain CSS, hash routing) lives in `web/`. `scripts/build-web.sh` builds it into `web/dist` with `npm ci` from the committed lockfile. `go build -tags webdist` embeds it. Without the tag, `/` serves a translated page that says how to build it.
- `internal/server` is a JSON API over the same engine, selector and store as the CLI. Both front ends call the shared steps in `internal/play` (next pick and seed, exercise texts, statistics rebuild). Exercise texts are filled server-side in the learner's language. UI strings come from `locales/*.json` through `/api/i18n/{lang}`, so there is one copy of every string.
- Endpoints: `GET /api/overview` (with the start in progress and each topic's progress toward the next level), `/api/exercises` (with the ticket header and the latest result), `/api/history`, `/api/stats`, `/api/session` (the active session, 204 if none), `/api/sessions/{id}`, `/api/i18n/{lang}`; `POST /api/start` (`{"exercise": ""}` starts the recommended one), `/api/hint`, `/api/check`, `/api/reset`, `/api/giveup`, `/api/stats/rebuild` (one rebuild at a time, 409 while one runs, 404 where a CronJob builds the marts); `PUT /api/lang`; `GET /api/terminal` (WebSocket, v2). Actions name the session the page shows and are refused if it is no longer the active one.
- Before a session exists, the inbox and the catalog show each ticket's header filled with the first value of each variant, a fixed display choice. A session shows its own variant.
- The dashboard follows the CLI by polling: the open session every 2 seconds, the overview every few seconds and on focus. The two front ends share only the database.
- The session view never contains the seed, variant values or derived values. Hints appear once revealed, and the walkthrough once the session is solved or given up.
- Everything the dashboard needs is in the binary: the app, its fonts (SIL OFL, licences next to the files) and its icons. It loads nothing from elsewhere, and the CSP did not change for it. The terminal's code loads only on a session page.
- `scripts/ui-check.sh` checks the dashboard in a real browser on the development laptop: axe-core accessibility rules, no layout shift when data arrives, the keyboard rules, the Tab order, terminal typing latency, the CSP, and screenshots in both languages at 1280, 390 and 320 px. It is not part of CI.

## 12. Languages

- UI strings live in `locales/en.json` and `locales/no.json`. No user-facing text hard-coded in Go or React.
- Exercise texts live in `exercises/<id>/i18n/{en,no}.yaml`.
- CI fails if a key exists in one language and not the other.
- Code, comments, commit messages and docs are in English.
- Counts use a `_one` / `_other` pair of keys, the two plural forms English and Norwegian both have.
- Interface text is plain and warm: verbs first on buttons, praise only for what was done, at most one exclamation mark (on a solve without hints), no streaks, guilt or urgency, no em dashes. Layouts leave room for Norwegian, which runs longer.

## 13. Security model

### 13.1 v1 (Docker)

- Sandboxes never get the Docker socket, the host network or privileged mode.
- `--cap-drop ALL`, then only the capabilities an exercise needs.
- Memory, CPU and process-count limits on every sandbox.
- Network `none`, or an internal network with no outside route.
- The learner is root inside the sandbox, so container root must not map to host root: user namespace remapping or rootless Docker (OQ-3).
- The local server binds to 127.0.0.1 only.
- It refuses any request whose Host header is not `127.0.0.1:PORT` or `localhost:PORT`, which blocks DNS rebinding.
- Every `/api` request must carry the header `X-RootCause-Token`, a random token made per server process and written only into the served `index.html`. Another website cannot read that page, so it cannot call the API. The server sends no CORS headers, and every response forbids framing.
- The web terminal's WebSocket (v2) cannot carry headers, so the browser offers the token as a subprotocol (`rootcause.token.<token>`) and the server answers with `rootcause.terminal` only. The request's `Origin` must be the dashboard itself, and the Host check applies as above.
- The Content-Security-Policy allows scripts from the dashboard only. Styles may be inline, because xterm.js writes its layout into `<style>` elements.

### 13.2 v2 (Kubernetes) adds

- The API ServiceAccount may only create, read, exec into and delete pods in `sandboxes`. Nothing else. It is a Role in `sandboxes`, not a ClusterRole: pods `create`, `get`, `list`, `delete`; `pods/exec` `create` and `get` (exec over WebSocket needs both); `pods/log` `get`. `scripts/k8s-prove.sh` checks a `kubectl auth can-i` matrix of allowed and denied actions.
- `sandboxes` has a default-deny NetworkPolicy, a ResourceQuota (4 pods, 1 GiB, 2 CPUs) and a LimitRange. The only exception to the default deny: a check runner may connect to sandboxes on the internal network, and those accept connections from runners only.
- `rootcause-system` is default-deny too. The API accepts 7300 and reaches Postgres, cluster DNS and the Kubernetes API server's address; dbt reaches Postgres and DNS; Postgres accepts the API and dbt.
- Session pods: their own user namespace (`hostUsers: false`, so pod root is not node root), no service account token mounted, no service links, seccomp `RuntimeDefault`, no hostPath volumes, all capabilities dropped and the exercise's allowlist added back, no privilege escalation, memory and CPU limits, in-memory `emptyDir` for `tmpfs`, and no cluster DNS. The pause container is PID 1 and reaps stopped processes. The process limit is the kubelet's `pod-max-pids` (256), since Kubernetes has no per-pod setting.
- Pod Security Admission enforces `restricted` in `rootcause-system` and `baseline` in `sandboxes`. It stays on next to the admission policy below: it also covers procMount, sysctls, AppArmor, SELinux options and host ports.
- A ValidatingAdmissionPolicy (`deploy/k8s/admission.yaml`, Phase 9) refuses every pod in `sandboxes` that is not built the way the runtime builds it: `hostUsers: false`; no host network, PID or IPC; no hostPath and only `emptyDir` volumes; no privileged container; `allowPrivilegeEscalation: false`; drop ALL and add only the exercise allowlist; seccomp `RuntimeDefault`; no service account token; `imagePullPolicy: Never`; only `rootcause/` images. It applies to containers, init containers and ephemeral containers, with failurePolicy Fail and a Deny binding with no match conditions, so no requester is exempt: not the API, not Argo CD. Argo CD manages it (the AppProject allows exactly these two cluster-wide kinds), so self-heal restores it. `TestSandboxAdmission`, run by `scripts/k8s-prove.sh`, admits every exercise's real pod and checks each forbidden change as both service accounts; `ci.yml` checks the rendered policy's rules and binding.
- Secrets never enter the repo. Local secrets live in git-ignored `.env` files. Cluster secrets are created outside git by `scripts/k8s-up.sh`, from files, so no secret appears in a manifest, on a command line or in output: `rootcause-db` (the Postgres password, generated once into git-ignored `deploy/k8s/.env`), `ghcr-pull` (the owner's GHCR token) and `repo-rootcause` in `argocd` (the owner's read-only deploy key). The owner keeps the token and the key in `~/.config/rootcause`, mode 600. Phase 8 decided against sealed secrets: there are three secrets, all created by one script on one laptop.
- What is not in git is created by `scripts/k8s-up.sh` and never tracked by Argo CD, so prune leaves it: the namespaces, the three Secrets, and the NetworkPolicy to the API server's address (which is known only once the cluster runs).
- The AppProject `rootcause` limits what a bad commit on `main` can make Argo CD do: this repository as the only source, `rootcause-system` and `sandboxes` on this cluster as the only destinations, no cluster-wide resources at all, and only the namespaced kinds `deploy/k8s` uses. The core install has no `default` project. `scripts/k8s-gitops-prove.sh` checks that an Application pointing at another namespace or another repository is refused.
- Argo CD's application controller can write anywhere in the cluster; the AppProject limits what it is asked to do, not what it can do. It is a trust boundary next to the API (ADR 0016).

### 13.3 Known limits (v1)

- Checks run inside the sandbox, where the learner is root. A determined learner could replace a tool the check uses, such as `curl` or `nginx`, and fake a pass. This is acceptable for a single-user practice tool. Exercises where it matters use `runs_from: runner`.
- `check.before: restart` restarts the container. Files stay, but only PID 1 starts again: processes that setup.sh started are gone, and setup.sh does not run again. An exercise that needs services started at boot uses `init: systemd`.
- In `init: systemd` sandboxes, journald and tmp.mount are masked, because they need capabilities outside the allowlist. Service output is not kept in a journal.
- `init: systemd` exercises need a host with cgroup v2 (the WSL2 default). The systemd in the exercise images does not boot on cgroup v1, so the CLI and the selftest refuse such an exercise there with a clear error.
- Sandboxes on the internal network can reach each other. With one learner and at most two sandboxes, this is accepted.

Known limits (v2, Kubernetes):

- k3d runs its node containers privileged and in the host's user namespace, so node root is host root, whatever Docker's userns-remap says. Phase 9 accepted this for a local, single-user cluster (ADR 0011): sandbox pods run in their own user namespaces, enforced by the admission policy for every requester, and system pods run as non-root users in their own user namespaces as well. Rootless k3d is the future fix; it needs sudo and k3d's support for it is experimental.
- Until Phase 9 k3d published the Kubernetes API on the host's 0.0.0.0. Under WSL2's NAT mode it was reachable from Windows, not from the network, and still required the kubeconfig's client certificate. `deploy/k8s/k3d.yaml` now binds it to 127.0.0.1 for a new cluster, and `scripts/k8s-prove.sh` fails if any published port is bound elsewhere.
- The API image's `exercises/` and the exercise images imported by `scripts/k8s-up.sh` come from different trees: the release and the working tree. A changed exercise bumps its `version`, and so its image tag, so a missing image fails the session start with `ErrImageNeverPull` instead of running a stale image.
- A check runner may connect to any sandbox on the internal network, not only its own. With two sessions, one session's runner could reach the other session's sandbox. Per-session NetworkPolicies would need the API to manage NetworkPolicies, which section 13.2 does not allow.
- `init: systemd` exercises (svc-02) do not run on Kubernetes: in a pod with its own user namespace the cgroup filesystem stays owned by host root, so systemd cannot boot. The CLI, the dashboard and the selftest refuse such an exercise there with a clear error. There is no restart before a check either (section 8.3).
- The cluster's dbt CronJob runs `dbt build` and fails on dbt's own exit code; the node-count check of section 9.2 applies to builds the binary starts.

## 14. CI/CD

| Workflow | Runs on | Does |
|---|---|---|
| `ci.yml` | Every PR and push to main | go vet, golangci-lint, go test, shellcheck on all scripts, frontend type check and build, i18n key parity. Postgres comes from `rootcause init`, and any skipped test fails the run |
| `exercises.yml` | Changes under `exercises/` | For each changed exercise and 3 seeds: build the image; break then check must fail; break, solution, check must pass |
| `dbt.yml` | Changes under `dbt/` | dbt build against a Postgres service container with seed data: a scripted series of sessions played through the engine |
| `commit-policy.yml` | Every PR | Rejects Co-authored-by trailers, tool attribution lines, author emails other than the owner's, and unsigned commits |
| `release.yml` (v2) | Tags `vX.Y.Z` on main | Checks that the tag is `vX.Y.Z`, on `main` and not pushed before, validates every exercise (it calls `exercises.yml`), then builds the API image and pushes it to `ghcr.io/dizamu/rootcause-api` under that tag only. The job summary shows the digest. It never commits: the owner pins the digest in `deploy/k8s/kustomization.yaml` in a pull request, `ci.yml` checks it, and Argo CD syncs it once merged |

- Private repo on the free plan means limited Actions minutes. Only changed exercises are validated per PR. Full validation runs on tags.
- `exercises.yml` and `dbt.yml` start on every PR and push to main, and decide inside the job whether anything changed, so they can be required checks.
- Every action is pinned by commit SHA. Workflows have no permissions by default, and each job gets read access to the repository only.
- GHCR images are private while the repo is private, so the cluster needs an image pull secret. GitHub Packages accepts only classic personal access tokens; the pull secret uses one with `read:packages` only and a 90-day expiry.
- Only the API image goes to GHCR. Exercise images stay local (`k3d image import`): all nine images together are about 210 MB compressed, and the free plan's 500 MB of private package storage would fill within about two releases.
- `packages: write` is granted only to `release.yml`'s push job, `packages: read` only to its tag check and to `ci.yml`'s `ci-release-image` job (not on pull requests from forks). GITHUB_TOKEN is the only credential.
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
- v2 resource budget: k3d with 1 server and 2 agents, Argo CD core, at most 2 sandboxes. Measured in Phase 8: about 2.5 GB of the 8 GB WSL2 cap at peak (OQ-4). `scripts/k8s-memory.sh` prints the current use per node and container.

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
- Kubernetes runtime implementation. API and dashboard run in the cluster, with the web terminal (exec into the session pod, see 8.2) in the dashboard.

Done when: a session runs fully in the cluster from the browser, a sandbox pod can't reach the internet or `rootcause-system`, and the API ServiceAccount can't touch anything outside `sandboxes`.

### Phase 8: GitOps
- `release.yml`, GHCR images, Argo CD syncing `deploy/k8s`.
- `scripts/k8s-gitops-prove.sh`: the Application Synced and Healthy at `main` with automated sync, prune and self-heal on; the running API image equals the pinned release digest; a change made by hand is reverted; the project refuses other destinations and sources.

Done when: a merged change reaches the cluster without manual kubectl.

### Phase 9: Hardening and presentation
- OQ-3 resolved for v2, and the security model reviewed against section 13, including the privileged k3d nodes that run as host root (section 13.3).
- README with an architecture diagram. The 60-second demo recording moved to the end of Phase 10, so it shows the final dashboard (owner, 2026-10-27).
- Architecture decision records in `docs/adr/` for each locked decision.

### Phase 10: Dashboard revamp
- The dashboard of section 11, rebuilt to the owner's UI agreement: sidebar, inbox, catalog, history with filters, stats with a chart, the session workbench, command palette and shortcuts.
- The real start steps, progress toward the next level, the statistics view and "rebuild now" in the API.
- The 60-second demo recording, made locally and kept off the internet while the repo is private.

Done when: `scripts/ui-check.sh` passes in both languages: accessibility, no layout shift, reflow at 320 px and at 200% zoom, the keyboard rules (shortcuts, the palette, the check from the terminal, a way out of the terminal), the Tab order, terminal latency unchanged, CSP unchanged.

## 19. Public readiness checklist

The owner decides when the repo goes public. Ready means:

- Phases 0 to 6 complete, CI green.
- A ruleset on `main` requires the CI checks, signed commits and pull requests, as listed in the README.
- README explains the purpose and architecture, and how to run it locally in under 10 minutes.
- Demo recording in the README.
- Git history scanned for secrets, none found.
- LICENSE present, first line reading `Required Notice: Copyright (c) 2026 DIZAMU (https://github.com/DIZAMU/RootCause)`.

## 20. Open questions

| ID | Question | Options | Decide by |
|---|---|---|---|
| OQ-1 | Service exercises usually rely on systemd, which often needs extra container privileges | Resolved in Phase 3: a) systemd as PID 1. It boots under the v1 security flags with no capability outside the allowlist, given in-memory `/run` and `/run/lock`. A unit that is started but not enabled is gone after `docker restart`; an enabled one is running again. journald and tmp.mount are masked (they need SETPCAP). Exercises opt in with `init: systemd` | Phase 3 |
| OQ-2 | Docker manages resolv.conf inside containers | Resolved in Phase 3: `/etc/resolv.conf` is a bind mount. In-place writes persist for the session, but tools that replace the file (`sed -i`, some editors) fail with "Device or resource busy", and `docker restart` regenerates it. So net-01's fault lives in lookup order (`/etc/nsswitch.conf`) or the local DNS server config, and its fix never needs a resolv.conf edit | Phase 3 |
| OQ-3 | The learner is root inside the sandbox | v1 resolved: userns-remap (`/etc/docker/daemon.json` with `{"userns-remap": "default"}`). Chosen over rootless Docker because it keeps the existing daemon and needs one config change. Verified with `docker info` showing `name=userns`; container root maps to host UID 100000. The CLI refuses to run sessions without userns or rootless mode. v2 resolved in Phase 9: user namespaces. Every sandbox pod runs with `hostUsers: false` (since Phase 7; pod root maps to an unprivileged host UID range, checked by `scripts/k8s-prove.sh`), and since Phase 9 the admission policy refuses any sandbox pod without it, whoever creates it. gVisor was rejected: a new runtime in a custom node image, changed exec and cgroup behaviour, and it would not remove the privileged k3d nodes, which are accepted as a known limit (section 13.3, ADR 0011) | Phase 9 (v2) |
| OQ-4 | Argo CD memory use on a 16 GB laptop next to the cluster | Resolved in Phase 8: Argo CD core v3.5.6, since it stays far below the agreed limit of 0.75 GB added (otherwise Flux). Phase 7 measured WSL2 memory in use (`free -m`, without page cache): 0.9 GB without the cluster, 1.7 GB with the bare cluster (1 server, 2 agents), 2.1 GB at peak with Postgres, the API, a session and a selftest with its runner. Argo CD's working set per pod (`scripts/k8s-memory.sh`): idle with no Application 226 MiB (application controller 149, repo server 56, redis 21); peak during a sync 369 MiB; steady with Applications synced 362 to 384 MiB (controller 262 to 270, repo server 71 to 103, redis 19 to 21). WSL2 in use with Argo CD: 2.27 GB before and 2.50 GB at peak during `scripts/k8s-prove.sh` (two sessions, a runner, a second API pod), so Argo CD adds about 0.4 GB | Phase 8 |
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
│   │   ├── kubernetes/
│   │   └── scripted/         stand-in runtime for tests
│   ├── session/              lifecycle, reaper
│   ├── grading/              stars, level rule
│   ├── selector/             next-exercise logic
│   ├── play/                 steps the CLI and the server share
│   ├── dbt/                  runs the dbt project in a container
│   ├── store/                Postgres access, migrations
│   ├── server/               HTTP API for the dashboard
│   ├── testenv/              shared skip and fail rules for integration tests
│   └── i18n/
├── web/                      React + TypeScript (Vite); embed.go embeds dist/ with -tags webdist
├── exercises/<id>/
├── dbt/                      dbt project; embed.go embeds it in the binary
├── deploy/
│   ├── compose/              v1: Postgres (dbt runs with docker run, see 9.2)
│   ├── k8s/                  v2: k3d cluster config, manifests (kustomize) Argo CD syncs, api/Dockerfile
│   ├── k8s-dev/              v2: overlay with the API image built from the working tree
│   └── argocd/               v2: Argo CD install kustomization, the AppProject and the Application
├── locales/                  en.json, no.json
├── scripts/                  validate-exercise.sh, build-web.sh, CI helpers, k8s-*.sh
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
