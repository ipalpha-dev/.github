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

for repo in shared-js projects-api person-api organization-api notification-api auth-api auth-webapp; do
  [[ -d "$ipalpha_tmp/IpAlpha/core/$repo/.git" ]] \
    || ipalpha_fail "ms repo not cloned: $repo"
done
[[ ! -f "$ipalpha_tmp/IpAlpha/core/auth-webapp/.env" ]] || ipalpha_fail "auth-webapp must not get an env"
grep -q '^auth-webapp_port=5100$' "$ipalpha_tmp/IpAlpha/.ipalpha/settings" || ipalpha_fail "settings missing auth-webapp_port"
[[ -x "$ipalpha_tmp/IpAlpha/.ipalpha/bin/web-dev" ]] || ipalpha_fail "web-dev missing"
# fake clone has no package.json -> web app is skipped from the runners until cloned for real
grep -q '"name": "auth-webapp"' "$ipalpha_tmp/IpAlpha/.ipalpha/projects.json" && ipalpha_fail "projects.json listed auth-webapp without package.json"
echo '{"name":"auth-webapp"}' >"$ipalpha_tmp/IpAlpha/core/auth-webapp/package.json"
[[ -d "$ipalpha_tmp/IpAlpha/deployment/.git" ]] || ipalpha_fail "deployment not cloned"
[[ ! -e "$ipalpha_tmp/IpAlpha/develop" ]] || ipalpha_fail "develop must not be cloned"
[[ ! -e "$ipalpha_tmp/IpAlpha/.github" ]] || ipalpha_fail ".github must not be cloned into the workspace"
[[ -x "$ipalpha_tmp/IpAlpha/set-keys" ]] || ipalpha_fail "./set-keys missing"

for repo in projects-api person-api organization-api notification-api auth-api; do
  [[ -f "$ipalpha_tmp/IpAlpha/core/$repo/.env" ]] || ipalpha_fail "env missing: $repo"
done
grep -q '^SUPERUSER_NAME=$' "$ipalpha_tmp/IpAlpha/core/auth-api/.env" || ipalpha_fail "auth-api env missing SUPERUSER_NAME"
grep -q '^SMSBARATO_KEY=$' "$ipalpha_tmp/IpAlpha/core/notification-api/.env" || ipalpha_fail "notification-api env missing SMSBARATO_KEY"
grep -q '^PORT=3001$' "$ipalpha_tmp/IpAlpha/core/projects-api/.env" || ipalpha_fail "projects-api env wrong PORT"
grep -q '^AUTH_API_URL=http://127.0.0.1:3005$' "$ipalpha_tmp/IpAlpha/core/person-api/.env" || ipalpha_fail "person-api env wrong AUTH_API_URL"
[[ ! -f "$ipalpha_tmp/IpAlpha/core/shared-js/.env" ]] || ipalpha_fail "shared-js must not get an env"

[[ -x "$ipalpha_tmp/IpAlpha/run" ]] || ipalpha_fail "./run missing"
[[ -x "$ipalpha_tmp/IpAlpha/publish" ]] || ipalpha_fail "./publish missing"
[[ -x "$ipalpha_tmp/IpAlpha/pull" ]] || ipalpha_fail "./pull missing"
[[ -f "$ipalpha_tmp/IpAlpha/.ipalpha/settings" ]] || ipalpha_fail "settings missing"

for key in lang org runtime ai_cli ai_model runner mongo_port redis_port rabbitmq_port rabbitmq_mgmt_port projects-api_port person-api_port organization-api_port notification-api_port auth-api_port; do
  grep -q "^${key}=" "$ipalpha_tmp/IpAlpha/.ipalpha/settings" || ipalpha_fail "settings missing key: $key"
done
grep -q '^lang=en-US' "$ipalpha_tmp/IpAlpha/.ipalpha/settings" || ipalpha_fail "settings lang wrong"
grep -q '^org=ipalpha' "$ipalpha_tmp/IpAlpha/.ipalpha/settings" || ipalpha_fail "settings org wrong"
grep -q '^ai_cli=pi' "$ipalpha_tmp/IpAlpha/.ipalpha/settings" || ipalpha_fail "settings ai_cli wrong"
grep -q '^ai_model=cpamc/muse-spark-1.3-contributor' "$ipalpha_tmp/IpAlpha/.ipalpha/settings" || ipalpha_fail "settings ai_model wrong"
grep -q '^runner=auto$' "$ipalpha_tmp/IpAlpha/.ipalpha/settings" || ipalpha_fail "settings runner must default to auto"
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
person-api
organization-api
notification-api
auth-api" ]] || ipalpha_fail "mprocs order wrong: $ipalpha_order"

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
cat >"$ipalpha_tmp/IpAlpha/core/person-api/.env.example" <<'EOF'
PORT=3002
NEW_KEY=42
EOF
ipalpha_out="$("$ipalpha_repo_root/setup" --skip-tools --keep-setup 2>&1)" \
  || ipalpha_fail "second setup failed: $ipalpha_out"
grep -q 'LOCAL_CUSTOMIZATION=1' "$ipalpha_tmp/IpAlpha/core/projects-api/.env" \
  || ipalpha_fail "second run overwrote local env"
grep -q '^NEW_KEY=42$' "$ipalpha_tmp/IpAlpha/core/person-api/.env" \
  || ipalpha_fail "second run did not merge new keys"
grep -q '"name": "auth-webapp", "kind": "app"' "$ipalpha_tmp/IpAlpha/.ipalpha/projects.json" || ipalpha_fail "projects.json missing auth-webapp app"
grep -q 'Web · auth-webapp' "$ipalpha_tmp/IpAlpha/.ipalpha/mprocs.yaml" || ipalpha_fail "mprocs missing auth-webapp"
grep -q 'web-dev auth-webapp' "$ipalpha_tmp/IpAlpha/.ipalpha/projects.json" || ipalpha_fail "projects.json auth-webapp cmd wrong"

echo "== pull re-clones missing repo"
rm -rf "$ipalpha_tmp/IpAlpha/core/person-api"
ipalpha_out="$(
  cd "$ipalpha_tmp/IpAlpha" && IPALPHA_CLONE_COMMAND="$ipalpha_repo_root/tests/fake-clone" ./pull 2>&1
)" || ipalpha_fail "pull failed: $ipalpha_out"
[[ -d "$ipalpha_tmp/IpAlpha/core/person-api/.git" ]] || ipalpha_fail "pull did not re-clone person-api"
[[ -f "$ipalpha_tmp/IpAlpha/core/person-api/.env" ]] || ipalpha_fail "pull did not reinstall env"

echo "== publish smoke (no AI, dry-run aborts without changes)"
repo_dir="$ipalpha_tmp/IpAlpha/core/projects-api"
git init -q -b master "$repo_dir"
git -C "$repo_dir" -c user.name=test -c user.email=test@example.invalid commit -q --allow-empty -m "root"
printf '{\n  "name": "projects-api",\n  "version": "0.0.0",\n  "scripts": { "start:dev": "node -e \\"console.log(1)\\"" }\n}\n' >"$repo_dir/package.json"
echo "dirty" >"$repo_dir/notes.txt"
inplace 's/^ai_cli=.*/ai_cli=bogus/' "$ipalpha_tmp/IpAlpha/.ipalpha/settings"
ipalpha_out="$(
  cd "$ipalpha_tmp/IpAlpha" && ./publish -d -f projects-api </dev/null 2>&1
)" || ipalpha_fail "publish dry-run failed: $ipalpha_out"
grep -qi 'aborted' <<<"$ipalpha_out" || ipalpha_fail "dry-run did not abort: $ipalpha_out"
[[ -n "$(git -C "$repo_dir" status --porcelain)" ]] || ipalpha_fail "dry-run changed the repo"

ipalpha_out="$(
  cd "$ipalpha_tmp/IpAlpha" && ./publish -f projects-api </dev/null 2>&1
)" || ipalpha_fail "publish failed: $ipalpha_out"
[[ -z "$(git -C "$repo_dir" status --porcelain)" ]] || ipalpha_fail "publish left dirty repo"
git -C "$repo_dir" log --format=%s -1 | grep -q 'Update projects-api' \
  || ipalpha_fail "publish commit message wrong: $(git -C "$repo_dir" log --format=%s -1)"
git -C "$repo_dir" tag | grep -q '^v0\.0\.1$' || ipalpha_fail "publish tag missing"
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

echo "setup-test: all assertions passed"
