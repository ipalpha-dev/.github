# IPAlpha local development (reference)

New machine:

```sh
curl -fsSL https://raw.githubusercontent.com/ipalpha-dev/.github/master/install.sh | sh     # macOS / Linux / WSL
irm https://raw.githubusercontent.com/ipalpha-dev/.github/master/install.ps1 | iex          # Windows PowerShell
cd IpAlpha
./run            # .\run on Windows
```

Everything below is done by one program, `ipalpha` (Go, no runtime dependencies). The workspace keeps a
copy in `.ipalpha/bin/` and small wrappers at its root (`run`, `pull`, … on macOS/Linux; `run.cmd`, … on
Windows). Something wrong? `./doctor` lists what is missing with the fix for each item.

## Supported platforms

| Platform | Notes |
| --- | --- |
| macOS (Apple Silicon, Intel) | Docker Desktop (preferred) or Apple `container` (macOS 26+) |
| Linux (x86-64, arm64) | Docker Engine + Compose v2. Not in the `docker` group? `./doctor` says how. |
| Windows 10/11 (x64, arm64) | Native: Docker Desktop + Git for Windows (its Git Bash runs the npm scripts). Windows Terminal recommended. Keep the workspace on a short path (e.g. `C:\IpAlpha`). |
| WSL2 | Works like Linux; enable *WSL integration* for the distro in Docker Desktop. |

## Setup

`ipalpha setup [folder]` (what the install one-liners run) — safe to run again on an existing workspace:

1. **Folder** (default `./IpAlpha`). The container prefix comes from its name (`IpAlpha` → `ipalpha`,
   `ipalpha-2` → `ipalpha-2`), so two workspaces never share a database.
2. **Tools**: Git, Node.js ≥ 20, Docker (+ Compose v2); optional GitHub CLI and kubectl. A missing tool can
   be installed after confirmation with winget (Windows), Homebrew (macOS) or apt/dnf/pacman (Linux).
3. **GitHub**: an SSH key works; without one the GitHub CLI signs in through the browser (`gh auth login`).
4. **Repositories** cloned in parallel into `core/`, `apps/<app>/` and `deployment/`. Optional app repos
   (Acampa Kids) are skipped quietly when your account cannot read them.
5. **`.env` per API** from its `.env.example` (or the embedded fallback in `internal/assets/files/env-fallback`
   until the repo has one). Re-runs and `./pull` only **add missing keys** and fill blanks; local values are
   never overwritten — except notifications, which always go to Mailpit locally.
6. **First sign-in**: your name and mobile number become the local superuser. Codes arrive in Mailpit.
7. **AI for `./publish`**: any installed coding CLI, any model, or your own command (see below).
8. **Files**: `.ipalpha/` (settings, compose file, binary, logs) and the wrappers.
9. **npm dependencies** (failures do not stop setup; `./run` retries).

Non-interactive (CI): `ipalpha setup <folder> --yes` with `IPALPHA_SUPERUSER_PHONE`, `IPALPHA_AI_CLI=none`.

## `./run`

1. Starts Docker Desktop / Apple container when it is installed but off (up to 2 minutes).
2. **Ports**: each service keeps its preferred port (`.ipalpha/settings`) unless something else holds it;
   then it moves to the next free port **for this run only** and every URL handed to the processes follows
   (`.env` files keep the defaults). The panel says which moved and who held the port. The infra keeps a
   moved port across runs while its container lives. Two workspaces can run at the same time.
3. Infra containers (MongoDB, Redis, RabbitMQ + UI, Mailpit) up and healthy (`docker compose --wait`).
4. `.env` completion, npm installs (shared-js/shared-ui built first, consumers' copies refreshed).
5. auth-api signing keys, then the **local** auth database is reconciled: drifted client secrets of the local
   services and the `localhost` origins/callbacks of the built-in apps (so sign-in keeps working when a web
   port moved). Only loopback MongoDB; non-loopback entries are never touched.
6. Every API and the remembered web apps start at once — nothing waits on a peer. Each API's `GET /ready`
   drives its dot: ● ready, ◐ starting/not ready, ✖ failed, ○ stopped.

Processes left behind by a closed terminal are stopped by the next `./run`.

**Panel keys** (`?` shows them): `s` start/stop · `r` restart · `x` stop and forget · `o` open the page ·
`p` move to a free port · `K` stop whoever holds the port · `e` copy an error report (for a teammate or an
AI) · `y` copy the log · `L` log file path · `c` clear · `f` follow · `m` mouse text selection · `q` quit
(the infra keeps running; `./stop` stops it).

When a process stops, a **crash card** above its log explains the likely cause (port in use, missing
module, missing env var, MongoDB auth, TypeScript error, connection refused) and what to press or run.

Flags: `--plain` (prefixed logs, no panel; automatic without a terminal), `--only a,b`, `--no-browser`,
`--skip-deps`, `--until-ready 5m` (exit 0 once every service is ready — CI and scripts).

Logs: `.ipalpha/logs/run/<service>.log` (one per process, rewritten on each start) and
`.ipalpha/logs/<command>-<time>.log` (every command, 20 kept).

### Browser pages

Remembered pages start (web apps) and open once their server answers. Default: Auth Web (starts, never gets
a tab — it is the sign-in popup), Oikos, Mailpit. Starting/opening a page in the panel remembers it, `x`
forgets it. Outside the panel: `./ipalpha apps` (menu), `./run apps set oikos-webapp mailpit`,
`./run apps defaults`. `IPALPHA_OPEN_BROWSERS=0` or `--no-browser` skips opening once.

### Local notifications

notifications-api always uses Mailpit locally (`MAIL_PROVIDER=mailpit`, `SMS_PROVIDER=mailpit`), even when
vendor keys exist; SMS appear as `[SMS]` e-mails. Mailpit binds to loopback, keeps ≤ 200 messages for 24 h,
no persistent volume. Use synthetic recipients only.

## `./publish`

1. Changed repositories (`core/`, `apps/`): uncommitted changes, or commits since the last `v*` tag not yet
   bumped. Several → a checklist (all selected).
2. Fetch every touched repo; fast-forward when applying; diverged history stops with the fix.
3. **AI decision** per repo (semver bump + one-line conventional commit message), cached by diff. Docs-only
   changes never bump. When the AI fails, the engine's own error is shown and you choose the bump and type
   the message.
4. Confirm → `package.json`/lock version, commit, push; libraries are tagged `v<version>` here, image repos by
   CI when they reach production. libraries → npm by TeamCity (the pushed `v<version>` tag triggers "<lib> — Publish to npm"; no npm login on your machine); images are built
   and pushed (Docker or Apple container) and `deployment` image tags bumped.

`-d` shows the plan and offers to apply exactly that plan · `-f <repo>` one repo · `--engine/--model` one-off
AI · `-y` no confirmation · `./publish clean` wipes the decision cache. Also `--resume`, `--npm-only`,
`--initialize`, `--deployment-path`, `--ci` (TeamCity builds images).

### AI configuration

`./ipalpha ai` (menu, tests the choice), `./ipalpha ai set <cli> [model]`, `./ipalpha ai test`.

| Preset | Command used | Model examples |
| --- | --- | --- |
| `pi` | `pi -p --no-session [--model M]` (prompt on stdin) | `anthropic/claude-sonnet-4-5` |
| `claude` | `claude -p [--model M]` | `sonnet`, `opus` |
| `codex` | `codex exec --skip-git-repo-check [--model M]` | `gpt-5` |
| `gemini` | `gemini -p " " [--model M]` | `gemini-2.5-pro` |
| `opencode` | `opencode run [--model M]` | `anthropic/claude-sonnet-4-5`, `ollama/qwen3` |
| `copilot` | `copilot -p <prompt> [--model M]` | `claude-sonnet-4.5` |
| `grok` | `grok -p <prompt> [--model M]` | `grok-4` |
| `ollama` | `ollama run <model>` (local, free) | `qwen3:8b` |
| `custom` | any command; prompt on stdin, or `{prompt}`, `{prompt_file}`, `{model}` | `llm -m {model}` |
| `none` | you type bump and message | |

The choice is per workspace (`ai_cli`, `ai_model`, `ai_command` in `.ipalpha/settings`).

## `./pull`

Downloads the newest tool (checksum verified) and continues with it; fast-forwards every repo (a conflict
leaves that folder untouched and is listed at the end), clones missing ones, moves repos whose folder
changed (e.g. `core/forms-*` → `apps/forms/`, feature worktrees included), adds new `.env` keys, refreshes
wrappers and generated files. Feature workspaces only fetch. Workspaces made by the old bash tooling are
migrated by their next `./pull`.

## Settings

`./ipalpha config` (menu) or `config get` / `config set <key> <value>`:

| Key | Meaning |
| --- | --- |
| `lang` | `pt-BR`, `en-US`, `es`, `fr`, `de`; empty follows the computer |
| `ai_cli`, `ai_model`, `ai_command` | publish AI |
| `browser_apps` | pages/web apps of `./run` |
| `runtime` | `docker`, `container` (Apple) or empty (auto) |
| `<service>_port` | preferred port (moves automatically when busy) |
| `infra_name` | container/volume prefix |

## Ports (defaults)

| Service | Port | Service | Port |
| --- | --- | --- | --- |
| projects-api | 3001 | auth-webapp | 5100 |
| persons-api | 3002 | forms-webapp | 5106 |
| organizations-api | 3003 | oikos-webapp | 5110 |
| notifications-api | 3004 | developers-webapp | 5111 |
| auth-api | 3005 | MongoDB | 27017 |
| forms-api | 3006 | Redis | 6379 |
| dispatch-api | 3007 | RabbitMQ | 5672 |
| ai-api | 3008 | RabbitMQ UI | 15672 |
| developers-api | 3009 | Mailpit | 8025 |
| places-api | 3011 | | |

## Core and apps

`core/` holds the shared capabilities and the core UIs; apps that consume core live in `apps/<app>/`
(`apps/forms/{forms-api,forms-webapp}`). **Acampa Kids** (`apps/acampa-kids/{backend,frontend,face-service}`)
has its own repositories, registry and TeamCity project; `./run` does not start it and its repos are optional.
In a feature environment every core service runs; an app joins only when one of its own repos changed.

## Feature environments: `./feature`

One feature = one isolated workspace, one `feat/<slug>` branch per repo, one preview namespace with its own
data and public HTTPS hosts, alive 72 h after each publish. 

```sh
./feature new hello-preview          # features/hello-preview/: worktrees on feat/hello-preview at the last green Core Deploy
cd features/hello-preview
./run                                # own ports (+100 per feature) and own infra
./publish                            # = --feature hello-preview: commit (no version bump), push, deploy the preview
cd ../..
./feature list                       # previews + local features, generation, expiry, URLs
./feature extend hello-preview       # +72 h without a build
./feature rebase hello-preview       # move to the latest green Core Deploy
./feature reset hello-preview        # back to the synthetic seed (type the slug to confirm)
./feature destroy hello-preview      # delete the preview and worktrees; branches stay
```

`--baseline <deployment commit>` pins an earlier recorded baseline. Slugs: `^[a-z0-9-]{3,30}$`, no hyphen at
either end, every host ≤ 63 chars.

| URL | What |
| --- | --- |
| `https://ipalpha-<slug>.kevyn.com.br` | Oikos |
| `https://forms-ipalpha-<slug>.kevyn.com.br` | IPAlpha Formulários |
| `https://auth-ipalpha-<slug>.kevyn.com.br` | sign-in popup |
| `https://developers-ipalpha-<slug>.kevyn.com.br` | IPAlpha Developers (when the baseline has it) |
| `https://ipalpha-<slug>.kevyn.com.br/mailbox` | captured e-mail/SMS (sign-in codes) |
| `https://acampa-ipalpha-<slug>.kevyn.com.br` | Acampa Kids — only when an `acampa-kids-*` repo changed |

How it works: main checkouts are never touched (`git worktree`); unchanged services run the baseline images
(`deployment/releases/core-latest.json`); `shared-js` changes rebuild every core API, `shared-ui` every core
web app. `./publish` pushes `previews/<slug>/release.json` to deployment master (a throwaway worktree, retried
on races, CI fields kept) and waits for CI's record commit — no CI token on laptops. Pushes use
`--force-with-lease` against the tip this workspace pushed, so a teammate's push is never overwritten.
`.env` files are kept out of commits. Acampa repos start at the `v<version>` tag their production manifest
names (origin/master with a warning when missing). Sign in with the fixture accounts in
`deployment/fixtures/2/README.md`. Previews only hold synthetic data (LGPD).

## Troubleshooting

Start with `./doctor` (`--ai` also tests the AI, `--copy` copies the report).

- **Docker not running**: `./run` starts it; if it cannot, open Docker Desktop (Windows/macOS) or
  `sudo systemctl start docker` (Linux). WSL: enable WSL integration.
- **A service crashed**: read its crash card; `e` copies a report you can paste to a teammate or an AI.
- **Port problems**: nothing to do — busy ports move. `p` moves a running service, `K` frees a port held by
  a leftover process.
- **MongoDB auth failures after changing `.ipalpha/.env`**: `./stop --volumes` recreates the local data.
- **Windows `EPERM`/`EBUSY` during npm install**: close editors/terminals using the folder and run again.
- **AI fails in `./publish`**: the box shows the engine's message; type the version by hand or
  `./ipalpha ai` to switch.

## FAQ

**Why are the APIs not in containers?** Node on the host keeps edit → restart fast and debuggable; only the
infra runs in containers.

**Where does the tooling live?** In the org's `.github` repository (Go). It is never cloned into a workspace:
the binary is downloaded and updates itself on `./pull`.
