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
  local port="$1" cid entry hp
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
    done < <(docker ps -aq --filter "label=com.docker.compose.project=${ipalpha_infra_name:-ipalpha}" 2>/dev/null)
  fi
  # Apple container: well-known infra names defined in lib/generate.sh.
  if command -v container >/dev/null 2>&1; then
    # Apple's CLI supports JSON, not Docker's Go-template --format syntax.
    if container ls --format json 2>/dev/null | node -e '
      let input = "";
      process.stdin.on("data", chunk => input += chunk).on("end", () => {
        try {
          const names = ["mongo", "redis", "rabbitmq"].map(kind => `${process.argv[1]}-${kind}`);
          const ours = JSON.parse(input).some(c => names.includes(c.configuration?.id)
            && (c.configuration?.publishedPorts || []).some(p => Number(p.hostPort) === Number(process.argv[2])));
          process.exitCode = ours ? 0 : 1;
        } catch { process.exitCode = 1; }
      });
    ' "${ipalpha_infra_name:-ipalpha}" "$port"; then return 0; fi
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
  if ipalpha_port_busy "$port" || ipalpha_port_reserved "$port"; then
    echo "  $(ipalpha_msg ports_remap): $default"
    port=$((default + 1))
    while ipalpha_port_busy "$port" || ipalpha_port_reserved "$port" || ipalpha_port_default "$port"; do port=$((port + 1)); done
  fi
  ipalpha_reserved_ports+=("$port")
  printf -v "$var" '%s' "$port"
  if [[ "$port" != "$default" ]]; then
    ipalpha_remap_from+=("$default")
    ipalpha_remap_to+=("$port")
  fi
}

ipalpha_port_reserved() {
  local candidate="$1" port
  for port in "${ipalpha_reserved_ports[@]:-}"; do [[ "$port" != "$candidate" ]] || return 0; done
  return 1
}

ipalpha_port_default() {
  local candidate="$1" port
  for port in "${ipalpha_known_default_ports[@]:-}"; do [[ "$port" != "$candidate" ]] || return 0; done
  return 1
}

# Every port variable a workspace owns, with its default ("var default" per line).
ipalpha_port_vars() {
  local repo
  echo "ipalpha_port_mongo $ipalpha_default_mongo_port"
  echo "ipalpha_port_redis $ipalpha_default_redis_port"
  echo "ipalpha_port_rabbitmq $ipalpha_default_rabbitmq_port"
  echo "ipalpha_port_rabbitmq_mgmt $ipalpha_default_rabbitmq_mgmt_port"
  for repo in "${ipalpha_ms_order[@]}"; do echo "ipalpha_port_${repo//-/_} $(ipalpha_default_ms_port "$repo")"; done
  for repo in "${ipalpha_web_repos[@]}"; do echo "ipalpha_port_${repo//-/_} $(ipalpha_default_web_port "$repo")"; done
}

ipalpha_resolve_ports() {
  echo "$(ipalpha_msg ports_check)"
  ipalpha_remap_from=()
  ipalpha_remap_to=()
  ipalpha_reserved_ports=()
  ipalpha_known_default_ports=()
  local var default
  # Replacement ports stay outside the default set, avoiding chained/idempotency
  # ambiguities when a later pull rewrites newly added env values.
  while read -r var default; do ipalpha_known_default_ports+=("$default"); done < <(ipalpha_port_vars)
  while read -r var default; do
    ipalpha_resolve_port "$var" "$default"
  done < <(ipalpha_port_vars)
}

ipalpha_apply_port_rewrites() {
  local root="$1"
  if [[ ${#ipalpha_remap_from[@]} -eq 0 ]]; then
    return 0
  fi
  local i repo env_file mappings=""
  for i in "${!ipalpha_remap_from[@]}"; do
    mappings+="${ipalpha_remap_from[$i]}:${ipalpha_remap_to[$i]} "
  done
  for repo in $(ipalpha_all_repos); do
    env_file="$(ipalpha_repo_path "$root" "$repo")/.env"
    [[ -f "$env_file" ]] || continue
    # One pass: 3001→3002 and 3002→3003 must not turn both APIs into 3003.
    node -e '
      const fs = require("fs"), file = process.argv[1];
      const ports = Object.fromEntries(process.argv[2].trim().split(" ").map(p => p.split(":")));
      const original = fs.readFileSync(file, "utf8");
      const updated = original.replace(/(localhost|127\.0\.0\.1):(\d+)(?=[/\s?\x22\x27,]|$)/g,
        (all, host, port) => ports[port] ? `${host}:${ports[port]}` : all)
        .replace(/^PORT=(\d+)$/m, (all, port) => ports[port] ? `PORT=${ports[port]}` : all);
      if (original !== updated) fs.writeFileSync(file, updated, { mode: 0o600 });
    ' "$env_file" "$mappings"
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
  for repo in "${ipalpha_web_repos[@]}"; do
    old="$(ipalpha_default_web_port "$repo")"
    new="$(ipalpha_settings_web_port "$repo")"
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
