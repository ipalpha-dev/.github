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
   `core/{auth-api,person-api,organization-api,projects-api,notification-api}`,
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
| `shared-ui` | React component library `@ipalpha/shared-ui`, consumed by every frontend via `file:../../shared-ui`; built by `install-deps` before the frontends (not published) |
| `auth-api` | NestJS + TypeScript backend, no frontend (its UI is `auth-webapp`) |
| `person-api`, `organization-api`, `projects-api`, `notification-api` | NestJS + TypeScript backend, each with a React+Vite frontend served under `/frontend`; sign-in goes through the auth-webapp popup (`VITE_AUTH_WEBAPP_URL`, `VITE_AUTH_API_URL` = webapp `/api`) |
| `auth-webapp` | standalone Vite app: the sign-in popup (account chooser + consent). No `.env`; served at `/` (no `/frontend` prefix, like prod) on its own port with `/api` proxied to auth-api so there is no CORS; published as an nginx image |
| `deployment` | k8s manifests under `core/<ms>/`, namespace `ipalpha-core` |

The org name lives in one place (`ipalpha_org` in `lib/common.sh`); image names
follow `ghcr.io/<org>/<ms>`.

## Day-to-day: `./run`

1. Starts infrastructure in containers and waits for health:
   MongoDB, Redis, RabbitMQ (+ management UI on 15672). Apple `container` runs
   the services directly (network `ipalpha`, named volumes `ipalpha-*-data`);
   Docker uses `docker compose --wait`.
2. Installs missing npm dependencies per service (`@ipalpha/shared-js` comes from npm; the `core/shared-js` clone is only for changing the library).
3. Starts the APIs in order — projects-api, person-api, organization-api,
   notification-api, auth-api. Everything except projects-api waits for
   projects-api's HTTP port; auth-api also waits for notification-api.
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
| person-api | 3002 |
| organization-api | 3003 |
| notification-api | 3004 |
| auth-api | 3005 |
| auth-webapp (Vite) | 5100 |
| MongoDB | 27017 |
| Redis | 6379 |
| RabbitMQ | 5672 |
| RabbitMQ management | 15672 |

Busy ports are remapped at setup; the mapping lives in `.ipalpha/settings` and
`.ipalpha/ports.env`.

## Expected degraded integrations (local)

- SMS providers need real `SMSBARATO_KEY` / `COMTELE_TOKEN` in
  `core/notification-api/.env` — without them notification-api runs but cannot
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
