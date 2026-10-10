#!/usr/bin/env bash
set -euo pipefail

ipalpha_repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ipalpha_repo_root"

# Native Node/Python on Git Bash cannot execute the POSIX-only /bin/bash path.
# Pass the host path explicitly; Linux/macOS keep the conventional path.
if [[ -z "${IPALPHA_BASH:-}" ]]; then
  if command -v cygpath >/dev/null 2>&1; then
    IPALPHA_BASH="$(cygpath -w "$(command -v bash)")"
  else
    IPALPHA_BASH="$(command -v bash)"
  fi
  export IPALPHA_BASH
fi

ipalpha_fail() { echo "FAIL: $1" >&2; exit 1; }

echo "== bash -n over setup, lib and tests"
bash -n setup || ipalpha_fail "syntax error in setup"
for ipalpha_script in lib/*.sh tests/*.sh tests/fake-clone tests/local-clone; do
  [[ -f "$ipalpha_script" ]] || continue
  bash -n "$ipalpha_script" || ipalpha_fail "syntax error in $ipalpha_script"
done

echo "== shellcheck"
if command -v shellcheck >/dev/null 2>&1; then
  shellcheck setup lib/*.sh tests/*.sh tests/fake-clone \
    || ipalpha_fail "shellcheck reported issues"
else
  echo "SKIP: shellcheck not installed"
fi

echo "== go tests"
if [[ -d templates/procs ]] && command -v go >/dev/null 2>&1; then
  (cd templates/procs && go test ./...) || ipalpha_fail "go tests failed"
else
  echo "SKIP: go not installed"
fi

echo "== docker compose config"
if command -v docker >/dev/null 2>&1 && docker compose version >/dev/null 2>&1; then
  ipalpha_compose_tmp="$(mktemp -d)"
  trap 'rm -rf "$ipalpha_compose_tmp"' EXIT
  printf 'MONGO_USERNAME=x\nMONGO_PASSWORD=x\nRABBITMQ_USERNAME=x\nRABBITMQ_PASSWORD=x\n' >"$ipalpha_compose_tmp/credentials.env"
  printf 'MONGO_HOST_PORT=27017\nREDIS_HOST_PORT=6379\nRABBITMQ_HOST_PORT=5672\nRABBITMQ_MGMT_HOST_PORT=15672\n' >"$ipalpha_compose_tmp/ports.env"
  if ! docker compose \
    --env-file "$ipalpha_compose_tmp/credentials.env" \
    --env-file "$ipalpha_compose_tmp/ports.env" \
    -f templates/compose.yaml config --quiet; then
    ipalpha_fail "compose config rejected compose.yaml"
  fi
  rm -rf "$ipalpha_compose_tmp"
  trap - EXIT
else
  echo "SKIP: docker compose unavailable"
fi

echo "== i18n keys present in both languages"
for key in help_feature feature_bad_slug feature_no_baseline feature_confirm cleanup update_tooling_ok update_tooling_fail choose_lang target_folder checking_tools done done_run_now update_done help_run help_pull help_publish publish_no_dirty publish_done update_no_settings; do
  grep -q "pt-BR:$key)" lib/i18n.sh || ipalpha_fail "missing pt-BR i18n key: $key"
  grep -q "en-US:$key)" lib/i18n.sh || ipalpha_fail "missing en-US i18n key: $key"
done

echo "== README setup lines"
grep -qF 'bash <(curl -fsSL https://raw.githubusercontent.com/ipalpha-dev/.github/master/bootstrap.sh)' README.md \
  || ipalpha_fail "README missing copy-paste setup line"
grep -qF 'bash <(curl -fsSL https://raw.githubusercontent.com/ipalpha-dev/.github/master/bootstrap.sh)' profile/README.md \
  || ipalpha_fail "org profile missing copy-paste setup line"
bash -n bootstrap.sh || ipalpha_fail "bootstrap.sh syntax"

echo "== env fallback coverage and blank secrets"
# shellcheck source=lib/common.sh
source lib/common.sh
for ipalpha_api in "${ipalpha_ms_order[@]}"; do
  [[ -f "templates/env-fallback/$ipalpha_api.env" ]] || ipalpha_fail "env-fallback missing file: $ipalpha_api.env"
  grep -q "^PORT=$(ipalpha_default_ms_port "$ipalpha_api")$" "templates/env-fallback/$ipalpha_api.env" \
    || ipalpha_fail "env-fallback $ipalpha_api.env PORT differs from its default port"
done
for key in AUTH_API_URL PROJECTS_API_URL RABBITMQ_URL AUTH_CLIENT_ID AUTH_CLIENT_SECRET MONGO_URI REDIS_URL PORT SMSBARATO_KEY COMTELE_API_URL COMTELE_TOKEN SUPERUSER_NAME SUPERUSER_PHONE SUPERUSER_EMAIL WEBAUTHN_RP_ID AI_API_KEY OPENROUTER_API_KEY AI_LIVE_API_KEY PORTAL_EXTERNAL_CLIENT_IDS; do
  grep -q "^${key}=" templates/env-fallback/*.env \
    || ipalpha_fail "env-fallback missing key: $key"
done
if grep -HE '^(SMSBARATO_KEY|COMTELE_TOKEN|AUTH_CLIENT_ID|AUTH_CLIENT_SECRET|SUPERUSER_PHONE|SUPERUSER_EMAIL|AI_API_KEY|OPENROUTER_API_KEY|AI_LIVE_API_KEY)=.+' templates/env-fallback/*.env; then
  ipalpha_fail "secrets must stay blank in templates"
fi

echo "== canonical security policy"
node "$ipalpha_repo_root/tests/policy-test.mjs"

echo "== setup fixture test"
node "$ipalpha_repo_root/tests/browser-test.mjs"
node "$ipalpha_repo_root/tests/mailpit-test.mjs"
"$ipalpha_repo_root/tests/setup-test.sh"

echo "== local environment completion test"
node "$ipalpha_repo_root/tests/local-env-test.mjs"
node "$ipalpha_repo_root/tests/local-clients-test.mjs"

echo "== initial superuser setup test"
node "$ipalpha_repo_root/tests/superuser-test.mjs"

echo "== parallel repository clone test"
node "$ipalpha_repo_root/tests/clone-test.mjs"

echo "== setup terminal UI test"
if command -v python3 >/dev/null 2>&1 && python3 -c 'import sys' >/dev/null 2>&1; then
  python3 "$ipalpha_repo_root/tests/setup-ui-test.py"
else
  echo "SKIP: functional python3 not installed"
fi

echo "== feature workspace test"
"$ipalpha_repo_root/tests/feature-test.sh"

echo "validate.sh: all checks passed"
