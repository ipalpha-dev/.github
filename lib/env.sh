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
  else
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
  fi
  local helper="$fallback_dir/../lib/local-env.mjs"
  [[ -f "$helper" ]] || helper="$(dirname "${BASH_SOURCE[0]}")/local-env.mjs"
  node "$helper" "$repo_dir" "$name" "$fallback_dir/$name.env"
}

ipalpha_env_get() {
  local file="$1" key="$2" value
  [[ -f "$file" ]] || return 1
  value="$(sed -n "s/^${key}=//p" "$file" | head -n1 | tr -d '\r')"
  if [[ "$value" =~ ^\'(.*)\'$ || "$value" =~ ^\"(.*)\"$ ]]; then value="${BASH_REMATCH[1]}"; fi
  printf '%s\n' "$value"
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
      secret="$(node -e 'process.stdout.write(require("crypto").randomBytes(24).toString("hex"))')"
      ipalpha_env_set_key "$env_file" AUTH_CLIENT_SECRET "$secret"
    fi
  done
  local paths=""
  for repo in "${ipalpha_ms_order[@]}"; do paths+="$repo=$(ipalpha_repo_rel "$repo") "; done
  IPALPHA_ROOT="$root" IPALPHA_MS_PATHS="$paths" node -e '
    const fs = require("fs");
    const path = require("path");
    const root = process.env.IPALPHA_ROOT;
    const manifest = [path.join(root, "deployment/base/core/auth-api/system-clients.json"),
      path.join(root, ".ipalpha/system-clients.json"),
      path.join(process.argv[1], "../templates/system-clients.json")].find(f => fs.existsSync(f));
    if (!manifest) throw new Error("Missing local system-client scope manifest");
    const approved = JSON.parse(fs.readFileSync(manifest, "utf8")).clients;
    const read = f => Object.fromEntries(fs.readFileSync(f, "utf8").split("\n")
      .filter(l => /^[A-Za-z_][A-Za-z0-9_]*=/.test(l))
      .map(l => [l.slice(0, l.indexOf("=")), l.slice(l.indexOf("=") + 1).replace(/^["\x27]|["\x27]$/g, "")]));
    const ours = process.env.IPALPHA_MS_PATHS.trim().split(" ").map(entry => {
      const [ms, rel] = entry.split("=");
      const f = path.join(root, rel, ".env");
      if (!fs.existsSync(f)) return null;
      const e = read(f);
      return e.AUTH_CLIENT_ID && e.AUTH_CLIENT_SECRET ? { clientId: e.AUTH_CLIENT_ID, secret: e.AUTH_CLIENT_SECRET, serviceId: ms, scopes: approved[ms] || [] } : null;
    }).filter(Boolean);
    const authEnv = path.join(root, "core", "auth-api", ".env");
    let current = [];
    try { current = JSON.parse(read(authEnv).SEED_CLIENTS_JSON || "[]"); } catch { throw new Error("Invalid local SEED_CLIENTS_JSON; refusing to replace operator clients"); }
    if (!Array.isArray(current)) throw new Error("Local SEED_CLIENTS_JSON must be an array");
    const ids = new Set(ours.map(c => c.clientId));
    const merged = [...current.filter(c => !ids.has(c.clientId)), ...ours.map(c => {
      const previous = current.find(p => p.clientId === c.clientId);
      return { ...c, scopes: [...new Set([...(Array.isArray(previous?.scopes) ? previous.scopes : []), ...c.scopes])] };
    })];
    const text = fs.readFileSync(authEnv, "utf8");
    const line = "SEED_CLIENTS_JSON=\x27" + JSON.stringify(merged) + "\x27";
    fs.writeFileSync(authEnv, /^SEED_CLIENTS_JSON=.*$/m.test(text)
      ? text.replace(/^SEED_CLIENTS_JSON=.*$/m, () => line)
      : text.replace(/\n?$/, "\n") + line + "\n");
    fs.chmodSync(authEnv, 0o600);
  ' "$(dirname "${BASH_SOURCE[0]}")"
}

ipalpha_prepare_local_envs() {
  local root="$1" fallback="$2" repo dir
  for repo in "${ipalpha_ms_repos[@]}"; do
    dir="$(ipalpha_repo_path "$root" "$repo")"
    [[ -d "$dir" ]] || continue
    ipalpha_install_repo_env "$dir" "$repo" "$fallback"
  done
  ipalpha_rewrites_from_settings
  ipalpha_apply_port_rewrites "$root"
  ipalpha_seed_local_clients "$root"
}
