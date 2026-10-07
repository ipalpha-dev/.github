# IPAlpha local development (reference)

Primary path for a new machine:

```sh
bash <(curl -fsSL https://raw.githubusercontent.com/ipalpha-dev/.github/master/bootstrap.sh)
cd IpAlpha
./run
```

Long-form reference for tools, ports, degraded integrations, and troubleshooting.

## Supported platforms

| Platform | Notes |
| --- | --- |
| macOS | Preferred — Apple `container` runtime or Docker Desktop |
| Debian/Ubuntu Linux | Docker Engine + Compose v2 |
| Windows | **Ubuntu WSL2 only** |

## What `setup` does

1. Language (pt-BR default, en-US), workspace root (default: `./IpAlpha` in the
   folder where you pasted the command).
2. Checks/installs tools: git, Node.js LTS ≥ 20 + npm, `gh`, `kubectl`, and a
   container runtime — **Apple `container` preferred, Docker fallback**
   (`runtime=` in `.ipalpha/settings`). The process panel uses an embedded
   background runner; logs land under `$TMPDIR/ipalpha-run-logs`. To use
   `mprocs` instead, install it manually and run with `IPALPHA_RUNNER=mprocs`.
3. Clones the org repos into the layout: `core/shared-js`,
   `core/{auth-api,persons-api,organizations-api,projects-api,notifications-api}`,
   `deployment`. Existing folders are kept.
4. Writes `.env` per microservice from each repo's `.env.example`; until those
   exist, the fallback templates in `templates/env-fallback/` are used (secrets
   always blank — never real secrets in this repo). Re-runs and `./pull` only
   **add missing keys**, never overwrite local values.
5. Resolves ports: if an infra or API port is busy, picks a free one, rewrites
   the `.env` files, and records everything in `.ipalpha/settings`.
6. Runs `npm install` in every repo that has a `package.json`.
7. Writes `.ipalpha/` (settings, compose, ports, mprocs config for opt-in, `lib/`,
   helper scripts) and the `./run`, `./publish`, `./pull` wrappers at the workspace root.

The `.github` clone is **temporary**: setup deletes it when done (`--keep-setup`
keeps it). `./pull` re-downloads `.github` into a temp folder to refresh
`.ipalpha/lib`, templates and `set-keys`, then deletes it.

## Repos (org `ipalpha-dev`)

| Repo | Kind |
| --- | --- |
| `.github` | org profile page + this tooling (not cloned into the workspace) |
| `shared-js` | npm library `@ipalpha/shared-js` on npmjs.com (helpers only) |
| `shared-ui` | React component library `@ipalpha/shared-ui`, consumed by every webapp via `file:../shared-ui`; built by `install-deps` before them (not published) |
| `auth-api`, `persons-api`, `organizations-api`, `projects-api`, `notifications-api`, `forms-api`, `dispatch-api`, `ai-api`, `developers-api` | NestJS + TypeScript backends, **no frontend of their own** |
| `mordomia-webapp` | standalone Vite app (port 5110): Mordomia, the one UI for superuser + stewards over every core API (persons, projects, org chart, notifications, access, my data), live through dispatch-api |
| `auth-webapp` | standalone Vite app (port 5100): the sign-in popup (account chooser + consent) |
| `forms-webapp` | standalone Vite app (port 5106): IPAlpha Formulários |
| `developers-webapp` | standalone Vite app (port 5111): IPAlpha Developers, the public developer portal (docs, app directory, requests); its /api proxy reaches developers-api, auth-api, projects-api and dispatch-api |

Every webapp is served at `/` and reaches the APIs same-origin at `/api/<name>` (`/api/auth`,
`/api/projects`, `/api/persons`, `/api/organizations`, `/api/notifications`, `/api/forms`, `/api/ai`,
`/api/developers`;
sockets at `/api/dispatch/socket.io`): the Vite proxy locally, the ingress in production. No CORS.
| `deployment` | k8s manifests under `core/<ms>/`, namespace `ipalpha-core` |

The org name lives in one place (`ipalpha_org` in `lib/common.sh`); image names
follow `ghcr.io/<org>/<ms>`.

## Day-to-day: `./run`

1. Starts infrastructure in containers and waits for health:
   MongoDB, Redis, RabbitMQ (+ management UI on 15672), and Mailpit. Apple `container` runs
   the services directly (network and named volumes prefixed by `infra_name`);
   Docker uses `docker compose --wait`. Each workspace has its own infra: setup derives
   `infra_name` in `.ipalpha/settings` from the folder (`IpAlpha` → `ipalpha`, `ipalpha-2` →
   `ipalpha-2`), so a second workspace never reuses another's database or client secrets.
   They share ports, so run one workspace at a time.
2. Installs missing npm dependencies in parallel: shared-js/shared-ui are installed and built
   first (consumers copy them), then every other project. A failure prints npm's last lines and
   the full log path under `.ipalpha/.state/install-deps/`.
3. Starts every API and the remembered web apps at once — nothing waits for a peer. Each core MS
   exposes `GET /live` (process up) and `GET /ready` (200 only when Mongo, Redis,
   RabbitMQ and the projects cache are all good; 503 `{ready:false, checks}` otherwise).
   The panel polls `/ready` and shows ● ready / ◐ up-but-not-ready per row; k8s uses the
   same two paths as liveness/readiness probes.
   Processes run in the background with logs under `$TMPDIR/ipalpha-run-logs`.

Stop: Ctrl+C, then `.ipalpha/bin/infra-down`.

Runner override: `runner=` in `.ipalpha/settings` or `IPALPHA_RUNNER=` —
`background` (default, embedded), `mprocs` (opt-in; needs `mprocs` installed).

### Browser pages

`./run` opens the remembered local pages once their servers respond. By default Auth Webapp,
Mordomia and Mailpit start and open; start any other web app with `s` and it is remembered. Auth Webapp starts but never opens its own tab: other
apps open it as the sign-in popup (`o` still opens it manually). The same selection controls which standalone
web apps start and which pages open. It is remembered in `browser_apps` in `.ipalpha/settings`, including
an empty selection. Setup and `./pull` preserve it; URLs always use current ports.

In the process panel, starting/restarting an app (`s`/`r`) remembers it; explicitly
stopping an app (`s`/`x`) removes it. Next `./run` starts the remembered apps and
automatically opens their pages after they respond. Quitting the whole runner
does not clear your choices; a startup failure does not clear them either.
Press `o` to open a page and remember it. Mailpit has its
own browser-only row: it is started by infrastructure, not as a second process.
The launcher cannot detect which tabs you later close in your browser; use `x`
to stop reopening a page.

APIs and shared infrastructure keep starting normally. Mailpit's selection controls
only its inbox tab, not the capture container needed by notifications-api.
Background and mprocs reuse the saved choices. Manage them without starting services:

```sh
./run apps                             # list remembered apps/pages (browsers is an alias)
./run apps set mordomia-webapp mailpit auth-webapp
./run apps set                         # start no web apps and open no pages
./run apps defaults                    # restore Auth Webapp, Mordomia and Mailpit
IPALPHA_OPEN_BROWSERS=0 ./run           # skip opening this time, keep preferences
```

Only loopback HTTP URLs from the generated workspace manifest are opened. Failed
or slow servers do not delay boot; automatic opening retries for at most two
minutes and exits if the runner exits. CI never opens browsers. The helper uses
the default system browser (`open` on macOS, `xdg-open` on Linux, `wslview` in WSL)
and records opener errors in `.ipalpha/.state/browser.log`. No application database,
member data, extra container, or paid service is involved.

## Day-to-day: `./publish`

Dev-only release (no prod rollout):

1. Detects dirty repos under `core/` (multi-select when several; all selected
   by default; `-f NAME` for one).
2. Asks the AI CLI (default `pi` / `cpamc/muse-spark-1.3-contributor`, from
   `.ipalpha/settings`; `--engine` overrides) for the semver bump + commit
   message, shown as a plan. Decisions are cached in
   `.ipalpha/.publish-cache` (same diff → same decision).
3. Docs-only changes clamp to no version bump.
4. Commits, tags `v<version>`, pushes each repo. shared-js is then published to
   npm (`npm publish`, needs `npm login`) — no image.
5. Builds and pushes `ghcr.io/ipalpha-dev/<ms>:<version>` (Docker or Apple
   `container build`), then bumps the image tag in `deployment/core/<ms>/` and
   pushes `deployment`.

```sh
./publish                 # full dev publish
./publish -d              # preview, then optionally apply that exact plan
./publish -f auth-api     # one repo
./publish clean           # wipe decision cache
```

`ghcr.io` push needs a one-time `docker login ghcr.io` (or
`container registry login`) with a GitHub token with `write:packages`.

## Day-to-day: `./pull`

1. Fast-forward pull of every repo (conflicts skip that folder untouched).
2. Clones repos that are missing from the layout.
3. Creates `.env` for new repos, adds missing keys to existing ones
   (local values are never overwritten).
4. Rewrites ports from `.ipalpha/settings` and refreshes `.ipalpha/`
   (lib, compose, mprocs config) and the `./run` wrapper (so new defaults
   reach existing workspaces).

## Ports (defaults)

| Service | Default host port |
| --- | --- |
| projects-api | 3001 |
| persons-api | 3002 |
| organizations-api | 3003 |
| notifications-api | 3004 |
| auth-api | 3005 |
| forms-api | 3006 |
| dispatch-api | 3007 |
| ai-api | 3008 |
| developers-api | 3009 |
| auth-webapp (Vite) | 5100 |
| forms-webapp (Vite) | 5106 |
| mordomia-webapp (Vite) | 5110 |
| developers-webapp (Vite) | 5111 |
| MongoDB | 27017 |
| Redis | 6379 |
| RabbitMQ | 5672 |
| RabbitMQ management | 15672 |
| Mailpit inbox + Send API (loopback only) | 8025 |

Busy ports are remapped at setup; the mapping lives in `.ipalpha/settings` and
`.ipalpha/ports.env`.

### Local notifications

New notifications-api `.env` files select `MAIL_PROVIDER=mailpit`, `SMS_PROVIDER=mailpit`,
`DEPLOYMENT_ENVIRONMENT=development`, and `MAILPIT_URL=http://127.0.0.1:8025`.
`./run` starts the inbox automatically. Open that URL to read captured email and SMS
(SMS appears as an `[SMS]` email). A busy port is remapped along with the URL.
Existing `.env` values are preserved; remove or update the notifications `.env` to opt in.
No SendGrid, SMS Barato, Comtele, or Mailpit credentials are required; failed capture
never falls back to real delivery. Production manifests and runtime defaults are unchanged.

The inbox is a local development dependency, not a church application or production asset.
It binds only to loopback; use synthetic recipients and codes, never real member data.
Mailpit keeps at most 200 messages for 24 hours in ephemeral container storage, with a
1 MiB message limit, no persistent volume, and a 128 MiB Docker limit (256 MiB with
Apple `container`, whose VM requires at least 200 MiB). This avoids
delivery charges and extra persistent disk cost. Container replacement clears the inbox.
Do not add SMTP relay settings.

## Core and apps

`core/` holds the shared capabilities (auth, persons, projects, organizations, notifications, dispatch,
ai, developers, shared-js, shared-ui) and the core UIs (Mordomia, the auth popup). Apps that only
*consume* core live in `apps/<app>/` — today `apps/forms/{forms-api,forms-webapp}` — and run in their
own namespace in production (`ipalpha-forms`, own Mongo/Redis, events to core over HTTP webhooks).
They still depend on `../../../core/shared-js` / `shared-ui`. `./pull` moves an older workspace's
`core/forms-*` (and its feature worktrees) to `apps/forms/` automatically.

**Acampa Kids** (`apps/acampa-kids/{backend,frontend,face-service}`, GitHub `ipalpha-dev/acampa-kids-backend` /
`acampa-kids-frontend` / `acampa-kids-face-service`) is an app outside core with its own repos, registry path and TeamCity project
(namespace `ipalpha-acampa-kids`). It does not depend on core packages and `./run` does not start it.
Its repos are optional: setup/`./pull` clone them when your account can read them (otherwise one warning)
and `./feature` skips them when absent.

In a feature environment every core service always runs; an app joins only when one of its own
repos changed — a forms change never deploys other apps.

## Feature environments: `./feature`

One feature = one isolated workspace, one `feat/<slug>` branch per touched repo, one preview
namespace with its own Mongo/Redis/RabbitMQ and public HTTPS hosts, alive 72 h after each
successful publish. Design and CI side: `deployment/docs/feature-environments.md`.

```sh
./feature new hello-preview          # features/hello-preview/: worktrees on feat/hello-preview at the last green Core Deploy
cd features/hello-preview
# … change code (e.g. core/forms-webapp) …
./run                                # optional: same as ./run, own ports (+100 per feature) and own infra containers
./publish                            # = ./publish --feature hello-preview: commit (no version bump), push, deploy the preview
cd ../..
./feature list                       # local features, generation, expiry, preview namespaces
./feature extend hello-preview       # +72 h without a build
./feature rebase hello-preview       # move to the latest green Core Deploy (rebases feat/hello-preview)
./feature reset hello-preview        # back to the synthetic seed (asks you to type the slug)
./feature destroy hello-preview      # delete the preview (namespace, record → archive); keeps branches
```

After a successful publish you get:

| URL | What |
| --- | --- |
| `https://ipalpha-<slug>.kevyn.com.br` | Mordomia (with the `preview · <slug> · expires in Nh` badge) |
| `https://forms-ipalpha-<slug>.kevyn.com.br` | IPAlpha Formulários |
| `https://auth-ipalpha-<slug>.kevyn.com.br` | sign-in popup |
| `https://developers-ipalpha-<slug>.kevyn.com.br` | IPAlpha Developers — when the baseline includes developers-webapp |
| `https://ipalpha-<slug>.kevyn.com.br/mailbox` | captured e-mail/SMS: your login codes (shared `previews` account) |
| `https://acampa-ipalpha-<slug>.kevyn.com.br` | Acampa Kids — only when an `acampa-kids-*` repo changed in this feature |

In previews the Developers portal lets an app owner edit their app's project message templates (production after a
reviewed audience change).

Acampa in a preview: `./feature new` pins `apps/acampa-kids/<repo>` at the tag `v<version>` of the image
its production manifest names in the baseline's deployment commit (e.g. backend `0.19.0` → `v0.19.0`;
origin/master with a warning when that tag is missing). Publishing a change to one of them builds it
(`<version>-<slug>-<buildId>`); the others run their production images (the face service included; the
import worker too when Kevyn stored the shared, budget-capped preview OpenRouter key — otherwise CI notes that it
left the worker out). The pipeline registers Acampa in the preview's core through IPAlpha
Developers (request → approval → owner secret → app-bound client) and sets up its project as a steward
would (roles, editions, memberships, message templates); people, families and roles live in core and are
synthetic (fixture v2). Its quota is the core-only budget (1200m / 2560Mi) plus Acampa's increment (+ the
worker's when it runs): core-only previews reserve only the core budget. The
preview needs the per-repo layout of `base/apps/acampa-kids/` in your `feat/<slug>` branch of
`deployment` (a baseline deployed after it, or rebase that branch on master).

Sign in with a fixture account (all fictional — `deployment/fixtures/2/README.md`):
`ana.superuser@example.test` (superuser), `bruno.cuidado@example.test` (steward), `carla@example.test`,
`gabi.presbi@example.test`. Codes never reach a real phone or mailbox; read them in `/mailbox`.
`ci.preview@example.test` is reserved for the pipeline's own sign-in check; the pipeline also signs in
as `rafael.acampa@example.test` (owner of the Acampa app) while it provisions Acampa. Acampa people sign in
by phone or "Entrar com IPAlpha" (`+55 11 90000-0011` coordenação, `-0012` team + bus check-in, `-0013`
team + saúde, `-0014`/`-0015`/`-0017` responsáveis, `-0003` responsável + team, `-0018` last year only →
"não encontramos seu cadastro"); every role per person is in that README.

- Slug: `^[a-z0-9-]{3,30}$`, no hyphen at either end.
- Your main checkouts are never touched: `features/<slug>/core/<repo>` are `git worktree`s. Inside a
  feature folder plain `./publish` means `--feature <slug>`: no release, tag or version bump — CI
  tags images `<version>-<slug>-<buildId>` in its own build copy.
- Unchanged services run the baseline images (`deployment/releases/core-latest.json`, pinned at
  `./feature new`); `shared-js` changes rebuild every API, `shared-ui` every web app. Manifests come
  from your `feat/<slug>` branch of `deployment` (it starts where the baseline was deployed from).
- How you learn the result without any CI token: `./publish` pushes `previews/<slug>/release.json`
  to deployment master, TeamCity `Preview` runs, and CI's answer is its own record commit on master
  (URLs + expiry are printed). `--no-wait` returns right after the push; failures show in TeamCity
  (*Ip Alpha / Core / Previews / Preview*).
- Data survives republishes; `reset` wipes it. 72 h without a publish or `extend` and the preview
  (with its data) is deleted by `PreviewCleanup` / the cluster janitor.
- Previews only hold synthetic data. Never copy member data into one (LGPD).

## Expected degraded integrations (local)

- SMS providers need real `SMSBARATO_KEY` / `COMTELE_TOKEN` in
  `core/notifications-api/.env` — without them notifications-api runs but cannot
  send.
- Superuser seed (`SUPERUSER_NAME/PHONE/EMAIL`) and `WEBAUTHN_RP_ID` are blank
  by default — auth-api will tell you what it needs.
- `AUTH_CLIENT_ID`/`AUTH_CLIENT_SECRET` (machine credentials) come from the
  auth-api client registry; blank until seeded.

## Troubleshooting

- **Apple `container` failing**: run `container system start`; if the runtime is
  unusable, set `runtime=docker` in `.ipalpha/settings` (requires Docker
  Desktop with the daemon running).
- **Docker daemon unavailable**: start Docker Desktop; on WSL enable WSL
  integration.
- **Port conflicts**: `./pull` re-applies saved ports; or re-run the setup command
  to remap busy ports.
- **Mongo auth failures after changing compose credentials**: wipe the volume
  (`ipalpha-mongo-data`) — local data only — and `./run` again.
- **`pi` missing for publish**: install the Pi CLI, or
  `./publish --engine claude|grok|codex` as a one-off.

## FAQ

### Why are the APIs not in containers?

Node processes on the host keep edit→restart fast and debuggable; only the
infra (Mongo, Redis, RabbitMQ) runs in containers.

### Where do env samples live?

Each repo should own `.env.example`. Until then, the tooling's
`templates/env-fallback/<ms>.env` is used with a warning. Secrets are always
blank in templates — fill them in each repo's local `.env` only.

### Why is the tooling not a repo in my workspace?

Same as Cross: tooling lives in the org `.github` repo and is copied into
`.ipalpha/`. The workspace only holds product repos, so nothing to keep in sync by hand.
