#!/usr/bin/env bash

ipalpha_gitignore_ensure() {
  local repo_dir="$1" entry="$2"
  local gi="$repo_dir/.gitignore"
  [[ -f "$gi" ]] && grep -qxF "$entry" "$gi" 2>/dev/null && return 0
  {
    [[ -f "$gi" && -s "$gi" && "$(tail -c1 "$gi" 2>/dev/null)" != $'\n' ]] && echo
    echo "$entry"
  } >>"$gi" 2>/dev/null || true
}

ipalpha_env_source_for() {
  local repo_dir="$1" name="$2" fallback_dir="$3"
  if [[ -f "$repo_dir/.env.example" ]]; then
    echo "$repo_dir/.env.example|example"
    return 0
  fi
  if [[ -f "$fallback_dir/${name}.env" ]]; then
    echo "$fallback_dir/${name}.env|fallback"
    return 0
  fi
  return 1
}

ipalpha_env_missing_keys() {
  local env_file="$1" source_file="$2"
  local key
  while IFS= read -r line; do
    [[ -z "$line" || "$line" == \#* ]] && continue
    key="${line%%=*}"
    [[ "$key" =~ ^[A-Za-z_][A-Za-z0-9_]*$ ]] || continue
    grep -qE "^${key}=" "$env_file" 2>/dev/null || echo "$line"
  done <"$source_file"
}

ipalpha_install_repo_env() {
  local repo_dir="$1" name="$2" fallback_dir="$3"
  local found source flag missing

  if ! found="$(ipalpha_env_source_for "$repo_dir" "$name" "$fallback_dir")"; then
    echo "  $(ipalpha_msg env_skip): $name"
    return 0
  fi
  IFS='|' read -r source flag <<<"$found"
  if [[ "$flag" == "fallback" ]]; then
    echo "  $(ipalpha_msg env_fallback): $name"
  fi

  ipalpha_gitignore_ensure "$repo_dir" ".env"

  if [[ ! -f "$repo_dir/.env" ]]; then
    cp "$source" "$repo_dir/.env"
    echo "  $(ipalpha_msg env_install): $repo_dir/.env"
    return 0
  fi

  missing="$(ipalpha_env_missing_keys "$repo_dir/.env" "$source")"
  if [[ -n "$missing" ]]; then
    {
      [[ -n "$(tail -c1 "$repo_dir/.env" 2>/dev/null)" ]] && echo
      echo "$missing"
    } >>"$repo_dir/.env"
    echo "  $(ipalpha_msg env_merge): $repo_dir/.env"
  else
    echo "  $(ipalpha_msg env_keep): $repo_dir/.env"
  fi
}

ipalpha_env_get() {
  local file="$1" key="$2"
  [[ -f "$file" ]] || return 1
  sed -n "s/^${key}=//p" "$file" | head -n1 | tr -d '\r'
}

ipalpha_env_set_key() {
  local file="$1" key="$2" value="$3"
  local tmp
  tmp="$(mktemp)"
  if [[ -f "$file" ]] && grep -qE "^${key}=" "$file"; then
    sed "s|^${key}=.*|${key}=${value}|" "$file" >"$tmp"
  else
    cat "$file" 2>/dev/null >"$tmp" || true
    echo "${key}=${value}" >>"$tmp"
  fi
  mv "$tmp" "$file"
}

ipalpha_rewrite_port_in_file() {
  local file="$1" old_port="$2" new_port="$3"
  [[ -f "$file" ]] || return 0
  local tmp
  tmp="$(mktemp)"
  sed -E \
    -e "s/@127\.0\.0\.1:${old_port}/@127.0.0.1:${new_port}/g" \
    -e "s/@localhost:${old_port}/@localhost:${new_port}/g" \
    -e "s|://127\.0\.0\.1:${old_port}|://127.0.0.1:${new_port}|g" \
    -e "s|://localhost:${old_port}|://localhost:${new_port}|g" \
    -e "s/^PORT=${old_port}\$/PORT=${new_port}/" \
    "$file" >"$tmp"
  mv "$tmp" "$file"
}
