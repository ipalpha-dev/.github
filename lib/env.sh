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

ipalpha_seed_local_clients() {
  local root="$1" repo env_file secret auth_env
  auth_env="$(ipalpha_repo_path "$root" auth-api)/.env"
  [[ -f "$auth_env" ]] || return 0
  echo "  $(ipalpha_msg auth_clients)"
  for repo in "${ipalpha_ms_order[@]}"; do
    env_file="$(ipalpha_repo_path "$root" "$repo")/.env"
    [[ -f "$env_file" ]] || continue
    [[ -n "$(ipalpha_env_get "$env_file" AUTH_CLIENT_ID)" ]] || ipalpha_env_set_key "$env_file" AUTH_CLIENT_ID "$repo"
    if [[ -z "$(ipalpha_env_get "$env_file" AUTH_CLIENT_SECRET)" ]]; then
      secret="$(openssl rand -hex 24 2>/dev/null || LC_ALL=C tr -dc 'a-f0-9' </dev/urandom | head -c 48)"
      ipalpha_env_set_key "$env_file" AUTH_CLIENT_SECRET "$secret"
    fi
  done
  IPALPHA_ROOT="$root" IPALPHA_MS="${ipalpha_ms_order[*]}" node -e '
    const fs = require("fs");
    const path = require("path");
    const root = process.env.IPALPHA_ROOT;
    const read = f => Object.fromEntries(fs.readFileSync(f, "utf8").split("\n")
      .filter(l => /^[A-Za-z_][A-Za-z0-9_]*=/.test(l))
      .map(l => [l.slice(0, l.indexOf("=")), l.slice(l.indexOf("=") + 1).replace(/^["\x27]|["\x27]$/g, "")]));
    const ours = process.env.IPALPHA_MS.split(" ").map(ms => {
      const f = path.join(root, "core", ms, ".env");
      if (!fs.existsSync(f)) return null;
      const e = read(f);
      return e.AUTH_CLIENT_ID && e.AUTH_CLIENT_SECRET ? { clientId: e.AUTH_CLIENT_ID, secret: e.AUTH_CLIENT_SECRET, ms } : null;
    }).filter(Boolean);
    const authEnv = path.join(root, "core", "auth-api", ".env");
    let current = [];
    try { current = JSON.parse(read(authEnv).SEED_CLIENTS_JSON || "[]"); } catch {}
    const ids = new Set(ours.map(c => c.clientId));
    const merged = [...current.filter(c => !ids.has(c.clientId)), ...ours];
    const text = fs.readFileSync(authEnv, "utf8");
    const line = "SEED_CLIENTS_JSON=\x27" + JSON.stringify(merged) + "\x27";
    fs.writeFileSync(authEnv, /^SEED_CLIENTS_JSON=.*$/m.test(text)
      ? text.replace(/^SEED_CLIENTS_JSON=.*$/m, () => line)
      : text.replace(/\n?$/, "\n") + line + "\n");
    fs.chmodSync(authEnv, 0o600);
  '
}
