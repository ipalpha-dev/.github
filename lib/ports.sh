#!/usr/bin/env bash

# Returns 0 if anything is listening on the host port, 1 otherwise.
ipalpha_tcp_listening() {
  [[ "${IPALPHA_TEST_NO_PORT_PROBE:-}" == "1" ]] && return 1
  local port="$1"
  if command -v lsof >/dev/null 2>&1; then
    lsof -iTCP:"$port" -sTCP:LISTEN -P -n >/dev/null 2>&1
  elif command -v nc >/dev/null 2>&1; then
    nc -z 127.0.0.1 "$port" >/dev/null 2>&1
  else
    if (exec 3<>"/dev/tcp/127.0.0.1/$port") 2>/dev/null; then
      exec 3>&- 3<&-
      return 0
    fi
    return 1
  fi
}

# Returns 0 if the port is held by one of our own ipalpha containers
# (Docker compose project "ipalpha", or Apple container named ipalpha-*).
# The port-busy check consults this so ./pull while the infra is already
# up doesn't keep remapping to ever-higher host ports.
ipalpha_owns_port() {
  local port="$1" cid entry hp name
  # Docker compose project name comes from the dir holding compose.yaml (.ipalpha/).
  if command -v docker >/dev/null 2>&1 && docker info >/dev/null 2>&1; then
    while IFS= read -r cid; do
      [[ -z "$cid" ]] && continue
      while IFS= read -r entry; do
        [[ -z "$entry" ]] && continue
        # docker port output: "27017/tcp -> 0.0.0.0:27017"
        hp="${entry##*:}"
        [[ "$hp" == "$port" ]] && return 0
      done < <(docker port "$cid" 2>/dev/null)
    done < <(docker ps -aq --filter "label=com.docker.compose.project=ipalpha" 2>/dev/null)
  fi
  # Apple container: well-known infra names defined in lib/generate.sh.
  if command -v container >/dev/null 2>&1; then
    while IFS= read -r name; do
      [[ -z "$name" ]] && continue
      case "$name" in
        ipalpha-mongo|ipalpha-redis|ipalpha-rabbitmq) ;;
        *) continue ;;
      esac
      while IFS= read -r entry; do
        [[ -z "$entry" ]] && continue
        hp="${entry##*:}"
        [[ "$hp" == "$port" ]] && return 0
      done < <(container inspect "$name"                 --format "{{range \$k, \$v := .NetworkSettings.Ports}}{{range \$v}}HostPort={{.HostPort}};{{end}}{{end}}"                 2>/dev/null | tr ';' "\n" | grep -oE "HostPort=[0-9]+" | sed "s/.*=//")
    done < <(container ls --format "{{.Names}}" 2>/dev/null)
  fi
  return 1
}

ipalpha_port_busy() {
  local port="$1"
  ipalpha_tcp_listening "$port" || return 1
  # Port has a listener. If it's ours, treat as free so we reuse the same port.
  ipalpha_owns_port "$port" && return 1
  return 0
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
  for repo in "${ipalpha_web_repos[@]}"; do
    var="ipalpha_port_${repo//-/_}"
    ipalpha_resolve_port "$var" "$(ipalpha_default_web_port "$repo")"
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
