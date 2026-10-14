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

## Hosted One Tap SDK and local HTTPS/FedCM

The auth-webapp development server and image both serve `/sdk/v1/client.js` and `/sdk/v1/client.css`.
Forms uses this SDK for recognized accounts while keeping creator tokens and one-time form exchanges
in its own client. Normal HTTP development still works; FedCM is optional and disabled when
`auth-api/.env` has a blank `FEDCM_PROVIDER_ORIGIN`. `./pull`, setup and `./run` add this new key without
replacing a developer's values. No extra service, port, seed client or Mongo collection is required.

For browser-level cross-site testing, use two genuinely different registrable HTTPS domains and trusted
local certificates (not certificates with browser warnings). In the auth webapp `.env`:

```dotenv
IPALPHA_WEB_TLS_CERT=<absolute trusted certificate path>
IPALPHA_WEB_TLS_KEY=<absolute private key path>
IPALPHA_WEB_ORIGIN=https://auth.ipalpha.test:5100
```

Use the actual port selected by `./run`; a mismatch fails startup rather than silently producing bad
callbacks. These values are local-only and forwarded to Vite, not browser bundle secrets or container
assets. The runner shows and checks the configured HTTPS URL and continues to verify its certificate.
Configure the application webapp similarly for its own trusted HTTPS domain. Existing app `VITE_*`
settings must point to that auth host; auth APIs remain `/api/auth` on **only** the auth host.

In `auth-api/.env`, include that HTTPS origin in `AUTH_FRONTEND_ORIGINS`, set
`FEDCM_PROVIDER_ORIGIN` to it, and use the matching `WEBAUTHN_RP_ID` (e.g. `ipalpha.test`). Preserve any
other origins needed by the local run. Register exact HTTPS app origins and callbacks; passkey RP
changes also require corresponding webapp origins. Choose the final RP before real-user enrollment.

Serve `https://ipalpha.test/.well-known/web-identity` through a separate trusted local HTTPS reverse
proxy, with the JSON returned by the auth host's `/api/auth/fedcm/discovery`. Route **only that exact
public discovery path**, never normal sign-in APIs through the apex. Set local DNS/hosts entries for
both sites and the discovery host. The final public IPAlpha domain is undecided; public apex deployment
is deliberately not included. The SDK supports configured domains, not one hard-coded host.

Check fresh proof → current/new consent, cancelled or unavailable passkeys, explicit SMS fallback,
blocked popups, wrong origin/source/state, expired/replayed bootstrap tickets and account removal.
FedCM selection is only a 120-second bootstrap ticket, never proof or permission consent. Independent
domains/browser versions without FedCM retain ordinary sign-in. Portal recipes: `/docs/one-tap` in all
five languages, including browser tokens, backend PAR and application-owned code adapters.

## Church campaign emails and AI writing

Oikos Comunicação now stores campaign drafts and queues test/immediate/scheduled email delivery. Local delivery remains Mailpit-only; no provider keys or paid sends are needed. The existing RustFS instance holds chosen images in `church-email-images`. Node 22.12+ is required by the patched server HTML sanitizer; setup/doctor enforce it, and the existing `node@22` install instruction stays valid.

Setup and `./pull` add missing notifications keys: `ORGANIZATIONS_API_URL`, `ORGANIZATIONS_API_AUDIENCE`, `EXCHANGE_CHURCH_EMAIL_CHANGED`, `CHURCH_EMAIL_PUBLIC_URL`, `CHURCH_EMAIL_UNSUBSCRIBE_KEY`, `S3_ENDPOINT`, `S3_REGION`, `CHURCH_EMAIL_IMAGES_BUCKET`, `S3_ACCESS_KEY`, `S3_SECRET_KEY`. The unsubscribe key is generated from 32 random bytes and kept; the S3 credentials follow the local RustFS credentials without overwriting custom values. Dispatch gets `NOTIFICATIONS_API_URL` and the same exchange. The notifications system client gains `organizations:snapshot`; no third-party scope or new microservice is added. Run/publish/feature use these normal env/catalog paths; do not add custom startup scripts.

`CHURCH_EMAIL_PUBLIC_URL` defaults to Oikos at `http://localhost:5110/api/notifications/church-emails`; the run-time port rewriter follows remapped ports. Use the actual Oikos HTTPS origin when configuring local TLS. Outside development/test the base must be HTTPS, on the Oikos host; its origin serves `/church-email-unsubscribe`. All notifications replicas share the private unsubscribe key. Public image URLs contain only opaque asset IDs. GET validates an unsubscribe without mutating it; browser confirmation and RFC 8058 provider POST disable campaigns only.

Writing uses `aiSettings.assist` in the existing AI service (defaults to the current completion model); images and reference edits use its existing image-model priority list. The person reviews all suggestions and explicitly sends. No AI action chooses recipients or sends email. Recipient-language preparation uses the existing translation model: four versions are prepared automatically without per-language manual approval, stored with the source hash and required current before send/schedule. Delivery selects the live preferred language (unknown uses pt-BR), never calls AI per recipient. No new env, scope, collection or microservice is needed. A 202 means queued, not delivered; provider acceptance is not an inbox receipt. Uncertain acceptance is never automatically retried. Draft content remains only in the open screen when offline; never persist it or person data in browser storage. Portal reference: `/docs/church-emails`, in all five languages.

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
2. **Tools**: Git, Node.js ≥ 22.12, Docker (+ Compose v2); optional GitHub CLI and kubectl. A missing tool can
   be installed after confirmation with winget (Windows), Homebrew (macOS) or apt/dnf/pacman (Linux).
3. **GitHub**: an SSH key works; without one the GitHub CLI signs in through the browser (`gh auth login`).
4. **Repositories** cloned in parallel into `core/`, `apps/<app>/` and `deployment/`. Optional app repos
   (Acampa Kids) are skipped quietly when your account cannot read them.
5. **`.env` per API** from its `.env.example` (or the embedded fallback in `internal/assets/files/env-fallback`
   until the repo has one). Re-runs and `./pull` only **add missing keys** and fill blanks; local values are
   never overwritten — except notifications, which always go to Mailpit locally, and the sample-data keys owned by the
   `seed` choice (see "Sample data").
6. **First sign-in**: your name and mobile number become the local superuser. Codes arrive in Mailpit. Setup then asks
   the **password** (masked, typed twice; 8-200 characters with a number and a symbol, the same rules as auth-api) so
   the first Oikos sign-in does not stop at "Precisamos configurar sua senha"; empty skips it. It is saved as
   `SUPERUSER_PASSWORD` in `core/auth-api/.env` only (git-ignored; a value already there is kept), and auth-api
   hashes it (argon2id) when it seeds the superuser.
7. **Sample data**: "Seed with data?" (Enter = yes; `--yes` = yes; `--no-seed` = no). See "Sample data" below.
8. **AI for `./publish`**: pick one of the coding CLIs already installed (all presets are listed only when none is),
   then pick its model from the list the CLI itself reports — no slug to type. Or your own command (see below).
9. **Files**: `.ipalpha/` (settings, compose file, binary, logs) and the wrappers.
10. **npm dependencies** (failures do not stop setup; `./run` retries).

Non-interactive (CI): `ipalpha setup <folder> --yes` with `IPALPHA_SUPERUSER_PHONE`, `IPALPHA_AI_CLI=none`, and optionally
`IPALPHA_SUPERUSER_PASSWORD` (absent = the password is created at the first sign-in, as before; an invalid one fails setup).

## `./run`

1. Starts Docker Desktop / Apple container when it is installed but off (up to 2 minutes).
2. **Ports**: each service keeps its preferred port (`.ipalpha/settings`) unless something else holds it;
   then it moves to the next free port **for this run only** and every URL handed to the processes follows
   (`.env` files keep the defaults). The panel says which moved and who held the port. The infra keeps a
   moved port across runs while its container lives. Two workspaces can run at the same time.
3. Infra containers (MongoDB, Redis, RabbitMQ + UI, Mailpit, RustFS, cpamc simulator) up and healthy (`docker compose --wait`),
   then the `photos`, `church-email-images` and `documents` buckets are created in RustFS when missing (one-shot
   `rustfs-init`, safe to repeat).
4. `.env` completion, npm installs (shared-js built first, consumers' copies refreshed). A build stamp
   (`.ipalpha/.state/shared-js.stamp`: shared-js git HEAD plus a hash of uncommitted changes) records what was built; when
   shared-js moved, `./run`, `./pull` and `./feature rebase` rebuild it and reinstall every consumer's copied
   `file:../shared-js` (so no service runs a stale dist). Unchanged stamp: nothing runs.
5. auth-api signing keys, then the **local** auth database is reconciled: drifted client secrets, `allowedScopes`
   (exactly the `SEED_CLIENTS_JSON` entry, so new grants reach clients auth-api seeded earlier), the `appId` of
   app-bound clients of the local services (removed from a client that is no longer bound, e.g. sign-api) and the `localhost` origins/callbacks of the built-in apps (so sign-in keeps working when a web
   port moved). Clients of outside apps (services with no entry in `system-clients.json`, e.g. Services Kids, registered through the portal) only get their secret synced: their `appId` and scopes (`login:relay`...) are never rewritten. Only loopback MongoDB; non-loopback entries are never touched.
6. Every API and the remembered web apps start at once — nothing waits on a peer. Each API's `GET /ready`
   drives its dot: ● ready, ◐ starting/not ready, ✖ failed, ○ stopped.

The panel and `./status` list **core APIs → core frontends → apps → infrastructure**. Forms and IPAlpha
Sign are core tools: Forms API and Sign API list with the core APIs, Forms Web and Sign Web with the core
frontends. An app outside core stays in one group, with its backend immediately before its frontend. This
is display order: infrastructure is prepared before application processes start.

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

Forms creator image generation uses the creator's `ipalpha:ai` church token and the same-origin
`/api/ai/images` route. `./run` already supplies `AI_API_URL` (and `FORMS_API_URL` / `DISPATCH_API_URL`)
with this run's assigned ports; the Forms Vite proxy reads those values. No new required environment
variable or system-client scope is introduced. Setup and `./pull` continue to preserve local values.
The production Forms host routes `/api/ai` to core ai-api, never `/api/auth`.

Church-only forms return through the existing respondent callback after full-page sign-in. Only PKCE
and transaction metadata cross that redirect in session storage; person values and respondent receipts
stay in memory and are never persisted on the device. Guests forms never invoke church prefill or emit
`forms.person.requested`. Image redo replaces its MongoDB slot, max 700,000 bytes of compressed WebP.

Remembered pages start (web apps) and open once their server answers. Default: Auth Web (starts, never gets
a tab — it is the sign-in popup), Oikos, Mailpit. Mailpit (it has the sign-in codes) always opens on a `./run`
that opens pages, whatever `browser_apps` holds, even with no other app chosen; `x` on it does not forget it and
`./ipalpha apps` does not list it. Starting/opening a page in the panel remembers it, `x`
forgets it. Outside the panel: `./ipalpha apps` (menu), `./run apps set oikos-webapp mailpit`,
`./run apps defaults`. `IPALPHA_OPEN_BROWSERS=0` or `--no-browser` skips opening once.

### Project-restricted Forms

Forms has a separate identity-only `respondent-login` entry; the existing `respondent` entry is still
prefill-only. Setup/pull add these keys without overwriting a developer's values:

- auth-api: `BUILTIN_FORMS_RESPONDENT_LOGIN_CLIENT_ID`, `_ORIGINS`, `_REDIRECT_URIS`
  (local callback `http://localhost:5106/respond/login/callback`).
- forms-api: `PROJECTS_API_URL`, `FORMS_RESPONDENT_LOGIN_ENTRY_POINT_ID`.
- projects-api: `SELF_ONLY_ENTRY_POINT_IDS` (restricts the respondent entry to explicit self-read APIs).

`./run` supplies `VITE_RESPONDENT_LOGIN_CLIENT_ID`, `VITE_RESPONDENT_LOGIN_ENTRY_POINT`, and
`VITE_RESPONDENT_LOGIN_CALLBACK_URI`, honoring local overrides and assigned ports. The Forms Vite proxy
and production host mount `/api/projects` same-origin; `/api/auth` is NEVER mounted on Forms.
Forms runtime `/config.json` has `respondentLoginClientId`, `respondentLoginEntryPoint`,
`respondentLoginCallbackUri`. No new repository, catalog port, system scope or shared-js version is needed.
`./publish` and `./feature` use the updated canonical manifests; host-rewriting remains generic.
Do not publish a preview for this pre-launch change; verify locally.

Audience targets are multiple `{projectId,roleKeys?}` entries. Permissions come live from
`GET /projects/:id/me/access`; the picker uses `GET /projects/me/managed`. Current-edition changes
change eligibility. Forms exposes paginated `/projects/:projectId/forms` for answerable discovery and `/forms/managed`
for administrative metadata from every creator. Oikos registers the optional provider in runtime
`projectContentProviders`; local `VITE_PROJECT_CONTENT_PROVIDERS_JSON` is emitted by `./run` and honors
overrides (`[]` disables it). The app-owned provider ingress and Vite proxy serve `/api/forms` on Oikos,
including the websocket at `/api/forms/socket.io`.

Never forward Oikos's broad tokens or its Projects token to the provider. Oikos sends its auth proof
only to auth-api `POST /oauth/delegate` and forwards only a one-project, one-audience, metadata-only
`delegated_access` JWT (role person, ≤60 s, capped by the proof expiry). Existing public JWKS verifies
it; no browser/shared secret, new system permission, database collection or shared-js release.
Required auth `PROJECT_CONTENT_DELEGATES_JSON` matches source app/entry, target app, audience and
`project-content:read` scope. Forms requires exact `PROJECT_CONTENT_ORIGINS`. Setup/pull add missing
keys and preserve local values. Generic run/feature host/port rewriting handles provider records and
websocket origins; publish consumes these canonical manifests without a new catalog service/port.

The provider owns Socket.IO and own Redis pub/sub of affected project ids; no dispatch/core business
integration. `project-content-invalidated` is an empty invalidation, not form/person data. Reconnects
refetch authoritative lists; credential expiry/permission refusal clears them and a provider outage
never prevents loading the project. Delegation is renewed lazily while the section is visible, roughly
once per 50 s. Oikos's public catalog includes the full provider family and phone/desktop rare states.
No feature preview or production deployment is authorized by implementing this change.

### Local notifications

notifications-api always uses Mailpit locally (`MAIL_PROVIDER=mailpit`, `SMS_PROVIDER=mailpit`), even when
vendor keys exist; SMS appear as `[SMS]` e-mails. Mailpit binds to loopback, keeps ≤ 200 messages for 24 h,
no persistent volume. Use synthetic recipients only.

### Local photos (RustFS and the face service)

photos-api stores files in **RustFS**, the S3-compatible store production uses (`rustfs/rustfs:1.0.1`). It runs
with the other infra on `127.0.0.1:9000` (volume `<infra>-rustfs-data`, no console). The keys default to
`ipalpha` / `ipalpha-secret` and live in `.ipalpha/.env` (`RUSTFS_ACCESS_KEY`, `RUSTFS_SECRET_KEY`); photos-api's
`S3_ACCESS_KEY` / `S3_SECRET_KEY` follow them, and `S3_ENDPOINT` follows the port of the run.

The **face service** (a core service, repository `ipalpha-dev/face-service` cloned to `core/face-service`; image
`registry.kevyn.com.br/ip-alpha/core/face-service`, built by Core's TeamCity like every core image; it used to be
Acampa's `acampa-kids-face-service`, and only photos-api calls it; needs registry access and downloads a model on first start) is **off by default**. With it off, `FACE_SERVICE_URL` is empty for
photos-api: uploads work, face indexing and search stay off. Turn it on with `./run --face` (remembered as
`face=on` in `.ipalpha/settings`; `ipalpha config set face off` turns it off). It then runs on `127.0.0.1:8000`.
The image only has a `linux/amd64` build, so compose and Apple `container` run it with `--platform linux/amd64`; on Apple
Silicon it is emulated and noticeably slower (first start and each detection).

The face service repository is optional for `./pull` and `./doctor` (skipped quietly when you cannot read it);
`./run --face` only needs the image. Env added for faces by consent and the programme: photos-api gets
`DOCUMENTS_API_URL` / `DOCUMENTS_API_AUDIENCE` (which document versions grant `faceRecognition`),
`SCHEDULE_API_URL` / `SCHEDULE_API_AUDIENCE` (the span of an album event linked to a programme entry) and the consumed
`EXCHANGE_DOCUMENTS_CHANGED`, `EXCHANGE_SCHEDULE_ENTRY_CHANGED`, `EXCHANGE_SCHEDULE_ENTRY_DELETED`; its system client
gains `documents:read` and `schedule:read`. places-api gets `AUDIT_RETENTION_DAYS=365` (how long `placeAudits` rows
are kept). Acampa Kids' backend is an `./run` process (see "Acampa Kids" below); its `IPALPHA_SCHEDULE_API_URL`,
`IPALPHA_PLACES_API_URL` and `IPALPHA_PHOTOS_API_URL` are filled in its own `.env`.

Oikos reaches photos-api at `/api/photos` through its Vite proxy; `./run` hands it `PHOTOS_API_URL` for the port of
the run, like every other API.

### Local developers-api AI (auto-translation)

developers-api needs `AI_API_URL` (ai-api, port 3008), `AI_API_AUDIENCE=ipalpha:ai` and `AI_BILLING_PROJECT_ID=ipalpha-developers` (required at boot; `TRANSLATE_DAILY_CAP` is optional, default 200). `./pull` and `./feature` only add the missing keys, never overwriting yours. Its local system client gets `ai:invoke`, and ai-api's `AI_INVOKE_CALLER_SERVICE_IDS` gains `developers-api` (an existing list is only extended). Without them `/translate` answers 503 `aiUnavailable`.

### Local app features and directory (apps-api)

apps-api (port 3015, MongoDB `apps`, audience `ipalpha:apps`) owns per-app features, skills, directory listings, links
and banner grants. `./run` starts it and hands auth-api, ai-api, developers-api and dispatch-api `APPS_API_URL`; Oikos
and Developers reach it at `/api/apps`. Its repository has an origin, so `./pull` and `setup` clone it like any other core service. ai-api also gets `AI_TRANSCRIBE_URL` / `_MODEL` / `_KEY`; the directory and banner-grant exchanges moved from
`developers-api.*` to `apps-api.*`.

### Local documents and consent (documents-api)

documents-api (port 3012, MongoDB `documents`) holds versioned legal texts (membership term, `face-recognition`
consent). `./run` starts it with the other APIs and hands every consumer `DOCUMENTS_API_URL`; Oikos reaches it
at `/api/documents`. persons-api verifies the consent version a client sends with a live call to it, so
consent writes answer 503 while documents-api is down.

Env added for this: persons-api gets the `EXCHANGE_PERSON_PHOTO_*` / `EXCHANGE_PERSON_CONSENT_CHANGED`
exchanges, `DOCUMENTS_API_URL`, `DOCUMENTS_API_AUDIENCE`, `MEMBERSHIP_TERM_DOCUMENT_SLUG` and
`FACE_RECOGNITION_DOCUMENT_SLUG`; photos-api gets `TOKEN_AUDIENCE=ipalpha:photos`, `PERSONS_API_URL` /
`PERSONS_API_AUDIENCE`, the consumed and published exchanges, `FACE_REVIEW_THRESHOLD=0.35` and the two slugs;
dispatch-api gets `PHOTOS_API_URL` and the `EXCHANGE_PHOTOS_*` exchanges. auth-api's
`AUTH_RESOURCE_AUDIENCES_JSON` lists `ipalpha:photos` and `ipalpha:documents`. `./pull` and `setup` only add
missing keys, never overwrite yours; the exception is appending missing built-in audiences to an existing
`AUTH_RESOURCE_AUDIENCES_JSON`, preserving every custom entry. This includes `ipalpha:schedule` and `ipalpha:sign`
alongside photos/documents. `./run`, `./pull` and setup apply the repair idempotently. If auth-api exits with
“must register ipalpha:schedule”, its local list is stale; use the updated tooling and restart `./run` so the
process receives the repaired environment. Persons' projects cache can recover once auth is available; never
remove readiness checks or bypass authentication to hide this failure. The embedded `system-clients.json` grants `persons:links`, `persons:consents`,
`persons:face-photos`, `persons:access-log` (photos-api, persons-api) and `documents:read` (persons-api).

### IPAlpha Drive (core tool)

`core/drive-api` (port **3018**, MongoDB `drive`, audience `ipalpha:drive`, RustFS bucket `drive`) and
`core/drive-webapp` (**5116**). Both repos have an origin and are cloned by `./pull` and `setup`.
`drive-api.env` follows the Sign pattern (`EXCHANGE_AUTH_REVOCATIONS`, `EXCHANGE_DRIVE_ITEMS_CHANGED=drive-api.items.changed`, `EXCHANGE_DRIVE_USAGE_CHANGED=drive-api.usage.changed`,
`EXCHANGE_PERSON_DELETED`, `PROJECTS_API_*`, `S3_*` with keys from `.ipalpha/.env`, `S3_PUBLIC_ENDPOINT` for presigned
browser URLs). Contact shares go out through notifications-api: `NOTIFICATIONS_API_URL`/`NOTIFICATIONS_API_AUDIENCE` and `DRIVE_PUBLIC_URL=http://localhost:5116` (no persons-api keys; persons-api has no `RESOLVE_CONTACT_CALLER_SERVICE_IDS`). The `drive-api` system client has `revocations:read` and `projects:read`. Added keys only: dispatch-api
gets `DRIVE_API_URL`, `EXCHANGE_DRIVE_ITEMS_CHANGED`, `EXCHANGE_DRIVE_USAGE_CHANGED` and `http://localhost:5116` in `SOCKET_ALLOWED_ORIGINS`; auth-api
gets `ipalpha:drive` in `AUTH_RESOURCE_AUDIENCES_JSON`. The `rustfs-init` job also creates the `drive` bucket and applies
CORS for `http://localhost:5116` and Oikos (`5110`). Oikos' Vite proxy reads `DRIVE_API_URL` like every other API.

### Virtual assistant (core)

`core/virtual-assistant` (port **3019**, MongoDB `virtual-assistant`, audience `ipalpha:assistant`, RustFS bucket
`virtual-assistant`; the design's 3018 was already Drive's). The repo has an origin (`ipalpha-dev/virtual-assistant`), so `./pull` and `./feature` clone and fetch it. Locally it listens on **3019**; in the cluster it stays on 3018. WhatsApp is off unless `WHATSAPP_ENABLED=true` (`./pull` only adds the missing `WHATSAPP_ENABLED=false`, never overwriting your value); the Meta `WHATSAPP_*` vars are required only then, and Twilio voice works independently. Its
`.env` fallback holds dummy WhatsApp/Twilio keys (nothing reaches a provider locally), the `EXCHANGE_ASSISTANT_*`
names and `S3_*` keys from `.ipalpha/.env`. Added keys only: ai-api gets `ON_BEHALF_CALLERS=virtual-assistant`;
dispatch-api gets `VIRTUAL_ASSISTANT_API_URL` and the two
`EXCHANGE_ASSISTANT_*` names; auth-api gets `ipalpha:assistant` in `AUTH_RESOURCE_AUDIENCES_JSON`. The `rustfs-init`
job creates the bucket. Since 0.2.0 it needs Redis (the shared local Redis `./run` already starts; `REDIS_URL`), and its fallback adds `ORGANIZATIONS_API_URL`/`ORGANIZATIONS_API_AUDIENCE`, `ASSISTANT_BILLING_PROJECT_ID` (a local placeholder; any non-empty string works) and `EXCHANGE_COSTS_USAGE_RECORDED`. `MAX_CALL_MINUTES` and `CALL_END_GRACE_MS` are gone (existing `.env` files just keep the unused lines). Calls need organizations-api and ai-api up, but boot never waits on a peer. The local `virtual-assistant` system client gets `ai:on-behalf` (replaces `ai:assistant`), `persons:eligibility` and `organizations:snapshot`, and persons-api's `ELIGIBILITY_CALLER_SERVICE_IDS` gains `virtual-assistant`. The assistant's behaviour is edited in Oikos (`/assistant-settings`). Web apps receive `VIRTUAL_ASSISTANT_API_URL` from `./run` (Oikos mounts `/api/virtual-assistant`).

### Services Photo desktop apps

`apps/services-photo/services-photo-macos` (SwiftUI; open in Xcode) and `apps/services-photo/services-photo-windows`
(WinUI 3 / C#; open in Visual Studio on Windows) are private repos. `./pull` clones them and `./feature new/rebase/destroy`
handle their worktrees, but they have no port or `.env`, are not started by `./run` and are not published or deployed.

### Forms and IPAlpha Sign (core tools)

Forms and IPAlpha Sign are **core tools** (Kevyn, 2026-10-12): `core/forms-api` (port **3006**, MongoDB `forms`,
core Redis with `forms:` key prefixes), `core/forms-webapp` (**5106**), `core/sign-api` (**3014**, MongoDB `sign`,
audience `ipalpha:sign`, no Redis) and `core/sign-webapp` (**5113**). Both use the core infra and talk to core
over RabbitMQ (no HTTP webhooks). The web apps' Vite proxies mount their APIs (`/api/forms`, `/api/sign`,
`/api/documents`, `/api/persons`, `/api/projects`, `/api/dispatch`) same-origin with the ports of the run;
auth-api stays cross-origin. Start a page from the panel (or `./run apps set sign-webapp`).

`./pull` moves an old `apps/forms/{forms-api,forms-webapp}` and `apps/sign/{sign-api,sign-webapp}` checkout to
`core/` (feature worktrees included, `.env` and uncommitted work kept). It only renames: when both folders exist
it moves nothing and warns, and `apps/forms` / `apps/sign` are removed only once empty.

Env for this (`./pull` and `setup` only add missing keys and fill blanks; yours are never overwritten):

- auth-api: `BUILTIN_FORMS_RESPONDENT_CLIENT_SECRET` (the respondent entry point is a confidential client;
  ≥ 16 characters, local default `dev-forms-respondent-secret-change-me`), `EXCHANGE_PERSON_DELETED`;
  `sign-api` is appended to an existing `SEND_TEMPLATE_UNBOUND_CLIENT_IDS`. `FORMS_API_URL` and
  `PREFILL_TRANSACTION_TTL_SEC` are no longer read (delete them when you like).
- forms-api: `RABBITMQ_URL`, `EXCHANGE_FORMS_RESPONSE_CREATED`, `EXCHANGE_FORMS_PERSON_REQUESTED`,
  `FORMS_RESPONDENT_CLIENT_ID` / `FORMS_RESPONDENT_CLIENT_SECRET` / `FORMS_RESPONDENT_REDIRECT_URI` (filled from
  auth-api's `BUILTIN_FORMS_RESPONDENT_*`: they must be equal), optional `FORMS_RESPONDENT_ENTRY_POINT=respondent`.
  `DISPATCH_API_URL` is no longer read.
- sign-api: `SERVICE_NAME`, `RABBITMQ_URL`, `AUTH_API_AUDIENCE=ipalpha:auth`, `EXCHANGE_AUTH_REVOCATIONS`,
  `EXCHANGE_SIGN_REQUEST_CHANGED`, `EXCHANGE_SIGNED_DOCUMENTS_CHANGED`, `EXCHANGE_PERSON_DELETED`, plus
  `DOCUMENTS_API_URL`, `NOTIFICATIONS_API_URL`, `SIGN_APP_ID`, `SIGN_ENTRY_POINT_ID`,
  `SIGN_WEBAPP_URL=http://localhost:5113`. `DISPATCH_API_URL` and `SIGN_WEBHOOK_SECRET` are no longer read.
- persons-api: `EXCHANGE_FORMS_PERSON_REQUESTED`, `SIGNING_FIELDS_CALLER_SERVICE_IDS=documents-api`.
  `WEBHOOK_ALLOWED_CLIENT_IDS` is no longer read.
- dispatch-api: `EXCHANGE_FORMS_RESPONSE_CREATED`, `SIGN_API_URL`, `EXCHANGE_SIGN_REQUEST_CHANGED`;
  `http://localhost:5113` is appended to an existing `SOCKET_ALLOWED_ORIGINS`. `WEBHOOK_ALLOWED_CLIENT_IDS` and
  `EXCHANGE_SIGNED_DOCUMENTS_CHANGED` are no longer read (sign-api consumes documents-api's event itself).
- documents-api: `PERSONS_API_URL` / `PROJECTS_API_URL` + audiences, `EXCHANGE_PERSON_DELETED`,
  `EXCHANGE_SIGNED_DOCUMENTS_CHANGED`, `SIGNING_MAX_AUTH_AGE_SEC`, `S3_*` (`S3_BUCKET=documents`; keys follow
  `.ipalpha/.env` like photos-api), `SEAL_DEV_SELF_SIGNED=true`, empty `SEAL_CERT_*`, `SIGN_WEBAPP_URL`,
  `TRUST_PROXY_HOPS=1`.
- notifications-api: `SIGN_APP_ID`, `SIGN_WEBAPP_URL`.
- auth-api built-in Sign entry: `BUILTIN_SIGN_CLIENT_ID=sign-web`, `BUILTIN_SIGN_ORIGINS`, `BUILTIN_SIGN_REDIRECT_URIS`;
  `ipalpha:sign` is appended to an existing `AUTH_RESOURCE_AUDIENCES_JSON`. `./run` reconciles the Sign entry's
  localhost origin and callback like the other built-in apps and hands sign-webapp `VITE_SIGN_CLIENT_ID`,
  `VITE_SIGN_ENTRY_POINT`, `VITE_SIGN_CALLBACK_URI`, `VITE_OIKOS_URL`.

Signed PDFs live in documents-api: it stores them in the RustFS bucket `documents` and seals them with a
generated self-signed key locally (`SEAL_DEV_SELF_SIGNED=true`; production uses the church seal certificate in
`SEAL_CERT_P12_BASE64` / `SEAL_CERT_PASSWORD`, never set locally). Signing asks for a fresh sign-in
(`SIGNING_MAX_AUTH_AGE_SEC=300`). E-mails (request, reminder, completed) arrive in Mailpit.

System clients (`system-clients.json`): forms-api holds only `persons:read` (its events go over RabbitMQ and it
runs without revocation sync); sign-api is an **unbound** core client with `revocations:read` and
`notifications:send-template` (`appBound` is empty). Setup and `./pull` remove exactly the retired grants
(forms-api `prefill:*`, `persons:write`, `dispatch:events`, `revocations:read`; sign-api `dispatch:app-channel`) and
sign-api's old `appId` from `SEED_CLIENTS_JSON`; every other scope you added is kept. documents-api has
`persons:signing-fields`, `persons:access-log`, `projects:read`.

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
   **Acampa Kids** (`-f acampa-kids-backend|acampa-kids-frontend`, also picked up by a bare run) goes the same way:
   image `registry.kevyn.com.br/ip-alpha/apps/acampa-kids/{backend,frontend}:<version>` built with the repository
   as context, and only the tag in `deployment/base/apps/acampa-kids` is bumped (never `base/core`). With `--ci`
   no local build runs and the push starts no TeamCity build (`IpAlpha_AcampaKids_{Backend,Frontend}_Build` has no VCS trigger):
   the manual `IpAlpha_AcampaKids` Deploy builds the images. `./publish` never triggers an app's Deploy (apps keep their manual one). **Core is different:** the image tag bump `./publish` pushes to `deployment` master touches production paths, so TeamCity "Core — Deploy" (`IpAlpha_Core_Deploy`) starts on that push and a core service reaches production once its build is green (not triggered: `previews/**`, `releases/**`, apps' own overlays, tests, docs, `[core-release]`/`[preview-ci]` commits). `face-service` (`core/face-service`) is not published here: Core's
   TeamCity builds its image.

`-d` shows the plan and offers to apply exactly that plan · `-f <repo>` one repo · `--engine/--model` one-off
AI · `-y` no confirmation · `./publish clean` wipes the decision cache. Also `--resume`, `--npm-only`,
`--initialize`, `--deployment-path`, `--ci` (TeamCity builds images).

### Application AI gateway (cpamc)

`./run` starts `docker.io/kevynsax/cpamc-simulator:0.1.2` as development infrastructure on loopback port 8317 (automatically moved when busy), using Docker Compose or Apple `container`. It appears in `./run`, `./logs cpamc` and `./stop`; it is not a core repository, production service or application publish target. The public image supports Linux amd64 and arm64; after pulling, it needs no provider account, network access or management credentials. Local feature workspaces get their own simulator port with the usual offset, never a shared gateway.

`ipalpha ai gateway` configures the **application gateway**, not the coding CLI:

```sh
ipalpha ai gateway --mode simulator
ipalpha ai gateway --mode real --url https://your-cpamc.example/v1
```

Interactive real setup asks for a hidden client key and optional hidden management key. Headless setup takes `CPAMC_SETUP_CLIENT_KEY` and optional `CPAMC_SETUP_MANAGEMENT_KEY` from the environment, never secret command-line flags. HTTPS is required except on loopback; use an origin or `/v1` base without user info, query or fragment. Keys must differ. Management is optional: without it, subscriptions/quotas are unavailable but AI client calls work.

The default `ai_gateway=simulator` overlays `AI_BASE_URL`, `AI_API_KEY` and `CPAMC_MANAGEMENT_KEY` **only in ai-api's process environment**, using the resolved port and public dummy keys. Existing `.env` values are preserved; real keys are never forwarded to the simulator. Explicit gateway setup updates just those three keys in `core/ai-api/.env` (ignored, permission 0600) plus the non-secret `ai_gateway` mode in `.ipalpha/settings`. `./pull` and setup still add/fill only missing or blank values. Restart `./run` after changing mode. In real mode the selected gateway URL is preserved, even when it points at another local gateway.

Chat, images, classification and the nightly review now share cpamc. Jev uses native `POST /v1/decisions` with `AI_API_KEY`; only cpamc owns the OpenRouter key. Real cpamc needs an enabled OpenRouter-compatible provider with base `/api/v1`, API key and registered `typesafe/jev-1.13` model. `OPENROUTER_API_KEY` is no longer required or consumed by core ai-api; old local values are left untouched.

The simulator offers fake models/subscriptions/quotas, text/SSE, a limited JSON-schema fixture generator and 256 px placeholder PNGs. Jev returns deterministic canned answers (first offered choice, `noul: 0.9`, first score level `0`) with fake probabilities/confidence and `simulator: true` on every answer. State is ignored: use these only to exercise local positive flows, never real safety, permissions or content approval. Voice returns `501`. `SIMULATOR_SCENARIO` selects `healthy`, `low-quota`, `rate-limited`, `upstream-error`; restart to switch. No database or volume; no stored/logged prompts; no real judgments, quality guarantees or costs. It never bypasses church tokens, consent or external-app restrictions. Keep this image and dummy connection settings out of production manifests and application images. Developer guide: `/docs/ai-local-development`.

### AI configuration

`./ipalpha ai` (menu, tests the choice), `./ipalpha ai set <cli> [model]`, `./ipalpha ai test`.

Models come from the CLI: `pi --list-models`, `codex debug models`, `opencode models`, `grok models`,
`ollama list`; `claude`, `gemini` and `copilot` have no list command, so a known list is offered. The list
filters with `/`; "Other" still lets you type an id, and typing is the fallback when the list cannot be read.

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

## Sample data

Core services (and the Acampa Kids backend and Services Kids API) seed fictitious people, places, events and files at their first boot, so you
can click through the screens. It is a per-workspace choice, `seed=on|off` in `.ipalpha/settings`, asked **once** in
`setup` (right after the first sign-in step), in `./feature new <slug>` (a new feature copies main's value; main asks only
if it never chose; `--no-seed` makes that feature `off` without touching main) and in the preview request
(`./publish` in a feature asks only if the workspace never chose; the record carries `seedData: true|false` for
`deployment`). The default is **yes**; change it later with `ipalpha config set seed on|off` (which rewrites the keys at once).

The choice owns these keys in the `.env` of auth-api, persons-api, organizations-api, places-api, projects-api, schedule-api,
drive-api, photos-api, documents-api, sign-api, forms-api, services-kids-api and the Acampa Kids backend. **This is the one explicit exception to
"never overwrite a local value"**: setup, `./pull` and `config set seed` rewrite them whenever the choice differs, and blank
them when it is off (nothing is written while the workspace has not chosen):

| Key | On | Where |
| --- | --- | --- |
| `SEED_DATA` | `true` | all of the services above |
| `IPALPHA_ENV` | `dev` (previews use `preview:<slug>` in their configmaps) | all of the services above |
| `SEED_KEY` | 32 random bytes in hex, generated once per workspace (`.ipalpha/seed.env`), the same value everywhere | auth-api, organizations-api, projects-api, schedule-api, drive-api, photos-api, sign-api, forms-api, services-kids-api, Acampa Kids backend (not persons-api: it seeds in-process; a stale `SEED_KEY` in its `.env` is removed by setup/`./pull`; not documents-api or places-api) |

A service refuses to boot with `SEED_DATA=true` unless `IPALPHA_ENV` is `dev` or `preview*`; production never gets either
key. For the Acampa Kids backend, while seeding is on, setup also generates once (same values the app's seed sends to auth-api,
filled only where blank): `IPALPHA_CLIENT_ID` (`acampa-kids-<hex>`), `IPALPHA_CLIENT_SECRET`, `IPALPHA_SYSTEM_CLIENT_SECRET`
(64 hex characters each), plus `IPALPHA_SYSTEM_CLIENT_ID=acampa-kids` and `IPALPHA_DEVELOPERS_API_URL`. `SUPER_ADMIN_PERSON_IDS` is no longer written: the sample people have normal generated ids, and the coordination (Marina Duarte (exemplo), +55 11 90000-0011) works through her project role. Every service that takes `SEED_KEY` finds the sample people by phone through persons-api under the seed-actor token, so its `.env` also needs `AUTH_API_URL` and `PERSONS_API_URL` (both in the env-fallback files; schedule-api also `PROJECTS_API_URL`). Services Kids API (`SERVICES_KIDS_PROJECT_ID` may stay blank while seeding: its seed creates the `services-kids` project and keeps the id in its own `seedState`; with seed off you set it) finds the sample people by phone (guardians 0041-0044, volunteers 0045-0048) and needs `AUTH_API_URL`, `PROJECTS_API_URL` and optionally `PLACES_API_URL`. The first `./run` boot applies the seeds; setup
prints the sample phones once (all fictional, `@example.test`): core `+55 11 90000-0001` Ana and `0002` Bruno (stewards), `0003` Carla, `0005` Elisa, `0006` Fernanda (responsável), not verified; Acampa Kids `0011`..`0020` (Marina is the coordination), verified; Services Kids at Tamboré `0041`..`0048` (guardians and volunteers), verified.
`SEED_PERSON_IDS_JSON` and `SEED_OWNER_PERSON_ID` are retired and there are no fixed ids: `./pull` removes them from your `.env`.
The matching developer guide is `/docs/local-seeding` in the developer portal.

## `./pull`

Downloads the newest tool (checksum verified) and continues with it; fast-forwards every repo (a conflict
leaves that folder untouched and is listed at the end), clones missing ones, moves repos whose folder
changed (e.g. `apps/forms/` → `core/forms-*`, `apps/sign/` → `core/sign-*`, feature worktrees included), adds new `.env` keys, refreshes
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
| `seed` | `on` / `off`: sample data in this workspace (default yes); owns `SEED_DATA`, `IPALPHA_ENV`, `SEED_KEY` in the `.env` files |
| `tmux` | `on`: `./run` starts inside the workspace's tmux session (same as `./run --tmux`); off by default |

## Ports (defaults)

Feature workspaces shift every port by +100·k. Blocks where any port would land on a browser-restricted port (WHATWG
"bad ports", e.g. 6000, X11) are skipped, and busy-port replacements never pick one either: Firefox and Chrome refuse
those ("This address is restricted"), which broke the sign-in popup. A workspace created before this fix and sitting on
one (offset 900 gave auth-webapp 6000): set its `<service>_port` entries in `features/<slug>/.ipalpha/settings` to a free
block (or use `ipalpha config` > Ports inside the workspace), then `./stop` and `./run`.

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
| places-api | 3011 | RustFS (S3) | 9000 |
| photos-api | 3010 | Face service (optional) | 8000 |
| documents-api | 3012 | schedule-api | 3013 |
| sign-api | 3014 | sign-webapp | 5113 |
| drive-api | 3018 | drive-webapp | 5116 |
| services-photo-api | 3016 | services-photo-webapp | 5114 |
| apps-api | 3015 | services-kids-api | 3017 |
| virtual-assistant | 3019 | services-kids-webapp | 5115 |
| acampa-kids-backend | 3020 | acampa-kids-frontend | 5117 |

## Services Photo (app outside core)

`apps/services-photo/services-photo-api` (port **3016**, MongoDB `services_photo` on the shared local Mongo) and
`apps/services-photo/services-photo-webapp` (**5114**, Vite; `/api` goes to the API, WebSocket `/api/socket.io`
included). It is an app, not a core tool: it talks to core only over HTTP with the signed-in person's tokens.
Start it from the panel (or `./run apps set services-photo-webapp`). The API runs as a **single instance** (live
updates are in-process websockets).

`./pull` and `setup` write `services-photo-api/.env` from `.env.example` (else the embedded fallback), only adding
missing keys and filling blanks: `MONGO_URI`, `WEB_ORIGINS` /
`PUBLIC_WEB_URL` (`http://localhost:5114`), `AUTH_API_URL`, `AUTH_TOKEN_ISSUER`, `PROJECTS|PERSONS|SCHEDULE|PHOTOS|PLACES_API_URL`, and the optional `DISPATCH_API_URL` (`http://127.0.0.1:3007/api/dispatch`; empty turns the photos-push relay off).

Sign-in uses auth-api's login relay, so the app needs an **app-bound client with `login:relay`** and a relay
entry point. The tooling does not seed these (no local app id exists): register the app in the Developers portal /
Oikos, then paste `AUTH_CLIENT_ID`, `AUTH_CLIENT_SECRET`, `RELAY_CLIENT_ID` and `RELAY_ENTRY_POINT` into
`services-photo-api/.env`. Until then the API boots but sign-in answers unavailable.

New core env for this feature (added, never overwritten): schedule-api `PERSONS_API_URL`, `PERSONS_API_AUDIENCE=ipalpha:persons`
(and the system-client scope `persons:coordination`); persons-api `PROFILE_CALLER_SERVICE_IDS=auth-api`,
`COORDINATION_CALLER_SERVICE_IDS=schedule-api`. Previews are not built for Services Photo before launch.

The app used to be called Photo Services (`apps/photo-services/photo-services-*`). `./pull` moves those folders once to
`apps/services-photo/services-photo-*`, together with their `features/<slug>/` worktrees (branches, `.env` and uncommitted
work stay; it only renames, and does nothing but warn when the new folder already exists). Rename
`PHOTO_SERVICES_API_URL` to `SERVICES_PHOTO_API_URL` in a webapp `.env` by hand if you set it; the Mongo database is now `services_photo`.

## Services Kids (app outside core)

`apps/services-kids/services-kids-api` (port **3017**, MongoDB `services_kids`) and
`apps/services-kids/services-kids-webapp` (**5115**, Vite; `/api` goes to the API via `SERVICES_KIDS_API_URL`, default
`http://localhost:3017`). Kids ministry Sunday service (design: `.ipalpha/design/services-kids.md`); an app outside core,
HTTP with the signed-in person's tokens, its own socket.io (no `dispatch-api` origin needed). Start it from the panel or
`./run apps set services-kids-webapp`. Single API instance.

The repositories have an origin, so `./pull`, `setup` and `./feature` clone them like every other repo.

`./pull` and `setup` write `services-kids-api/.env` from `.env.example` (else the embedded fallback), only adding
missing keys and filling blanks: `VAPID_PUBLIC_KEY` / `VAPID_PRIVATE_KEY` (one Web Push P-256
pair, base64url, generated once and kept), `VAPID_SUBJECT=mailto:dev@example.test`, `MONGO_URI`, `WEB_ORIGINS` /
`PUBLIC_WEB_URL` (`http://localhost:5115`), `COOKIE_SECURE=false`, `AUTH_API_URL`,
`PROJECTS|PERSONS|PLACES|SCHEDULE|NOTIFICATIONS_API_URL`, `WHATSAPP_ENABLED=false`.

As with Services Photo, register the app in the Developers portal / Oikos and paste `AUTH_CLIENT_ID`, `AUTH_CLIENT_SECRET`
(app-bound, `login:relay` + `notifications:send-template`), `RELAY_CLIENT_ID`, `RELAY_ENTRY_POINT` and `OAUTH_CLIENT_ID`
into `services-kids-api/.env`; the tooling does not seed them.

## Acampa Kids (app outside core)

`apps/acampa-kids/backend` (**Acampa Kids API**, port **3020**, Bun + Hono, MongoDB `camping` on the shared local Mongo,
`GET /live` + `/ready`) and `apps/acampa-kids/frontend` (**Acampa Kids Web**, **5117**, Vite) appear under "Apps" in `./run`
and `./status`, like Services Photo. The repositories stay optional: without access the rows say why they cannot start.
The backend runs with **Bun** (`bun run dev`): install it from https://bun.sh; `./run` installs the dependencies with
`bun install` (the frontend with npm) and, without Bun, shows the row as unable to start. The frontend's `/api` proxy
points to this run's backend (`IPALPHA_WEB_PROXY_API`, applied by `web-dev.mjs`; the app's own `vite.config.ts` is not touched).
Start the pages from the panel or `./run apps set acampa-kids-frontend`.

`./pull` and `setup` write `apps/acampa-kids/backend/.env` from `.env.example`, only adding missing keys and filling blanks
(never overwriting your values): the ./run Mongo (`MONGODB_URI`, `MONGODB_DB=camping`), `PORT`, `CORS_ORIGIN` / `APP_URL` /
`IPALPHA_REDIRECT_URI` for the frontend, every `IPALPHA_*_API_URL` of the local core, `IPALPHA_AUTH_ORIGIN`, `IPALPHA_TOKEN_ISSUER`,
`IPALPHA_ENTRY_POINT=login`, `AI_API_URL`, and the app's own secrets `SESSION_TOKEN_KEY` and `WORKER_SECRET`. With sample data on (see "Sample data") setup also writes the registration secrets and the seed values and the app registers
itself at boot. With it off, the values that come from registering the app in core are **not** written (auth-api refuses to boot on
a client bound to an unknown app): register Acampa in the Developers portal / Oikos and paste `IPALPHA_CLIENT_ID`,
`IPALPHA_CLIENT_SECRET`, `IPALPHA_SYSTEM_CLIENT_ID`, `IPALPHA_SYSTEM_CLIENT_SECRET`, `IPALPHA_PROJECT_ID` and `IPALPHA_APP_ID`.
Until then the API boots and "Entrar com IPAlpha" answers unavailable (`GET /api/auth/ipalpha/config` is not enabled).

## Core and apps

`core/` holds the shared capabilities, the core UIs and the core tools Forms (`core/forms-api`,
`core/forms-webapp`) and IPAlpha Sign (`core/sign-api`, `core/sign-webapp`); apps that consume core live in
`apps/<app>/`. **Acampa Kids** (`apps/acampa-kids/{backend,frontend}`; its former face service is now `core/face-service`)
has its own repositories, registry and TeamCity project; `./run` starts its backend and frontend (see "Acampa Kids") and its repos are optional.
In a feature environment every core service runs; an app joins only when one of its own repos changed.

## Environments picker: `./envs`

One terminal screen over every IPAlpha environment on this computer: the main workspace and each
`features/<slug>`. Per row: `./run` state (services up/total and the Oikos URL, the assigned one when ports moved),
whether the tmux session `ipalpha-<slug>` (`ipalpha-main` for the main workspace) exists, the agents (claude, pi, codex,
grok) whose working folder is inside it, uncommitted files and commits ahead of the default branch (all repositories),
the branches checked out, and the open warden requests filed `from: <slug>` (skipped when `~/src/ipalpha-qa` is missing;
override with `IPALPHA_QA_DIR`). It works from the main workspace or from any `features/<slug>`.

```sh
./envs                 # picker: arrows, type to filter (name, branch, agent)
./envs forms           # hop straight into features/forms (./envs main for the main workspace)
./envs --list          # plain table for scripts (also what a non-interactive terminal gets)
```

Keys: `Enter` hop, `Ctrl+T` open a shell window in that workspace, `Ctrl+L` open `./logs` in a `logs` window,
`Ctrl+R` start `./run` in a `run` window, `F5` refresh (it also refreshes every few seconds), `Esc` clears the
filter, then quits. Hop is `tmux switch-client` inside tmux and `tmux attach` outside; a missing session is created
(`tmux new -d -s ipalpha-<slug> -c <workspace>`). Without tmux (`brew install tmux`) the list still works.

To reach the `./run` panel from the picker, run it with `./run --tmux` (or set `tmux=on` in `.ipalpha/settings`, which is
local and ignored by git): outside tmux it restarts itself in the `run` window of the workspace session and attaches
there; inside tmux, or when tmux is missing, it runs where it is. Off by default.

## Feature environments: `./feature`

One feature = one isolated workspace, one `feat/<slug>` branch per repo, one preview namespace with its own
data and public HTTPS hosts, alive 72 h after each publish. 

```sh
./feature new hello-preview          # features/hello-preview/: worktrees on feat/hello-preview at each repo's local default-branch tip
                                     # (--no-seed: no sample data in this feature; otherwise main's seed choice, asked once if unset)
cd features/hello-preview
./run                                # own ports (+100 per feature) and own infra
./publish                            # = --feature hello-preview: commit (no version bump), push, deploy the preview
cd ../..
./feature list                       # previews + local features, generation, expiry, URLs
./feature extend hello-preview       # +72 h without a build
./feature rebase hello-preview       # move onto each repo's local default-branch tip
./feature reset hello-preview        # back to the synthetic seed (type the slug to confirm)
./feature destroy hello-preview      # delete the preview and worktrees; branches stay
```

`./feature new` starts every repo's `feat/<slug>` from that repo's **local** default-branch tip (`master`, or
`main` where that is the default), not from the last green Core Deploy; an existing `feat/<slug>` branch is reused as is.
`./feature rebase` rebases onto the same local tips. Pull the repos first (`./pull`) if you want upstream work. Slugs: `^[a-z0-9-]{3,30}$`, no hyphen at
either end, every host ≤ 63 chars.

| URL | What |
| --- | --- |
| `https://oikos.<slug>.ipalpha.dev` | Oikos |
| `https://forms.<slug>.ipalpha.dev` | IPAlpha Formulários |
| `https://auth.<slug>.ipalpha.dev` | sign-in popup |
| `https://developers.<slug>.ipalpha.dev` | IPAlpha Developers |
| `https://oikos.<slug>.ipalpha.dev/mailbox` | captured e-mail/SMS (sign-in codes) |
| `https://acampa-kids.<slug>.ipalpha.dev` | Acampa Kids — only when an `acampa-kids-*` repo changed |
| `https://sign.<slug>.ipalpha.dev` | IPAlpha Sign (core tool, always served) |

How it works: main checkouts are never touched (`git worktree`); unchanged services run the last green Core Deploy images
(`deployment/releases/core-latest.json`, resolved by CI, not by `./feature`); `shared-js` changes rebuild every core API
(each web app owns its UI kit, so no library rebuilds them). `./publish` pushes `previews/<slug>/release.json` to deployment master (a throwaway worktree, retried
on races, CI fields kept) and waits for CI's record commit — no CI token on laptops. Pushes use
`--force-with-lease` against the tip this workspace pushed, so a teammate's push is never overwritten.
`.env` files are kept out of commits. Sign in with the fixture accounts in
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
