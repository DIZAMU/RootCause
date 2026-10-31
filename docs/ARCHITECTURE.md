# RootCause architecture

This document describes how RootCause works, from the exercise format to the security model. The reasons behind the main choices are in the [decision records](adr/README.md). Section numbers are stable, because comments in the code refer to them.

The product is called RootCause. The CLI command and the binary are lowercase, `rootcause`, and the Go module path is `github.com/DIZAMU/RootCause`.

RootCause runs in two ways that share the same engine, database and dashboard.

| Mode | Sandboxes | Terminal |
|---|---|---|
| Docker mode | Containers on the local Docker Engine | The learner's own, through `rootcause shell` |
| Kubernetes mode | Pods in a local k3d cluster, deployed by Argo CD | In the browser, on the session page |

`RC_RUNTIME` selects the mode. It is `docker` by default, or `kubernetes`.

## 1. Purpose

### 1.1 Problem

New IT and operations staff often learn troubleshooting on production systems, or not at all. Courses teach commands, but few let you practise fixing a broken machine safely and repeatedly, in both English and Norwegian.

### 1.2 Goals

- A practice tool that hands you a broken Linux machine as a helpdesk ticket and grades the fix.
- One working system that shows containers, Kubernetes, CI/CD, a data pipeline and security design together.
- Free to use for anyone, for any noncommercial purpose.

### 1.3 Non-goals

- Public hosting or several users
- Accounts or login
- Windows or Active Directory exercises
- A mobile layout
- Commercial use

## 2. Key choices

| Area | Choice |
|---|---|
| Engine | Go, for the CLI, the local server, grading and sandbox control |
| Dashboard | React and TypeScript (Vite), built and embedded in the Go binary |
| Database | PostgreSQL, run with Docker Compose in Docker mode |
| Transformations | dbt Core with the Postgres adapter, run in a container |
| Fault and check scripts | Bash |
| Sandboxes | Docker Engine, or Kubernetes with k3d |
| CI/CD | GitHub Actions, with GitOps through Argo CD in Kubernetes mode |
| Interface | A CLI and a local web dashboard |
| Accounts | None, one local profile |
| Languages | English and Norwegian |
| Exercise framing | Helpdesk tickets |
| Scoring | 1 to 3 stars, based on the hints used |
| Time | Recorded, never scored |
| Adaptation | Levels per topic with a 2 up, 2 down rule |
| Variants | Randomized from a seed, in several exercises |
| Commits | Conventional Commits, SSH-signed |

## 3. Concepts

| Term | Meaning |
|---|---|
| Exercise | A folder that defines a healthy machine, a fault, a check, a reference solution and the texts |
| Variant | One concrete version of an exercise, chosen by a seed. The fault type and difficulty stay the same, the details change (file names, ports, users, the ticket's sender) |
| Session | One attempt at one variant in one sandbox, from the start until it is solved, given up or expired |
| Sandbox | The isolated container or pod a session runs in. It is thrown away afterwards |
| Check | The automated grader. It tests the end state, never the commands typed |
| Hint tiers | Hint 1 says where to look, hint 2 which tool to use, and hint 3 nearly gives the answer |
| Walkthrough | The explained solution, shown after a solve or after giving up |
| Topic level | The learner's level in a topic, from 1 to 3 |

## 4. Exercise format

### 4.1 Folder layout

```
exercises/svc-01-intranet-config/
├── exercise.yaml      metadata and runtime settings
├── Dockerfile         builds the base machine (no exercise scripts inside)
├── setup.sh           optional, builds the healthy state at session start
├── break.sh           plants the fault at session start
├── check.sh           grades the end state
├── solution.sh        the reference fix, used only by CI
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
difficulty: 1              # 1 to 3
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
capabilities: [SETUID, SETGID, CHOWN]   # added back after --cap-drop ALL, only from the loader's allowlist
variants:
  site_name: [intranet, wiki, portal]
  port: [8080, 8081, 8088]
```

- `init: systemd` boots systemd as PID 1, for exercises about services at boot. The image runs systemd as its entrypoint, and the sandbox gets in-memory `/run` and `/run/lock` instead of Docker's init, since systemd reaps processes itself. A new sandbox counts as ready only when `systemctl is-system-running` reports `running`. After the check's restart, `degraded` also counts, so the check grades what the learner left.
- `tmpfs` volumes count toward `resources.memory`, and their total must stay below it.
- `runs_from: runner` needs `network: internal` and cannot be combined with `init: systemd`.
- Every sandbox runs within `resources`. The default is 256 MiB and half a CPU, and at most two sandboxes run at once.

### 4.3 Script contracts

**setup.sh** (optional)
- Builds the healthy state from the variant values, for example by rendering a config and starting a service.
- Receives the same environment variables as break.sh. It is copied in, run as root and deleted before break.sh runs.
- It exists so the healthy state is never baked into the image, where the learner could read it or re-run it as a shortcut.

**break.sh**
- Receives `RC_SEED` and every variant value as an `RC_VAR_<NAME>` environment variable.
- Is deterministic, so the same seed always plants the same fault.
- Is copied into the sandbox at session start, run as root and then deleted, leaving no trace of itself.
- May print derived values as one line of JSON, such as `{"line": 14}`. These are stored with the session and fill placeholders in the hints and the walkthrough.

**check.sh**
- Exits 0 for a pass and 1 for a fail. Exit 2 means the check could not run, which is an infrastructure error and never counts against the learner.
- Prints one line of JSON, such as `{"passed": false, "feedback": "site_unreachable"}`. The feedback key maps to a text that describes the symptom, never the fix.
- Accepts every valid fix and rejects shortcuts. Each check has at least one assertion against shortcuts. For example, the config must be valid and the original site content must be served, so replacing the config with a default one fails.
- Is copied in at check time and deleted afterwards, so it is not lying around during the session. While it runs, the learner is root in the same sandbox and could still read it and its environment (section 13.3).
- With `runs_from: runner` it runs in a separate runner container instead. That is a fresh copy of the exercise image on the internal network, with no added capabilities, destroyed after the check. `RC_SANDBOX_HOST` holds the sandbox's host name.

**solution.sh**
- The reference fix. It is never copied into a sandbox image or a session, and only CI uses it.

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
  - "nginx has a built-in config test. Run nginx -t"
  - "Line {line} of the config file has a syntax error."
walkthrough: |
  Explains the cause, how to find it, the fix, and why the fix works.
feedback:
  site_unreachable: "The site still doesn't respond."
```

Placeholders are filled from variant values and derived values. Both language files must define the same keys, and CI enforces it.

## 5. Exercise catalog

| ID | Topic | Ticket | Fault (hidden) | Check passes when | Shortcut that fails |
|---|---|---|---|---|---|
| perm-01-shared-folder | permissions | Økonomi can't save to the shared folder | Group write missing, or the wrong group owns the share | A group member can create and edit a file | Making the share world-writable |
| perm-02-backup-denied | permissions | The nightly backup fails with "permission denied" | The script lacks the execute bit, or the backup user cannot write to the target | The backup runs as the backup user and produces an archive | A world-writable script, or running as root |
| svc-01-intranet-config | services | The intranet went down after a config change | A syntax error in the nginx config | The config test passes and the original page is served | A default page or an empty config |
| svc-02-dies-on-reboot | services | The app works until the server reboots | The service is not set to start at boot | The app runs after a sandbox restart | Starting it by hand, since the check restarts first |
| log-01-crash-on-start | logs | The app crashes on start | A wrong path or a missing value in the app's config, visible in its log | The app runs and its health endpoint returns OK | The health endpoint must report the correct config |
| net-01-no-names | networking | The server reaches IP addresses but not names | Name resolution misconfigured (resolver address or lookup order) | Internal host names resolve to the expected address | Hard-coding the answer in the hosts file |
| net-02-outside-only | networking | The site works on the server but not from outside | The service listens only on the loopback address | A request from outside the sandbox succeeds | The check runs from the runner, not inside |
| disk-01-stopped-saving | disk | The server stopped saving data | The data volume is full because of a large hidden file | Free space is above the threshold and the app writes again | The required data files must still exist |

- Sandboxes have no internet access, so net-01 runs its own small DNS server (dnsmasq) inside the sandbox, with an internal zone.
- disk-01 mounts a size-limited tmpfs as its data volume.
- Every exercise has randomized variants.

## 6. Grading and fairness

| Result | Stars |
|---|---|
| Solved with no hints | 3 |
| Solved with hint 1 or 2 | 2 |
| Solved after hint 3 | 1 |
| Gave up | 0, and the walkthrough is shown |

- Time is recorded for the statistics, never used in scores or levels.
- Checks are unlimited, and a failed check costs nothing.
- A reset gives a fresh copy of the same variant with no penalty. Revealed hints stay revealed.
- Giving up becomes possible once hint 3 is revealed.
- Infrastructure errors (check exit 2, or a sandbox that crashed or was removed) void the session, so it does not count.
- An expired session counts as a fail only if at least one check was run.

## 7. Adaptation

### 7.1 Topic levels

The engine applies these at once.
- Every topic starts at level 1, and levels go from 1 to 3.
- Two solves in a row in a topic, each with at least 2 stars at the current level, move it up.
- Two fails in a row in a topic (given up, or expired after a check) move it down.
- Each change is logged as a `level_changed` event.

### 7.2 Next exercise

The selector reads the dbt marts and applies these rules in order.
1. Reviews come first. An exercise comes back when its latest counted session (solved, given up, or expired after a check) was a fail or a 1-star solve that ended at least 72 hours ago. The oldest due exercise goes first, and a later solve with 2 or more stars takes it out of the queue. It comes back as a new variant, using the first of up to 20 random seeds whose values differ from the last session's.
2. Otherwise the weakest topic is picked. That is the lowest level, with ties broken by the topic practised least recently (never practised first) and then by name.
3. Within the topic, the difficulty closest to the topic level wins, unseen exercises first, then the one attempted least recently.
4. The same exercise never comes twice in a row. This rule wins over rule 1.

### 7.3 Calibration

dbt computes the solve rate, hint use and duration per exercise. Once an exercise has at least 10 sessions, it is flagged when difficulty 1 is solved less than half the time, or difficulty 3 more than 9 times in 10. With one learner the flags are for information only.

## 8. Architecture

### 8.1 Docker mode

```
Linux or WSL2
├── rootcause (Go binary)
│   ├── CLI
│   ├── local server, API and embedded dashboard, 127.0.0.1 only
│   └── Docker runtime (the docker CLI)
└── Docker Engine
    ├── postgres (Compose)
    ├── dbt (a container, run on demand)
    └── sandbox containers, one per session

Browser → http://localhost:7300
```

In Docker mode the terminal is the learner's own. `rootcause shell` opens a shell inside the sandbox.

### 8.2 Kubernetes mode

```
Linux or WSL2
└── k3d cluster, 1 server and 2 agents
    ├── namespace rootcause-system
    │   ├── api and dashboard (Deployment)
    │   ├── postgres (StatefulSet)
    │   └── dbt (a CronJob every 10 minutes, or a Job on demand by script)
    ├── namespace sandboxes
    │   └── one pod per session, the exercise image with nothing added
    └── namespace argocd
        └── Argo CD core, with the application controller, repo server and redis
```

- The cluster is reached through k3d's port mapping to `127.0.0.1:7300` on the host only.
- The browser terminal is xterm.js in the dashboard, on a WebSocket to the API. The API runs `bash` in the session pod through the Kubernetes exec API, the same model as `docker exec`. Nothing in a sandbox listens on the network, so sandboxes stay closed to incoming traffic (ADR 0013).
- `scripts/k8s-up.sh` creates the cluster, builds the exercise images and imports them into the nodes with `k3d image import`, so sandbox pods never pull. It then creates the namespaces and the Secrets, installs Argo CD and applies `deploy/argocd`. Argo CD syncs `deploy/k8s` from `main`, and the API runs the release pinned there by digest (section 14).
- `scripts/k8s-up.sh --dev` builds the API image from the working tree instead and applies `deploy/k8s-dev`, after turning Argo CD's automated sync off. It needs no credentials.
- `scripts/k8s-prove.sh` proves the isolation, and `scripts/k8s-gitops-prove.sh` proves the GitOps setup. The CLI uses the cluster with `RC_RUNTIME=kubernetes`, and `rootcause selftest` then validates exercises in sandbox pods.
- Argo CD is the core install at a pinned version, without the UI, SSO, notifications or the ApplicationSet controller. Its install manifest is checked against a pinned SHA-256 and its images are pinned by digest. Its one Application belongs to the AppProject `rootcause` (section 13.2).
- The whole cluster with Argo CD and two sessions uses about 2.5 GB of memory at peak. `scripts/k8s-memory.sh` prints the current use per node and container.

### 8.3 Runtime abstraction

All sandbox control goes through one interface, so Kubernetes mode swaps the implementation, not the callers.

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

The implementations are `internal/runtime/docker` and `internal/runtime/kubernetes` (client-go). Optional abilities sit beside the interface. These are `Sheller` (`rootcause shell` in Docker mode), `Terminal` (the web terminal in Kubernetes mode), `Diagnoser` and `SystemdChecker`.

The Kubernetes runtime makes each sandbox a pod in `sandboxes`. `Create` returns once the container runs, as `docker run -d` does. `Exec` and `CopyIn` go through the exec API, and `CopyIn` streams the file to `cat`. `Restart` is not supported, because a restarted container starts again from its image. The only exercise that restarts before its check boots systemd, which the runtime refuses first. Pods on the internal network get a hosts entry for every other internal pod, so a check runner reaches its sandbox by name, as on Docker's internal network.

### 8.4 Session lifecycle

1. The selector picks an exercise and a seed.
2. The runtime creates the sandbox from the exercise image, with its resource limits and network mode.
3. setup.sh (if present) and then break.sh are copied in, run with the seed and variant values, and deleted. The values are passed only to these commands, never to the sandbox environment. Derived values are stored with the session.
4. The ticket is shown in the CLI and the dashboard.
5. The learner works. Hints, checks and resets are logged as events.
6. On a check, the sandbox restarts if the exercise asks for it, check.sh runs in the sandbox or from the runner, and the result is logged.
7. On a solve or after giving up, the walkthrough is shown, the stars and the level rule are applied, the sandbox is destroyed and a dbt build starts.
8. A reaper destroys sandboxes that are past their time limit.

## 9. Data

### 9.1 Source tables

The engine writes these.

| Table | Holds |
|---|---|
| `learner` | One row |
| `exercise` | Synced from exercise.yaml on startup (id, version, topic, difficulty) |
| `session` | id, learner, exercise id and version, seed, variant values, derived values, status, sandbox id, started_at, ended_at, expires_at |
| `event` | id, occurred_at, session_id, type, payload (jsonb) |

The event types are `session_started`, `hint_revealed`, `check_run`, `reset`, `gave_up`, `solved`, `expired`, `voided` and `level_changed`.

Migrations are versioned SQL files in `internal/store/migrations`, applied on startup.

### 9.2 dbt models

| Layer | Model | Purpose |
|---|---|---|
| staging | stg_events, stg_sessions, stg_exercises | Typed, cleaned source data |
| intermediate | int_session_outcomes | One row per finished session, with outcome, stars, hints, checks and duration |
| marts | mart_topic_levels | The current level per learner and topic, from level_changed events, and when the topic was last practised |
| marts | mart_review_queue | Review candidates per learner (rule 1 of section 7.2), with the time each is due |
| marts | mart_exercise_stats | Solve rate, hint use, duration and the calibration flag |
| marts | mart_progress | Per learner, topic and day, the sessions, solves, average stars and the level at the end of the day |
| marts | mart_build_info | One row with the time of the latest build. The dashboard shows it, so statistics that lag behind are visibly older |

dbt reads the source tables in the schema `public` and writes every model to the schema `analytics`, so a build never touches the engine's tables. The outcomes in `int_session_outcomes` are `solved`, `gave_up`, `expired_fail` (expired after a check), `expired_skip` (expired with no check) and `voided`. Only the first three count in the statistics and the review queue. The engine stays the authority for levels (section 7.1). `mart_topic_levels` replays its `level_changed` events, and a test asserts that the two agree.

The tests cover unique and not-null keys, accepted values for event types, statuses and outcomes, and the relationships between sessions and events. Singular tests assert one level row per learner and topic, and one review row per learner and exercise.

The project lives in `dbt/` and is embedded in the binary. `internal/dbt` extracts it into a fresh temporary directory for each build, so a build never reads the working tree, and runs `dbt build` in the pinned `dbt-postgres` image with `docker run`. The container joins the Compose network and reaches Postgres by service name. It runs with no capabilities, a read-only root filesystem and the project mounted read-only, with targets and logs in `/tmp`. The connection comes from the engine's database URL, and the password reaches the container only through its environment. dbt runs only against the Compose Postgres, never over the host network. For tests and CI only, `RC_DBT_NETWORK` and `RC_DBT_HOST` name another Postgres container's network and host, such as a CI service container.

Every dbt container is labelled with its database. Before a build, containers left by an earlier build of the same database are removed. A build passes only when dbt exits 0 and runs exactly as many nodes as the project defines, so a run that silently skips models or tests fails. A failed build keeps its full `dbt.log` in the user cache directory and prints the path.

### 9.3 When dbt runs

- In Docker mode dbt runs after each solve and each give up, after the result is shown and with a 120-second limit. It also runs on `rootcause stats --refresh`, and in `rootcause next` when the marts are stale. Expiry by the reaper does not run dbt, and the next `rootcause next` catches up. In the dashboard, a solve or give up returns at once and dbt runs in the background, as does a rebuild when the home view finds the marts stale. Starting the recommended exercise from the dashboard rebuilds stale marts first, like `rootcause next`. A Postgres advisory lock lets only one build run at a time, across the CLI and the dashboard.
- In Kubernetes mode a CronJob runs every 10 minutes (`concurrencyPolicy: Forbid`). The API's service account may not start Jobs (section 13.2), so the dashboard shows when the marts were last built instead. `scripts/k8s-dbt.sh` runs a build at once. An init container writes the project embedded in the binary (`rootcause dbt-project`), so the cluster builds exactly what the binary ships. The API runs with `RC_DBT_MODE=cronjob` and never starts dbt itself.
- The marts are stale when they are missing, or when the learner's latest finished session is newer than the latest one in `int_session_outcomes`. `rootcause next` then rebuilds them once. If they are still stale or dbt fails, the selector falls back to topic levels alone, using rules 2 to 4 of section 7.2 with the engine's levels and the source tables, without the review queue. A failed build never changes a session's result.

## 10. CLI

| Command | Does |
|---|---|
| `rootcause init` | Starts Postgres, runs the migrations and creates the local profile |
| `rootcause next` | Starts the recommended next exercise |
| `rootcause start <id>` | Starts a specific exercise |
| `rootcause shell` | Opens a shell in the active sandbox |
| `rootcause ticket` | Shows the active ticket again |
| `rootcause hint` | Reveals the next hint |
| `rootcause check` | Grades the active session |
| `rootcause reset` | Gives a fresh copy of the same variant |
| `rootcause reset --all [--yes]` | Deletes all of the learner's data, like "Reset all data" in the dashboard. It asks for the confirmation word (RESET, or NULLSTILL in Norwegian) unless `--yes` is given |
| `rootcause giveup` | Ends the session and shows the walkthrough (after hint 3) |
| `rootcause status` | Shows the topic levels, the active session and the review queue |
| `rootcause list` | Lists all exercises with their best stars |
| `rootcause stats [--refresh]` | Shows statistics per exercise from the marts, with calibration flags. `--refresh` rebuilds the marts first |
| `rootcause serve [--port N] [--listen-all]` | Starts the local dashboard on 127.0.0.1, port 7300 by default, until Ctrl-C. It reaps expired sessions every minute while it runs. `--listen-all` is only for the cluster and is refused unless `RC_RUNTIME=kubernetes` |
| `rootcause lang en\|no` | Sets the interface language |

## 11. Dashboard

The dashboard is a calm workbench. It uses flat panels divided by hairlines, nearly square corners, Inter for text and JetBrains Mono for ids, commands and the terminal, and a teal accent. It has a light and a dark theme and follows the system unless switched.

- **Sidebar.** Inbox, Exercises, History, Stats and the session in progress, the five topic level rings, the language and theme switches, and a "?" button for help. It collapses (remembered per browser) to a 64 px rail with only the mark, the expand toggle, the page icons, one language button, one theme button and "?". The width animates only after the learner toggles it, never on load, and changes at once under reduced motion. On a phone it becomes a top bar.
- **Help.** The "?" button or key shows the keyboard shortcuts and the switch for single-key shortcuts, "Show the intro again", and a Data section with "Reset all data".
- **Intro.** A browser's first visit opens a four-step guided intro in a dialog. It covers the inbox and why a ticket is picked, the session, hints and stars, and levels and stats. In Docker mode the session step points to `rootcause shell`, and in Kubernetes mode to the terminal on the page. Back, Next and Skip move through it, Esc skips, and the browser remembers when it is finished or skipped.
- **Reset all data.** A dialog says what goes (history, sessions, topic levels, statistics and settings) and that it cannot be undone. Its button works only once the learner has typed the confirmation word (RESET, or NULLSTILL in Norwegian). The reset ends a session in progress and destroys its sandbox, and deletes the sessions, events, statistics and profile language in one transaction. Then it rebuilds the statistics where the server builds them. The browser's stored preferences are cleared and the app loads fresh, with the intro. The exercise catalog and the migrations stay.
- **Inbox.** The home page shows the tickets as a helpdesk inbox, one row each with title, sender, topic, difficulty, a status tag (next, due, new, tried, or solved with the best stars) and the exercise id. The recommended ticket is pinned first, with why it was picked, the last result and what would change that. A session in progress is pinned above everything.
- **Session start.** The engine's real steps (build the image, start the machine, set up, plant the fault) tick off as they happen, on the row that was clicked. Only the exercise id and the step reach the browser.
- **Session.** The ticket and hints are on the left and the work is on the right. The check bar has Run check, Show the next hint with the stars it leaves, Reset, an optional timer, and Give up behind a menu, with each destructive action confirmed in place. Below it are the last check's result, the terminal (Kubernetes mode) or the `rootcause shell` command (Docker mode), and a check log. A solve plays the app's one expressive animation, under 1.2 seconds, which can be skipped and never takes focus from the terminal.
- **Exercises.** The catalog by topic, with difficulty, best stars and when each was last played.
- **History.** Finished sessions with outcome, stars, hints, checks and time, filtered by topic and outcome.
- **Stats.** The topic rings with progress toward the next level, solves per day as a column chart with a data table, and per exercise the sessions, solve rate, hints, time and calibration flag. Where the server builds the marts itself, there is a "rebuild now" button.
- **Wide screens.** The page column is centred, up to 1440 px (the inbox up to 1280 px), with prose kept to a readable width.
- **Keyboard.** Ctrl+K opens a command palette and Ctrl+Enter runs the check, from the terminal too. `g` then `i`, `e`, `h` or `s` goes to a page, `[` folds the sidebar and `?` opens help. The single-key shortcuts can be switched off and never fire while typing in a field, a dialog or the terminal. The terminal keeps Tab for completion, and Esc then Tab leaves it.
- **Languages.** The EN/NO switch sets the profile language, the same one `rootcause lang` sets.
- **Accessibility.** WCAG 2.2 AA is the floor. Every task works by keyboard alone, reduced motion is respected, and layouts reflow at 320 px and at 200% zoom.

How it is built.

- The app (Vite, React, TypeScript, plain CSS, hash routing) lives in `web/`. `scripts/build-web.sh` builds it into `web/dist` with `npm ci` from the committed lockfile, and `go build -tags webdist` embeds it. Without the tag, `/` serves a translated page that explains how to build it.
- `internal/server` is a JSON API over the same engine, selector and store as the CLI. Both front ends call the shared steps in `internal/play`, such as the next pick and seed, the exercise texts and the statistics rebuild. Exercise texts are filled in on the server in the learner's language. UI strings come from `locales/*.json` through `/api/i18n/{lang}`, so there is one copy of every string.
- The read endpoints are `GET /api/overview`, `/api/exercises`, `/api/history`, `/api/stats`, `/api/session` (204 if there is none), `/api/sessions/{id}` and `/api/i18n/{lang}`. The actions are `POST /api/start` (`{"exercise": ""}` starts the recommended one), `/api/hint`, `/api/check`, `/api/reset`, `/api/giveup`, `/api/stats/rebuild` and `/api/data/reset`, plus `PUT /api/lang` and `GET /api/terminal` (a WebSocket, in Kubernetes mode). `/api/stats/rebuild` runs one rebuild at a time and answers 409 while one runs, or 404 where a CronJob builds the marts. `/api/data/reset` needs `{"confirm": "RESET"}` and answers 409 while another action, such as a session start, runs. Actions name the session the page shows and are refused if it is no longer the active one.
- Before a session exists, the inbox and the catalog fill each ticket's header with the first value of each variant. A session shows its own variant.
- The dashboard follows the CLI by polling, the open session every 2 seconds and the overview every few seconds and on focus. The two front ends share only the database.
- The session view never contains the seed, the variant values or the derived values. Hints appear once revealed, and the walkthrough once the session is solved or given up.
- Everything the dashboard needs is in the binary, including its fonts (SIL OFL, with the licences next to the files) and its icons. It loads nothing from elsewhere. The terminal's code loads only on a session page.
- An error the dashboard has no text for shows a short translated message, and its technical detail goes to the terminal that runs `rootcause serve`, never to the browser. Static files are served without folder listings.
- `scripts/ui-check.sh` checks the dashboard in a real browser. It covers the first-run intro, the collapsed rail, centred pages on wide screens, the reset dialog, axe-core accessibility rules, layout shift, the keyboard rules, the Tab order, terminal latency, the CSP, and screenshots in both languages. It is not part of CI.

## 12. Languages

- UI strings live in `locales/en.json` and `locales/no.json`. No user-facing text is hard-coded in Go or React.
- Exercise texts live in `exercises/<id>/i18n/{en,no}.yaml`.
- CI fails if a key exists in one language and not the other.
- Code, comments, commit messages and docs are in English.
- Counts use a `_one` and `_other` pair of keys, the two plural forms English and Norwegian both have.
- Interface text is plain and warm. Buttons start with a verb, praise is only for what was done, and there is at most one exclamation mark (on a solve without hints). There are no streaks, no guilt and no urgency. Layouts leave room for Norwegian, which runs longer.

## 13. Security model

### 13.1 Docker mode

- Sandboxes never get the Docker socket, the host network or privileged mode.
- `--cap-drop ALL`, then only the capabilities an exercise needs.
- Memory, CPU and process limits on every sandbox.
- Network `none`, or an internal network with no route out.
- The learner is root inside the sandbox, so container root must not map to host root. Docker must use user namespace remapping or rootless mode (ADR 0010), and the CLI refuses to start a session otherwise.
- The local server binds to 127.0.0.1 only.
- It refuses any request whose Host header is not `127.0.0.1:PORT` or `localhost:PORT`, which blocks DNS rebinding.
- Every `/api` request must carry the header `X-RootCause-Token`, a random token made per server process and written only into the served `index.html`. Another website cannot read that page, so it cannot call the API. The server sends no CORS headers, and every response forbids framing.
- The web terminal's WebSocket cannot carry headers, so the browser offers the token as a subprotocol (`rootcause.token.<token>`) and the server answers with `rootcause.terminal` only. The request's `Origin` must be the dashboard itself, and the Host check applies as above.
- The Content-Security-Policy allows scripts from the dashboard only. Styles may be inline, because xterm.js writes its layout into `<style>` elements.

### 13.2 Kubernetes mode adds

- The API's ServiceAccount may only create, read, exec into and delete pods in `sandboxes`, and nothing else. It is a Role in `sandboxes`, not a ClusterRole, with pods `create`, `get`, `list` and `delete`, `pods/exec` `create` and `get` (exec over WebSocket needs both) and `pods/log` `get`. `scripts/k8s-prove.sh` checks a `kubectl auth can-i` matrix of allowed and denied actions.
- `sandboxes` has a default-deny NetworkPolicy, a ResourceQuota (4 pods, 1 GiB, 2 CPUs) and a LimitRange. The one exception to the default deny is that a check runner may connect to sandboxes on the internal network, and those accept connections from runners only.
- `rootcause-system` is default-deny too. The API accepts 7300 and reaches Postgres, cluster DNS and the Kubernetes API server's address. dbt reaches Postgres and DNS, and Postgres accepts the API and dbt.
- Session pods run in their own user namespace (`hostUsers: false`, so pod root is not node root), with no service account token, no service links, seccomp `RuntimeDefault` and no hostPath volumes. All capabilities are dropped and the exercise's allowlist is added back, privilege escalation is off, memory and CPU are limited, `tmpfs` becomes an in-memory `emptyDir`, and there is no cluster DNS. The pause container is PID 1 and reaps stopped processes. The process limit is the kubelet's `pod-max-pids` (256), since Kubernetes has no per-pod setting.
- Pod Security Admission enforces `restricted` in `rootcause-system` and `baseline` in `sandboxes`. It stays on next to the admission policy, because it also covers procMount, sysctls, AppArmor, SELinux options and host ports.
- A ValidatingAdmissionPolicy (`deploy/k8s/admission.yaml`) refuses every pod in `sandboxes` that is not built the way the runtime builds it. It requires `hostUsers: false`, no host network, PID or IPC, only `emptyDir` volumes and no hostPath, no privileged container, `allowPrivilegeEscalation: false`, drop ALL with only the exercise allowlist added, seccomp `RuntimeDefault`, no service account token, `imagePullPolicy: Never` and only `rootcause/` images. It applies to containers, init containers and ephemeral containers, with failurePolicy Fail and a Deny binding with no match conditions, so no requester is exempt, neither the API nor Argo CD. Argo CD manages the policy (the AppProject allows exactly these two cluster-wide kinds), so self-heal restores it. `TestSandboxAdmission`, run by `scripts/k8s-prove.sh`, admits every exercise's real pod and checks each forbidden change as both service accounts, and `ci.yml` checks the rendered policy's rules and binding.
- Secrets never enter the repository. Local secrets live in git-ignored `.env` files. Cluster secrets are created outside git by `scripts/k8s-up.sh` from files, so no secret appears in a manifest, on a command line or in any output. They are `rootcause-db` (the Postgres password, generated once into git-ignored `deploy/k8s/.env`), and in the GitOps setup `ghcr-pull` (a GHCR token) and `repo-rootcause` in `argocd` (a read-only deploy key). The token and the key live in `~/.config/rootcause` with mode 600.
- What is not in git is created by `scripts/k8s-up.sh` and never tracked by Argo CD, so prune leaves it alone. That covers the namespaces, the Secrets and the NetworkPolicy to the API server's address, which is known only once the cluster runs.
- The AppProject `rootcause` limits what a bad commit on `main` can make Argo CD do. This repository is the only source, `rootcause-system` and `sandboxes` on this cluster are the only destinations, and only the namespaced kinds `deploy/k8s` uses are allowed. The core install has no `default` project. `scripts/k8s-gitops-prove.sh` checks that an Application pointing at another namespace or another repository is refused.
- Argo CD's application controller can write anywhere in the cluster. The AppProject limits what it is asked to do, not what it can do, so it is a trust boundary next to the API (ADR 0016).

### 13.3 Known limits

**In Docker mode**
- Checks run inside the sandbox, where the learner is root. A determined learner could replace a tool the check uses, such as `curl` or `nginx`, and fake a pass. This is acceptable for a single-user practice tool, and exercises where it matters use `runs_from: runner`.
- `check.before: restart` restarts the container. Files stay, but only PID 1 starts again, so processes setup.sh started are gone and setup.sh does not run again. An exercise that needs services started at boot uses `init: systemd`.
- In `init: systemd` sandboxes, journald and tmp.mount are masked, because they need capabilities outside the allowlist. Service output is not kept in a journal.
- `init: systemd` exercises need a host with cgroup v2, which is the WSL2 default. The systemd in the exercise images does not boot on cgroup v1, so the CLI and the selftest refuse such an exercise there with a clear error.
- Sandboxes on the internal network can reach each other. With one learner and at most two sandboxes, this is accepted.
- check.sh and the seed and variant values it receives are copied in only for the check, but while the check runs, the learner is root in the same sandbox and could read them. That only spoils the learner's own practice, and the scripts are public in this repository anyway.
- A sandbox's writable filesystem has no size limit, so a learner (or a runaway command) could fill the host's disk. Docker can only limit it on XFS with project quotas. The tmpfs volumes an exercise declares are limited.

**In Kubernetes mode**
- k3d runs its node containers privileged and in the host's user namespace, so node root is host root, whatever Docker's userns-remap says. This is accepted for a local, single-user cluster (ADR 0011). Sandbox pods run in their own user namespaces, enforced by the admission policy for every requester, and system pods run as non-root users in their own user namespaces as well. Rootless k3d would fix this, but it needs sudo and k3d's support for it is still experimental.
- `deploy/k8s/k3d.yaml` binds the Kubernetes API to 127.0.0.1, and `scripts/k8s-prove.sh` fails if any published port is bound elsewhere.
- The API image's `exercises/` and the exercise images imported by `scripts/k8s-up.sh` come from different trees, the release and the working tree. A changed exercise bumps its `version`, and so its image tag, so a missing image fails the session start with `ErrImageNeverPull` instead of running a stale image.
- A check runner may connect to any sandbox on the internal network, not only its own. Per-session NetworkPolicies would need the API to manage NetworkPolicies, which section 13.2 does not allow.
- `init: systemd` exercises (svc-02) do not run on Kubernetes. In a pod with its own user namespace the cgroup filesystem stays owned by host root, so systemd cannot boot. The CLI, the dashboard and the selftest refuse such an exercise there with a clear error, and there is no restart before a check either (section 8.3).
- The cluster's dbt CronJob runs `dbt build` and relies on dbt's own exit code. The node count check of section 9.2 applies to builds the binary starts.

## 14. CI/CD

| Workflow | Runs on | Does |
|---|---|---|
| `ci.yml` | Every pull request and push to main | gofmt, go vet, golangci-lint, govulncheck, go test, shellcheck on all scripts, the frontend type check and build, and i18n key parity. Postgres comes from `rootcause init`, and any skipped test fails the run |
| `exercises.yml` | Changes under `exercises/` | For each changed exercise and 3 seeds, it builds the image, checks that break then check fails, and that break, solution, check passes |
| `dbt.yml` | Changes under `dbt/` | A dbt build against a Postgres service container, seeded with a scripted series of sessions played through the engine |
| `commit-policy.yml` | Every pull request | Rejects Co-authored-by trailers, tool attribution lines, author emails other than the author's, and unsigned commits |
| `release.yml` | Tags `vX.Y.Z` on main | Checks that the tag is `vX.Y.Z`, on `main` and new, validates every exercise (it calls `exercises.yml`), then builds the API image and pushes it to `ghcr.io/dizamu/rootcause-api` under that tag only. The job summary shows the digest. It never commits. The author pins the digest in `deploy/k8s/kustomization.yaml` in a pull request, `ci.yml` checks it, and Argo CD syncs it once merged |

- Only changed exercises are validated per pull request, and all of them on tags.
- `exercises.yml` and `dbt.yml` start on every pull request and push to main, and decide inside the job whether anything changed, so they can be required checks.
- Every action is pinned by commit SHA. Workflows have no permissions by default, and each job gets read access to the repository only.
- Only the API image goes to GHCR, and it is public (ADR 0020). Exercise images stay local and are imported with `k3d image import`.
- `packages: write` is granted only to `release.yml`'s push job, and `packages: read` only to its tag check and to `ci.yml`'s `ci-release-image` job. GITHUB_TOKEN is the only credential.
- Images target linux/amd64 only.
- The scripts only CI uses live in `.github/scripts`.

## 15. Git and authorship

- Commits are authored by DIZAMU with the GitHub noreply address.
- Every commit is made by the author after reviewing the diff.
- Commits follow Conventional Commits (`feat:`, `fix:`, `docs:`, `test:`, `refactor:`, `ci:`, `chore:`).
- There are no `Co-authored-by` trailers and no tool attribution lines in commits, pull requests, code comments or docs.
- Commits are signed with an SSH key (`gpg.format ssh`), so GitHub shows them as Verified.
- `.githooks/commit-msg` (enabled with `git config core.hooksPath .githooks`) rejects messages that break these rules, and `commit-policy.yml` checks again in CI.
- Work happens on short-lived branches, merged to `main` through a pull request so CI runs first.

## 16. Repository layout

```
RootCause/
├── cmd/rootcause/            CLI entry point
├── internal/
│   ├── cli/
│   ├── exercise/             loader and schema validation
│   ├── runtime/
│   │   ├── docker/
│   │   ├── kubernetes/
│   │   └── scripted/         stand-in runtime for tests
│   ├── session/              lifecycle and reaper
│   ├── grading/              stars and the level rule
│   ├── selector/             next-exercise logic
│   ├── play/                 steps the CLI and the server share
│   ├── dbt/                  runs the dbt project in a container
│   ├── store/                Postgres access and migrations
│   ├── server/               HTTP API for the dashboard
│   ├── testenv/              shared skip and fail rules for integration tests
│   └── i18n/
├── web/                      React and TypeScript (Vite), embedded with -tags webdist
├── exercises/<id>/
├── dbt/                      the dbt project, embedded in the binary
├── deploy/
│   ├── compose/              Postgres for Docker mode
│   ├── k8s/                  k3d config and the manifests Argo CD syncs
│   ├── k8s-dev/              overlay with the API image built from the working tree
│   └── argocd/               Argo CD install, the AppProject and the Application
├── locales/                  en.json, no.json
├── scripts/                  setup, build and cluster scripts
├── .github/                  workflows and the scripts only CI uses
├── .githooks/                commit-msg
└── docs/                     this document and the decision records
```
