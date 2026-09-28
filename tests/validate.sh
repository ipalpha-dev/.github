#!/usr/bin/env bash
set -euo pipefail

ipalpha_repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ipalpha_repo_root"

ipalpha_fail() { echo "FAIL: $1" >&2; exit 1; }

echo "== bash -n over setup, lib and tests"
bash -n setup || ipalpha_fail "syntax error in setup"
for ipalpha_script in lib/*.sh tests/*.sh tests/fake-clone; do
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
if [[ -d templates/publish ]] && command -v go >/dev/null 2>&1; then
  (cd templates/publish && go test ./...) || ipalpha_fail "go tests failed"
else
  echo "SKIP: no Go code (publish is bash)"
fi

echo "== docker compose config"
if command -v docker >/dev/null 2>&1 && docker compose version >/dev/null 2>&1; then
  docker compose \
    --env-file <(printf 'MONGO_USERNAME=x\nMONGO_PASSWORD=x\nRABBITMQ_USERNAME=x\nRABBITMQ_PASSWORD=x\n') \
    --env-file <(printf 'MONGO_HOST_PORT=27017\nREDIS_HOST_PORT=6379\nRABBITMQ_HOST_PORT=5672\nRABBITMQ_MGMT_HOST_PORT=15672\n') \
    -f templates/compose.yaml \
    config --quiet || ipalpha_fail "compose config rejected compose.yaml"
else
  echo "SKIP: docker compose unavailable"
fi

echo "== i18n keys present in both languages"
for key in cleanup update_tooling_ok update_tooling_fail choose_lang target_folder checking_tools done done_run_now update_done help_run help_pull help_publish publish_no_dirty publish_done update_no_settings; do
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
for key in AUTH_API_URL PROJECTS_API_URL RABBITMQ_URL AUTH_CLIENT_ID AUTH_CLIENT_SECRET MONGO_URI REDIS_URL PORT SMSBARATO_KEY COMTELE_API_URL COMTELE_TOKEN SUPERUSER_NAME SUPERUSER_PHONE SUPERUSER_EMAIL WEBAUTHN_RP_ID; do
  grep -q "^${key}=" templates/env-fallback/*.env \
    || ipalpha_fail "env-fallback missing key: $key"
done
if grep -HE '^(SMSBARATO_KEY|COMTELE_TOKEN|AUTH_CLIENT_ID|AUTH_CLIENT_SECRET|SUPERUSER_PHONE|SUPERUSER_EMAIL)=.+' templates/env-fallback/*.env; then
  ipalpha_fail "secrets must stay blank in templates"
fi

echo "== setup fixture test"
"$ipalpha_repo_root/tests/setup-test.sh"

echo "validate.sh: all checks passed"
