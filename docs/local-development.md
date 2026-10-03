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
| `auth-api`, `persons-api`, `organizations-api`, `projects-api`, `notifications-api`, `forms-api`, `dispatch-api` | NestJS + TypeScript backends, **no frontend of their own** |
| `mordomia-webapp` | standalone Vite app (port 5110): Mordomia, the one UI for superuser + stewards over every core API (persons, projects, org chart, notifications, access, my data), live through dispatch-api |
| `auth-webapp` | standalone Vite app (port 5100): the sign-in popup (account chooser + consent) |
| `forms-webapp` | standalone Vite app (port 5106): IPAlpha Formulários |

Every webapp is served at `/` and reaches the APIs same-origin at `/api/<name>` (`/api/auth`,
`/api/projects`, `/api/persons`, `/api/organizations`, `/api/notifications`, `/api/forms`;
sockets at `/api/dispatch/socket.io`): the Vite proxy locally, the ingress in production. No CORS.
| `deployment` | k8s manifests under `core/<ms>/`, namespace `ipalpha-core` |

The org name lives in one place (`ipalpha_org` in `lib/common.sh`); image names
follow `ghcr.io/<org>/<ms>`.

## Day-to-day: `./run`

1. Starts infrastructure in containers and waits for health:
   MongoDB, Redis, RabbitMQ (+ management UI on 15672). Apple `container` runs
   the services directly (network `ipalpha`, named volumes `ipalpha-*-data`);
   Docker uses `docker compose --wait`.
2. Installs missing npm dependencies per service (`@ipalpha/shared-js` comes from npm; the `core/shared-js` clone is only for changing the library).
3. Starts every API and web app at once — nothing waits for a peer. Each core MS
   exposes `GET /live` (process up) and `GET /ready` (200 only when Mongo, Redis,
   RabbitMQ and the projects cache are all good; 503 `{ready:false, checks}` otherwise).
   The panel polls `/ready` and shows ● ready / ◐ up-but-not-ready per row; k8s uses the
   same two paths as liveness/readiness probes.
   Processes run in the background with logs under `$TMPDIR/ipalpha-run-logs`.

Stop: Ctrl+C, then `.ipalpha/bin/infra-down`.

Runner override: `runner=` in `.ipalpha/settings` or `IPALPHA_RUNNER=` —
`background` (default, embedded), `mprocs` (opt-in; needs `mprocs` installed).

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
| auth-webapp (Vite) | 5100 |
| MongoDB | 27017 |
| Redis | 6379 |
| RabbitMQ | 5672 |
| RabbitMQ management | 15672 |

Busy ports are remapped at setup; the mapping lives in `.ipalpha/settings` and
`.ipalpha/ports.env`.

## Core and apps

`core/` holds the shared capabilities (auth, persons, projects, organizations, notifications, dispatch,
ai, developers, shared-js, shared-ui) and the core UIs (Mordomia, the auth popup). Apps that only
*consume* core live in `apps/<app>/` — today `apps/forms/{forms-api,forms-webapp}` — and run in their
own namespace in production (`ipalpha-forms`, own Mongo/Redis, events to core over HTTP webhooks).
They still depend on `../../../core/shared-js` / `shared-ui`. `./pull` moves an older workspace's
`core/forms-*` (and its feature worktrees) to `apps/forms/` automatically.

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
| `https://ipalpha-<slug>.kevyn.com.br/mailbox` | captured e-mail/SMS: your login codes (shared `previews` account) |

Sign in with a fixture account (all fictional — `deployment/fixtures/1/README.md`):
`ana.superuser@example.test` (superuser), `bruno.cuidado@example.test` (steward), `carla@example.test`,
`gabi.presbi@example.test`. Codes never reach a real phone or mailbox; read them in `/mailbox`.
`ci.preview@example.test` is reserved for the pipeline's own sign-in check.

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
