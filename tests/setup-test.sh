#!/usr/bin/env bash
set -euo pipefail

ipalpha_repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
ipalpha_tmp="$(mktemp -d)"
trap 'rm -rf "$ipalpha_tmp"' EXIT

export IPALPHA_TEST_NO_PORT_PROBE=1
export IPALPHA_CLONE_COMMAND="$ipalpha_repo_root/tests/fake-clone"
export IPALPHA_LANG=en-US
export IPALPHA_SKIP_TOOLS=1
export IPALPHA_TARGET_DIR="$ipalpha_tmp/IpAlpha"
export IPALPHA_SKIP_INSTALL=1
export IPALPHA_NO_SHELL=1
export IPALPHA_SUPERUSER_PHONE=99900000000

ipalpha_fail() { echo "FAIL: $1" >&2; exit 1; }

inplace() {
  if sed --version >/dev/null 2>&1; then
    sed -i "$@"
  else
    sed -i '' "$@"
  fi
}

echo "== setup (skip tools)"
ipalpha_out="$("$ipalpha_repo_root/setup" --skip-tools --keep-setup 2>&1)" \
  || ipalpha_fail "setup failed: $ipalpha_out"

for repo in shared-js shared-ui projects-api persons-api organizations-api notifications-api auth-api auth-webapp \
  ai-api developers-api developers-webapp; do
  [[ -d "$ipalpha_tmp/IpAlpha/core/$repo/.git" ]] \
    || ipalpha_fail "ms repo not cloned: $repo"
done
[[ ! -f "$ipalpha_tmp/IpAlpha/core/auth-webapp/.env" ]] || ipalpha_fail "auth-webapp must not get an env"
[[ ! -f "$ipalpha_tmp/IpAlpha/core/developers-webapp/.env" ]] || ipalpha_fail "developers-webapp must not get an env"
grep -q '^auth-webapp_port=5100$' "$ipalpha_tmp/IpAlpha/.ipalpha/settings" || ipalpha_fail "settings missing auth-webapp_port"
grep -q '^developers-webapp_port=5111$' "$ipalpha_tmp/IpAlpha/.ipalpha/settings" || ipalpha_fail "settings missing developers-webapp_port"
grep -q '^ai-api_port=3008$' "$ipalpha_tmp/IpAlpha/.ipalpha/settings" || ipalpha_fail "settings missing ai-api_port"
grep -q '^developers-api_port=3009$' "$ipalpha_tmp/IpAlpha/.ipalpha/settings" || ipalpha_fail "settings missing developers-api_port"
[[ -x "$ipalpha_tmp/IpAlpha/.ipalpha/bin/web-dev" ]] || ipalpha_fail "web-dev missing"
# fake clone has no package.json -> web app is skipped from the runners until cloned for real
grep -q '"name": "auth-webapp"' "$ipalpha_tmp/IpAlpha/.ipalpha/projects.json" && ipalpha_fail "projects.json listed auth-webapp without package.json"
echo '{"name":"auth-webapp"}' >"$ipalpha_tmp/IpAlpha/core/auth-webapp/package.json"
echo '{"name":"developers-webapp"}' >"$ipalpha_tmp/IpAlpha/core/developers-webapp/package.json"
[[ -d "$ipalpha_tmp/IpAlpha/deployment/.git" ]] || ipalpha_fail "deployment not cloned"
[[ ! -e "$ipalpha_tmp/IpAlpha/develop" ]] || ipalpha_fail "develop must not be cloned"
[[ ! -e "$ipalpha_tmp/IpAlpha/.github" ]] || ipalpha_fail ".github must not be cloned into the workspace"
[[ -x "$ipalpha_tmp/IpAlpha/set-keys" ]] || ipalpha_fail "./set-keys missing"

for repo in projects-api persons-api organizations-api notifications-api auth-api ai-api developers-api; do
  [[ -f "$ipalpha_tmp/IpAlpha/core/$repo/.env" ]] || ipalpha_fail "env missing: $repo"
done
grep -q '^PORT=3008$' "$ipalpha_tmp/IpAlpha/core/ai-api/.env" || ipalpha_fail "ai-api env wrong PORT"
grep -q '^AI_API_KEY=$' "$ipalpha_tmp/IpAlpha/core/ai-api/.env" || ipalpha_fail "ai-api env must keep AI_API_KEY blank"
grep -q '^PORT=3009$' "$ipalpha_tmp/IpAlpha/core/developers-api/.env" || ipalpha_fail "developers-api env wrong PORT"
grep -q '^AUTH_CLIENT_ID=developers-api$' "$ipalpha_tmp/IpAlpha/core/developers-api/.env" || ipalpha_fail "developers-api system client not seeded"
grep -q '"clientId":"ai-api"' "$ipalpha_tmp/IpAlpha/core/auth-api/.env" || ipalpha_fail "auth-api SEED_CLIENTS_JSON lacks ai-api"
grep -q '^SOCKET_ALLOWED_ORIGINS=.*http://localhost:5111' "$ipalpha_tmp/IpAlpha/core/dispatch-api/.env" \
  || ipalpha_fail "dispatch-api must allow the local developers-webapp origin"
grep -q '^BUILTIN_DEVELOPERS_ORIGINS=http://localhost:5111$' "$ipalpha_tmp/IpAlpha/core/auth-api/.env" \
  || ipalpha_fail "auth-api must allow the local developers-webapp origin"
grep -q '^SUPERUSER_NAME=Joao Silva Costa$' "$ipalpha_tmp/IpAlpha/core/auth-api/.env" || ipalpha_fail "default superuser name missing"
grep -q '^SUPERUSER_PHONE=+5599900000000$' "$ipalpha_tmp/IpAlpha/core/auth-api/.env" || ipalpha_fail "superuser phone must be normalized"
grep -q '^SMSBARATO_KEY=$' "$ipalpha_tmp/IpAlpha/core/notifications-api/.env" || ipalpha_fail "notifications-api env missing SMSBARATO_KEY"
grep -q '^MAIL_PROVIDER=mailpit$' "$ipalpha_tmp/IpAlpha/core/notifications-api/.env" || ipalpha_fail "local email must use Mailpit"
grep -q '^SMS_PROVIDER=mailpit$' "$ipalpha_tmp/IpAlpha/core/notifications-api/.env" || ipalpha_fail "local SMS must use Mailpit"
grep -q '^DEPLOYMENT_ENVIRONMENT=development$' "$ipalpha_tmp/IpAlpha/core/notifications-api/.env" || ipalpha_fail "Mailpit needs development environment"
grep -q '^MAILPIT_URL=http://127.0.0.1:8025$' "$ipalpha_tmp/IpAlpha/core/notifications-api/.env" || ipalpha_fail "local inbox URL missing"
grep -q '^PORT=3001$' "$ipalpha_tmp/IpAlpha/core/projects-api/.env" || ipalpha_fail "projects-api env wrong PORT"
grep -q '^AUTH_API_URL=http://127.0.0.1:3005$' "$ipalpha_tmp/IpAlpha/core/persons-api/.env" || ipalpha_fail "persons-api env wrong AUTH_API_URL"
[[ ! -f "$ipalpha_tmp/IpAlpha/core/shared-js/.env" ]] || ipalpha_fail "shared-js must not get an env"
[[ ! -f "$ipalpha_tmp/IpAlpha/core/shared-ui/.env" ]] || ipalpha_fail "shared-ui must not get an env"

[[ -x "$ipalpha_tmp/IpAlpha/run" ]] || ipalpha_fail "./run missing"
[[ -x "$ipalpha_tmp/IpAlpha/publish" ]] || ipalpha_fail "./publish missing"
[[ -x "$ipalpha_tmp/IpAlpha/pull" ]] || ipalpha_fail "./pull missing"
[[ -f "$ipalpha_tmp/IpAlpha/.ipalpha/settings" ]] || ipalpha_fail "settings missing"

for key in lang org runtime ai_cli ai_model runner mongo_port redis_port rabbitmq_port rabbitmq_mgmt_port projects-api_port persons-api_port organizations-api_port notifications-api_port auth-api_port; do
  grep -q "^${key}=" "$ipalpha_tmp/IpAlpha/.ipalpha/settings" || ipalpha_fail "settings missing key: $key"
done
grep -q '^lang=en-US' "$ipalpha_tmp/IpAlpha/.ipalpha/settings" || ipalpha_fail "settings lang wrong"
grep -q '^org=ipalpha' "$ipalpha_tmp/IpAlpha/.ipalpha/settings" || ipalpha_fail "settings org wrong"
grep -q '^ai_cli=pi' "$ipalpha_tmp/IpAlpha/.ipalpha/settings" || ipalpha_fail "settings ai_cli wrong"
grep -q '^ai_model=cpamc/muse-spark-1.3-contributor' "$ipalpha_tmp/IpAlpha/.ipalpha/settings" || ipalpha_fail "settings ai_model wrong"
grep -q '^runner=auto$' "$ipalpha_tmp/IpAlpha/.ipalpha/settings" || ipalpha_fail "settings runner must default to auto"
grep -q '^browser_apps=auth-webapp mordomia-webapp mailpit$' "$ipalpha_tmp/IpAlpha/.ipalpha/settings" || ipalpha_fail "default browser selection wrong"
[[ -f "$ipalpha_tmp/IpAlpha/.ipalpha/bin/browser-dev.mjs" ]] || ipalpha_fail "browser helper missing"
grep -q 'browser-dev.mjs.*watch' "$ipalpha_tmp/IpAlpha/run" || ipalpha_fail "run must open selected browsers"
grep -q 'IPALPHA_RUNNER:-${ipalpha_runner:-auto}' "$ipalpha_tmp/IpAlpha/run" || ipalpha_fail "./run must default to auto"

echo "== port-busy check ignores our own containers"
# Fake a docker CLI that pretends an ipalpha container is holding port 27017.
ipalpha_fakebin="$(mktemp -d)"
cat >"$ipalpha_fakebin/docker" <<SH
#!/usr/bin/env bash
case "\$1" in
  info) exit 0 ;;
  ps)
    # Only the -aq form is consumed by lib/ports.sh.
    if [[ " \$* " == *" -aq "* ]]; then
      echo "fakecontainerid"
    fi
    ;;
  port)
    # docker port <id> outputs lines like "27017/tcp -> 0.0.0.0:27017"
    echo "27017/tcp -> 0.0.0.0:27017"
    ;;
esac
SH
chmod +x "$ipalpha_fakebin/docker"
# shellcheck disable=SC1091
PATH="$ipalpha_fakebin:$PATH" source "$ipalpha_repo_root/lib/ports.sh"
PATH="$ipalpha_fakebin:$PATH" ipalpha_owns_port 27017   || ipalpha_fail "ipalpha_owns_port must recognise our own container on 27017"
PATH="$ipalpha_fakebin:$PATH" ipalpha_owns_port 5432   && ipalpha_fail "ipalpha_owns_port must not claim a port our container doesn\'t hold"
rm -rf "$ipalpha_fakebin"

[[ -f "$ipalpha_tmp/IpAlpha/.ipalpha/compose.yaml" ]] || ipalpha_fail "compose.yaml missing"
grep -q 'axllent/mailpit:v1.31.3' "$ipalpha_tmp/IpAlpha/.ipalpha/compose.yaml" || ipalpha_fail "Compose inbox missing"
grep -q '127.0.0.1:${MAILPIT_HOST_PORT:-8025}:8025' "$ipalpha_tmp/IpAlpha/.ipalpha/compose.yaml" || ipalpha_fail "inbox must be loopback-only"
grep -q '^MAILPIT_HOST_PORT=8025$' "$ipalpha_tmp/IpAlpha/.ipalpha/ports.env" || ipalpha_fail "inbox port missing"
grep -q '^mailpit_port=8025$' "$ipalpha_tmp/IpAlpha/.ipalpha/settings" || ipalpha_fail "inbox setting missing"
for script in infra-up infra-down infra-logs; do
  grep -q '\$infra-mailpit' "$ipalpha_tmp/IpAlpha/.ipalpha/bin/$script" || ipalpha_fail "$script omits Mailpit"
done
[[ -f "$ipalpha_tmp/IpAlpha/.ipalpha/ports.env" ]] || ipalpha_fail "ports.env missing"
[[ -f "$ipalpha_tmp/IpAlpha/.ipalpha/mprocs.yaml" ]] || ipalpha_fail "mprocs.yaml missing"
[[ -d "$ipalpha_tmp/IpAlpha/.ipalpha/lib" ]] || ipalpha_fail ".ipalpha/lib missing"
[[ -d "$ipalpha_tmp/IpAlpha/.ipalpha/env-fallback" ]] || ipalpha_fail ".ipalpha/env-fallback missing"
for bin in infra-up infra-down wait-for-http node-dev web-dev install-deps fallback-run; do
  [[ -x "$ipalpha_tmp/IpAlpha/.ipalpha/bin/$bin" ]] || ipalpha_fail "bin/$bin missing"
  bash -n "$ipalpha_tmp/IpAlpha/.ipalpha/bin/$bin" || ipalpha_fail "bin/$bin syntax error"
done

ipalpha_order="$(grep -E '^  "MS · ' "$ipalpha_tmp/IpAlpha/.ipalpha/mprocs.yaml" | sed 's/.*MS · //; s/":$//')"
[[ "$ipalpha_order" == "projects-api
persons-api
organizations-api
notifications-api
auth-api
forms-api
ai-api
developers-api
dispatch-api" ]] || ipalpha_fail "mprocs order wrong: $ipalpha_order"
grep -q '"name": "developers-api", "kind": "service", "group": "", "path": "core/developers-api", "display": "Developers", "port": "3009"' \
  "$ipalpha_tmp/IpAlpha/.ipalpha/projects.json" || ipalpha_fail "projects.json developers-api entry wrong"
grep -q '"name": "forms-api", "kind": "service", "group": "forms", "path": "apps/forms/forms-api", "display": "Forms API"' \
  "$ipalpha_tmp/IpAlpha/.ipalpha/projects.json" || ipalpha_fail "projects.json must list forms-api in the forms app group"
grep -q '"name": "ai-api", "kind": "service", "group": "", "path": "core/ai-api", "display": "AI", "port": "3008"' \
  "$ipalpha_tmp/IpAlpha/.ipalpha/projects.json" || ipalpha_fail "projects.json ai-api entry wrong"
grep -q 'export AI_API_URL="$(ms_url ai-api)" DEVELOPERS_API_URL="$(ms_url developers-api)"' "$ipalpha_tmp/IpAlpha/.ipalpha/bin/web-dev" \
  || ipalpha_fail "web-dev must export AI_API_URL and DEVELOPERS_API_URL for the /api proxies"

echo "== localized help"
help_run="$("$ipalpha_tmp/IpAlpha/run" --help 2>&1)" || true
grep -qi 'Usage: ./run' <<<"$help_run" || ipalpha_fail "run --help wrong: $help_run"
help_pull="$("$ipalpha_tmp/IpAlpha/pull" --help 2>&1)" || true
grep -qi 'Usage: ./pull' <<<"$help_pull" || ipalpha_fail "pull --help wrong: $help_pull"
help_pub="$("$ipalpha_tmp/IpAlpha/publish" --help 2>&1)" || true
grep -qi 'Usage: ./publish' <<<"$help_pub" || ipalpha_fail "publish --help wrong: $help_pub"

echo "== localized help (pt-BR)"
inplace 's/^lang=en-US/lang=pt-BR/' "$ipalpha_tmp/IpAlpha/.ipalpha/settings"
help_run="$("$ipalpha_tmp/IpAlpha/run" --help 2>&1)" || true
grep -qi 'Uso: ./run' <<<"$help_run" || ipalpha_fail "run --help pt-BR wrong: $help_run"
inplace 's/^lang=pt-BR/lang=en-US/' "$ipalpha_tmp/IpAlpha/.ipalpha/settings"

echo "== idempotent second run keeps local env and merges new keys"
echo "LOCAL_CUSTOMIZATION=1" >>"$ipalpha_tmp/IpAlpha/core/projects-api/.env"
cat >"$ipalpha_tmp/IpAlpha/core/persons-api/.env.example" <<'EOF'
PORT=3002
NEW_KEY=42
EOF
ipalpha_out="$("$ipalpha_repo_root/setup" --skip-tools --keep-setup 2>&1)" \
  || ipalpha_fail "second setup failed: $ipalpha_out"
grep -q 'LOCAL_CUSTOMIZATION=1' "$ipalpha_tmp/IpAlpha/core/projects-api/.env" \
  || ipalpha_fail "second run overwrote local env"
grep -q '^NEW_KEY=42$' "$ipalpha_tmp/IpAlpha/core/persons-api/.env" \
  || ipalpha_fail "second run did not merge new keys"
grep -q '"name": "auth-webapp", "kind": "app"' "$ipalpha_tmp/IpAlpha/.ipalpha/projects.json" || ipalpha_fail "projects.json missing auth-webapp app"
grep -q 'Web · auth-webapp' "$ipalpha_tmp/IpAlpha/.ipalpha/mprocs.yaml" || ipalpha_fail "mprocs missing auth-webapp"
grep -q 'web-dev auth-webapp' "$ipalpha_tmp/IpAlpha/.ipalpha/projects.json" || ipalpha_fail "projects.json auth-webapp cmd wrong"
grep -q '"name": "developers-webapp", "kind": "app", "group": "", "path": "core/developers-webapp", "display": "Developers Web", "port": "5111"' \
  "$ipalpha_tmp/IpAlpha/.ipalpha/projects.json" || ipalpha_fail "projects.json missing developers-webapp app on 5111"
grep -q 'Web · developers-webapp' "$ipalpha_tmp/IpAlpha/.ipalpha/mprocs.yaml" || ipalpha_fail "mprocs missing developers-webapp"
echo "== web-dev proxies developers-webapp to the local APIs"
ipalpha_fakevite="$ipalpha_tmp/IpAlpha/core/developers-webapp/node_modules/vite"
mkdir -p "$ipalpha_fakevite"
printf '{"type":"module"}\n' >"$ipalpha_fakevite/package.json"
mkdir -p "$ipalpha_fakevite/dist/node"
printf 'export function loadEnv(){return {}}; export async function createServer(config){ return {listen(){console.log("vite --port "+config.server.port+" --strictPort auth="+process.env.AUTH_API_URL+" developers="+process.env.DEVELOPERS_API_URL+" dispatch="+process.env.DISPATCH_API_URL+" projects="+process.env.PROJECTS_API_URL+" ai="+process.env.AI_API_URL)},printUrls(){},bindCLIShortcuts(){}} }\n' >"$ipalpha_fakevite/dist/node/index.js"
touch "$ipalpha_tmp/IpAlpha/core/developers-webapp/node_modules"
ipalpha_out="$("$ipalpha_tmp/IpAlpha/.ipalpha/bin/web-dev" developers-webapp 2>&1)" || ipalpha_fail "web-dev developers-webapp failed: $ipalpha_out"
[[ "$ipalpha_out" == "vite --port 5111 --strictPort auth=http://127.0.0.1:3005 developers=http://127.0.0.1:3009 dispatch=http://127.0.0.1:3007 projects=http://127.0.0.1:3001 ai=http://127.0.0.1:3008" ]] \
  || ipalpha_fail "web-dev developers-webapp env wrong: $ipalpha_out"

echo "== pull re-clones missing repo"
rm -rf "$ipalpha_tmp/IpAlpha/core/persons-api"
ipalpha_out="$(
  cd "$ipalpha_tmp/IpAlpha" && IPALPHA_CLONE_COMMAND="$ipalpha_repo_root/tests/fake-clone" ./pull 2>&1
)" || ipalpha_fail "pull failed: $ipalpha_out"
[[ -d "$ipalpha_tmp/IpAlpha/core/persons-api/.git" ]] || ipalpha_fail "pull did not re-clone persons-api"
[[ -f "$ipalpha_tmp/IpAlpha/core/persons-api/.env" ]] || ipalpha_fail "pull did not reinstall env"

echo "== publish lists the new core repos"
for repo in ai-api developers-api developers-webapp; do
  repo_dir="$ipalpha_tmp/IpAlpha/core/$repo"
  rm -rf "$repo_dir/.git"
  git init -q -b master "$repo_dir"
  git -C "$repo_dir" -c user.name=test -c user.email=test@example.invalid commit -q --allow-empty -m "root"
done
# shellcheck disable=SC1091
ipalpha_dirty="$(source "$ipalpha_repo_root/lib/common.sh" && source "$ipalpha_repo_root/lib/publish.sh" \
  && ipalpha_publish_dirty_repos "$ipalpha_tmp/IpAlpha")"
for repo in ai-api developers-api developers-webapp; do
  grep -qx "$repo" <<<"$ipalpha_dirty" || ipalpha_fail "publish does not offer untagged $repo: $ipalpha_dirty"
done
for repo in ai-api developers-api developers-webapp; do rm -rf "$ipalpha_tmp/IpAlpha/core/$repo/.git"; mkdir -p "$ipalpha_tmp/IpAlpha/core/$repo/.git"; done

echo "== publish smoke (no AI, dry-run aborts without changes)"
repo_dir="$ipalpha_tmp/IpAlpha/core/projects-api"
git init -q -b master "$repo_dir"
git -C "$repo_dir" -c user.name=test -c user.email=test@example.invalid commit -q --allow-empty -m "root"
git init -q --bare "$ipalpha_tmp/origin/projects-api.git"
git -C "$repo_dir" remote add origin "$ipalpha_tmp/origin/projects-api.git"
printf '{\n  "name": "projects-api",\n  "version": "0.0.0",\n  "scripts": { "start:dev": "node -e \\"console.log(1)\\"" }\n}\n' >"$repo_dir/package.json"
echo "dirty" >"$repo_dir/notes.txt"
inplace 's/^ai_cli=.*/ai_cli=bogus/' "$ipalpha_tmp/IpAlpha/.ipalpha/settings"
ipalpha_out="$(
  cd "$ipalpha_tmp/IpAlpha" && ./publish -d -f projects-api </dev/null 2>&1
)" || ipalpha_fail "publish dry-run failed: $ipalpha_out"
grep -qi 'aborted' <<<"$ipalpha_out" || ipalpha_fail "dry-run did not abort: $ipalpha_out"
[[ -n "$(git -C "$repo_dir" status --porcelain)" ]] || ipalpha_fail "dry-run changed the repo"

echo "== publish sees committed-but-untagged work as publishable"
git -C "$repo_dir" -c user.name=test -c user.email=test@example.invalid add -A
git -C "$repo_dir" -c user.name=test -c user.email=test@example.invalid commit -q -m "notes"
git -C "$repo_dir" tag v0.0.0
ipalpha_out="$(cd "$ipalpha_tmp/IpAlpha" && ./publish -d -f projects-api </dev/null 2>&1)" || true
grep -qi 'Nothing to publish' <<<"$ipalpha_out" || ipalpha_fail "tagged clean repo should have nothing to publish: $ipalpha_out"
echo "// more" >>"$repo_dir/index.js"; git -C "$repo_dir" add -A
git -C "$repo_dir" -c user.name=test -c user.email=test@example.invalid commit -qam "more notes"
ipalpha_out="$(cd "$ipalpha_tmp/IpAlpha" && ./publish -d -f projects-api </dev/null 2>&1)" || ipalpha_fail "publish dry-run (untagged commit) failed: $ipalpha_out"
grep -qi 'aborted' <<<"$ipalpha_out" || ipalpha_fail "untagged commit was not offered for publish: $ipalpha_out"

ipalpha_out="$(
  cd "$ipalpha_tmp/IpAlpha" && ./publish -f projects-api </dev/null 2>&1
)" || ipalpha_fail "publish failed: $ipalpha_out"
[[ -z "$(git -C "$repo_dir" status --porcelain)" ]] || ipalpha_fail "publish left dirty repo"
git -C "$repo_dir" log --format=%s -1 | grep -q 'Update projects-api' \
  || ipalpha_fail "publish commit message wrong: $(git -C "$repo_dir" log --format=%s -1)"
[[ -z "$(git -C "$repo_dir" tag | grep '^v0\.0\.1$')" ]] || ipalpha_fail "publish must not tag image repos (CI tags after deploy)"
node -p 'require(process.argv[1]).version' "$repo_dir/package.json" | grep -q '^0\.0\.1$' \
  || ipalpha_fail "publish version not bumped"

echo "== publish clean wipes cache"
[[ -d "$ipalpha_tmp/IpAlpha/.ipalpha/.publish-cache" ]] || ipalpha_fail "publish cache missing"
ipalpha_out="$(
  cd "$ipalpha_tmp/IpAlpha" && ./publish clean </dev/null 2>&1
)" || ipalpha_fail "publish clean failed"
[[ ! -d "$ipalpha_tmp/IpAlpha/.ipalpha/.publish-cache" ]] || ipalpha_fail "publish cache not wiped"

echo "== setup deletes its own .github clone"
ipalpha_copy="$ipalpha_tmp/copy/.github"
mkdir -p "$ipalpha_tmp/copy"
cp -R "$ipalpha_repo_root" "$ipalpha_copy"
IPALPHA_TARGET_DIR="$ipalpha_tmp/IpAlpha2" "$ipalpha_copy/setup" --skip-tools >/dev/null 2>&1 \
  || ipalpha_fail "setup from a .github copy failed"
[[ ! -e "$ipalpha_copy" ]] || ipalpha_fail "setup did not delete its .github clone"
[[ -x "$ipalpha_tmp/IpAlpha2/run" ]] || ipalpha_fail "setup from copy did not generate ./run"
grep -q '^infra_name=ipalpha$' "$ipalpha_tmp/IpAlpha/.ipalpha/settings" || ipalpha_fail "IpAlpha must keep infra_name=ipalpha"
grep -q '^infra_name=ipalpha2$' "$ipalpha_tmp/IpAlpha2/.ipalpha/settings" || ipalpha_fail "a second workspace needs its own infra_name"

echo "== re-running setup keeps a second workspace off the first one's infra"
sed -i.bak 's/^infra_name=.*/infra_name=ipalpha/' "$ipalpha_tmp/IpAlpha2/.ipalpha/settings"
mkdir -p "$ipalpha_tmp/copy" && cp -R "$ipalpha_repo_root" "$ipalpha_copy"
IPALPHA_TARGET_DIR="$ipalpha_tmp/IpAlpha2" "$ipalpha_copy/setup" --skip-tools >/dev/null 2>&1 \
  || ipalpha_fail "setup re-run failed"
grep -q '^infra_name=ipalpha2$' "$ipalpha_tmp/IpAlpha2/.ipalpha/settings" || ipalpha_fail "setup re-run put the workspace back on the shared infra"
grep -q '^IPALPHA_INFRA_NAME=ipalpha2$' "$ipalpha_tmp/IpAlpha2/.ipalpha/ports.env" || ipalpha_fail "ports.env infra name not updated"

echo "== runtime detection prefers Docker with Compose v2, then Apple container"
ipalpha_fake="$ipalpha_tmp/fake-runtime"
mkdir -p "$ipalpha_fake/both" "$ipalpha_fake/cli-only"
printf '#!/bin/sh\nexit 0\n' >"$ipalpha_fake/both/docker"
printf '#!/bin/sh\n[ "$1" = compose ] && exit 1\nexit 0\n' >"$ipalpha_fake/cli-only/docker"
for dir in both cli-only; do printf '#!/bin/sh\nexit 0\n' >"$ipalpha_fake/$dir/container"; done
chmod +x "$ipalpha_fake"/*/*
for case in "both docker" "cli-only container"; do
  set -- $case
  got="$(PATH="$ipalpha_fake/$1:/usr/bin:/bin" bash -c 'source "$0/lib/tools.sh"; ipalpha_detect_runtime; echo "$ipalpha_runtime"' "$ipalpha_repo_root")"
  [[ "$got" == "$2" ]] || ipalpha_fail "runtime detection ($1): expected $2, got $got"
done

echo "setup-test: all assertions passed"
