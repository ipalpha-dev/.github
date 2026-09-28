#!/usr/bin/env bash

ipalpha_port_busy() {
  local port="$1"
  if command -v lsof >/dev/null 2>&1; then
    lsof -iTCP:"$port" -sTCP:LISTEN -P -n >/dev/null 2>&1
  elif command -v nc >/dev/null 2>&1; then
    nc -z 127.0.0.1 "$port" >/dev/null 2>&1
  else
    (exec 3<>"/dev/tcp/127.0.0.1/$port") 2>/dev/null && { exec 3>&- 3<&-; return 0; }
    return 1
  fi
}

ipalpha_find_free_port() {
  local port="$1"
  while ipalpha_port_busy "$port"; do
    port=$((port + 1))
  done
  echo "$port"
}

ipalpha_resolve_port() {
  local var="$1" default="$2"
  local port="$default"
  if ipalpha_port_busy "$port"; then
    echo "  $(ipalpha_msg ports_remap): $default"
    port="$(ipalpha_find_free_port $((default + 1)))"
  fi
  printf -v "$var" '%s' "$port"
  if [[ "$port" != "$default" ]]; then
    ipalpha_remap_from+=("$default")
    ipalpha_remap_to+=("$port")
  fi
}

ipalpha_resolve_ports() {
  echo "$(ipalpha_msg ports_check)"
  ipalpha_remap_from=()
  ipalpha_remap_to=()
  ipalpha_resolve_port ipalpha_port_mongo "$ipalpha_default_mongo_port"
  ipalpha_resolve_port ipalpha_port_redis "$ipalpha_default_redis_port"
  ipalpha_resolve_port ipalpha_port_rabbitmq "$ipalpha_default_rabbitmq_port"
  ipalpha_resolve_port ipalpha_port_rabbitmq_mgmt "$ipalpha_default_rabbitmq_mgmt_port"
  local repo var
  for repo in "${ipalpha_ms_order[@]}"; do
    var="ipalpha_port_${repo//-/_}"
    ipalpha_resolve_port "$var" "$(ipalpha_default_ms_port "$repo")"
  done
}

ipalpha_apply_port_rewrites() {
  local root="$1"
  if [[ ${#ipalpha_remap_from[@]} -eq 0 ]]; then
    return 0
  fi
  local i old new repo env_file
  for i in "${!ipalpha_remap_from[@]}"; do
    old="${ipalpha_remap_from[$i]}"
    new="${ipalpha_remap_to[$i]}"
    for repo in $(ipalpha_all_repos); do
      env_file="$(ipalpha_repo_path "$root" "$repo")/.env"
      ipalpha_rewrite_port_in_file "$env_file" "$old" "$new"
    done
  done
}

ipalpha_rewrites_from_settings() {
  ipalpha_remap_from=()
  ipalpha_remap_to=()
  local pairs=(
    "$ipalpha_default_mongo_port:${ipalpha_port_mongo:-$ipalpha_default_mongo_port}"
    "$ipalpha_default_redis_port:${ipalpha_port_redis:-$ipalpha_default_redis_port}"
    "$ipalpha_default_rabbitmq_port:${ipalpha_port_rabbitmq:-$ipalpha_default_rabbitmq_port}"
  )
  local repo old new
  for repo in "${ipalpha_ms_order[@]}"; do
    old="$(ipalpha_default_ms_port "$repo")"
    new="$(ipalpha_settings_ms_port "$repo")"
    pairs+=("$old:$new")
  done
  local p
  for p in "${pairs[@]}"; do
    old="${p%%:*}"
    new="${p#*:}"
    if [[ "$old" != "$new" ]]; then
      ipalpha_remap_from+=("$old")
      ipalpha_remap_to+=("$new")
    fi
  done
  return 0
}
