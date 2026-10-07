#!/usr/bin/env bash

ipalpha_prompt_superuser() {
  local root="$1" helper auth_file name phone answer error='' interactive=false
  helper="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/superuser.mjs"
  auth_file="$(ipalpha_repo_path "$root" auth-api)/.env"
  name="${IPALPHA_SUPERUSER_NAME:-$(ipalpha_env_get "$auth_file" SUPERUSER_NAME 2>/dev/null || true)}"
  phone="${IPALPHA_SUPERUSER_PHONE:-$(ipalpha_env_get "$auth_file" SUPERUSER_PHONE 2>/dev/null || true)}"
  name="${name:-Joao Silva Costa}"
  if [[ "${ipalpha_ui_active:-false}" == true ]] || [[ -t 0 && -t 1 ]]; then interactive=true; fi
  while true; do
    answer="$name"
    if [[ "$interactive" == true && -z "${IPALPHA_SUPERUSER_NAME:-}" ]]; then
      if [[ "${ipalpha_ui_active:-false}" == true ]]; then
        ipalpha_ui_input "$(ipalpha_msg seed_account)" "$(ipalpha_msg seed_name)" "$name" "$(ipalpha_msg seed_name_hint)" "$error" || exit 130
        answer="$ipalpha_ui_answer"
      else
        printf '%s [%s]: ' "$(ipalpha_msg seed_name)" "$name" >/dev/tty
        IFS= read -r answer </dev/tty || exit 130
        answer="${answer:-$name}"
      fi
    fi
    if name="$(printf '%s' "$answer" | node "$helper" normalize name)"; then break; fi
    error="$(ipalpha_msg seed_name_invalid)"
    if [[ "$interactive" != true || -n "${IPALPHA_SUPERUSER_NAME:-}" ]]; then echo "$error" >&2; return 1; fi
    name="$answer"
  done
  error=''
  while true; do
    answer="$phone"
    if [[ "$interactive" == true && -z "${IPALPHA_SUPERUSER_PHONE:-}" ]]; then
      if [[ "${ipalpha_ui_active:-false}" == true ]]; then
        ipalpha_ui_input "$(ipalpha_msg seed_account)" "$(ipalpha_msg seed_phone)" "$phone" "$(ipalpha_msg seed_phone_hint)" "$error" || exit 130
        answer="$ipalpha_ui_answer"
      else
        printf '%s [%s]: ' "$(ipalpha_msg seed_phone)" "$phone" >/dev/tty
        IFS= read -r answer </dev/tty || exit 130
        answer="${answer:-$phone}"
      fi
    fi
    if phone="$(printf '%s' "$answer" | node "$helper" normalize phone)"; then break; fi
    error="$(ipalpha_msg seed_phone_invalid)"
    if [[ "$interactive" != true || -n "${IPALPHA_SUPERUSER_PHONE:-}" ]]; then echo "$error" >&2; return 1; fi
    phone="$answer"
  done
  ipalpha_superuser_name="$name"
  ipalpha_superuser_phone="$phone"
}

ipalpha_write_superuser() {
  local root="$1"
  if ! IPALPHA_SEED_NAME="$ipalpha_superuser_name" IPALPHA_SEED_PHONE="$ipalpha_superuser_phone" \
    node "$(dirname "${BASH_SOURCE[0]}")/superuser.mjs" write "$(ipalpha_repo_path "$root" auth-api)/.env"; then
    ipalpha_msg seed_save_failed >&2
    return 1
  fi
}
