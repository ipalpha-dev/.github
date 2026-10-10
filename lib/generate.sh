#!/usr/bin/env bash

ipalpha_write_ports_env() {
  local dest="$1"
  cat >"$dest" <<EOF
MONGO_HOST_PORT=${ipalpha_port_mongo:-$ipalpha_default_mongo_port}
REDIS_HOST_PORT=${ipalpha_port_redis:-$ipalpha_default_redis_port}
RABBITMQ_HOST_PORT=${ipalpha_port_rabbitmq:-$ipalpha_default_rabbitmq_port}
RABBITMQ_MGMT_HOST_PORT=${ipalpha_port_rabbitmq_mgmt:-$ipalpha_default_rabbitmq_mgmt_port}
MAILPIT_HOST_PORT=${ipalpha_port_mailpit:-$ipalpha_default_mailpit_port}
IPALPHA_INFRA_NAME=${ipalpha_infra_name:-ipalpha}
EOF
}

ipalpha_write_compose_env() {
  local dest="$1"
  cat >"$dest" <<'EOF'
MONGO_USERNAME=ipalpha
MONGO_PASSWORD=ipalpha
RABBITMQ_USERNAME=ipalpha
RABBITMQ_PASSWORD=ipalpha
EOF
}

ipalpha_write_bin_infra_up() {
  local dest="$1"
  cat >"$dest" <<'SCRIPT'
#!/usr/bin/env bash
set -euo pipefail

ipalpha_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
state_dir="$ipalpha_dir/.state"
mkdir -p "$state_dir"

set -a
# shellcheck disable=SC1091
[[ -f "$ipalpha_dir/ports.env" ]] && source "$ipalpha_dir/ports.env"
# shellcheck disable=SC1091
[[ -f "$ipalpha_dir/.env" ]] && source "$ipalpha_dir/.env"
set +a
infra="${IPALPHA_INFRA_NAME:-ipalpha}"

runtime="container"
if [[ -f "$ipalpha_dir/settings" ]]; then
  runtime="$(sed -n 's/^runtime=//p' "$ipalpha_dir/settings" | head -n1)"
fi
runtime="${runtime:-container}"

container_status() {
  container inspect "$1" 2>/dev/null | tr -d ' \n' | sed -n 's/.*"status":"\([a-z]*\)".*/\1/p'
}

ensure_container() {
  local name="$1"; shift
  local spec_hash status hash_file="$state_dir/$name.spec"
  spec_hash="$(printf '%s\n' "$@" | { shasum 2>/dev/null || sha256sum; } | awk '{print $1}')"
  status="$(container_status "$name")"
  if [[ -n "$status" && "$(cat "$hash_file" 2>/dev/null)" != "$spec_hash" ]]; then
    echo "  $name: config changed — recreating"
    container stop "$name" >/dev/null 2>&1 || true
    container delete --force "$name" >/dev/null 2>&1 || true
    status=""
  fi
  case "$status" in
    running)
      echo "  $name: running"
      ;;
    "")
      echo "  $name: creating"
      container run --detach --name "$name" "$@" >/dev/null 2>"$state_dir/$name.err" \
        || { cat "$state_dir/$name.err" >&2; return 1; }
      printf '%s\n' "$spec_hash" >"$hash_file"
      ;;
    *)
      echo "  $name: starting"
      container start "$name" >/dev/null 2>"$state_dir/$name.err" \
        || { cat "$state_dir/$name.err" >&2; return 1; }
      ;;
  esac
}

wait_ready() {
  local name="$1"; shift
  local _
  for _ in $(seq 1 60); do
    if container exec "$name" "$@" >/dev/null 2>&1; then
      echo "  $name ready"
      return 0
    fi
    sleep 1
  done
  echo "  warning: $name not healthy yet (continuing)" >&2
}

infra_up_container() {
  container system start >/dev/null 2>&1 || true
  container network create "$infra" >/dev/null 2>&1 || true
  container volume create "$infra-mongo-data" >/dev/null 2>&1 || true
  container volume create "$infra-redis-data" >/dev/null 2>&1 || true
  container volume create "$infra-rabbitmq-data" >/dev/null 2>&1 || true

  ensure_container "$infra-mongo" \
    --network "$infra" \
    --publish "${MONGO_HOST_PORT:-27017}:27017" \
    --env "MONGO_INITDB_ROOT_USERNAME=${MONGO_USERNAME:-ipalpha}" \
    --env "MONGO_INITDB_ROOT_PASSWORD=${MONGO_PASSWORD:-ipalpha}" \
    --volume "$infra-mongo-data":/data/db \
    mongo:8 || return 1

  ensure_container "$infra-redis" \
    --network "$infra" \
    --publish "${REDIS_HOST_PORT:-6379}:6379" \
    --volume "$infra-redis-data":/data \
    redis:7-alpine --appendonly yes || return 1

  ensure_container "$infra-rabbitmq" \
    --network "$infra" \
    --publish "${RABBITMQ_HOST_PORT:-5672}:5672" \
    --publish "${RABBITMQ_MGMT_HOST_PORT:-15672}:15672" \
    --env "RABBITMQ_DEFAULT_USER=${RABBITMQ_USERNAME:-ipalpha}" \
    --env "RABBITMQ_DEFAULT_PASS=${RABBITMQ_PASSWORD:-ipalpha}" \
    --volume "$infra-rabbitmq-data":/var/lib/rabbitmq \
    rabbitmq:4-management || return 1

  # Local capture only: HTTP Send API + inbox, no published SMTP or relay.
  ensure_container "$infra-mailpit" \
    --network "$infra" \
    --publish "127.0.0.1:${MAILPIT_HOST_PORT:-8025}:8025" \
    --cpus 1 --memory 256M \
    --env MP_MAX_MESSAGES=200 \
    --env MP_MAX_AGE=24h \
    --env MP_MAX_MESSAGE_SIZE=1 \
    --env MP_DISABLE_VERSION_CHECK=true \
    --env MP_SMTP_DISABLE_RDNS=true \
    axllent/mailpit:v1.31.3 || return 1

  wait_ready "$infra-mongo" mongosh --quiet \
    --username "${MONGO_USERNAME:-ipalpha}" --password "${MONGO_PASSWORD:-ipalpha}" \
    --authenticationDatabase admin --eval 'db.adminCommand({ ping: 1 })'
  wait_ready "$infra-redis" redis-cli ping
  wait_ready "$infra-rabbitmq" rabbitmq-diagnostics -q ping
  wait_ready "$infra-mailpit" /mailpit readyz
}

compose() {
  docker compose --env-file "$ipalpha_dir/.env" --env-file "$ipalpha_dir/ports.env" \
    -f "$ipalpha_dir/compose.yaml" "$@"
}

docker_ours_running() {
  command -v docker >/dev/null 2>&1 && docker info >/dev/null 2>&1 \
    && [[ -n "$(compose ps --quiet 2>/dev/null)" ]]
}

if [[ "$runtime" == "container" ]] && command -v container >/dev/null 2>&1; then
  if docker_ours_running; then
    echo "  stopping Docker copy of the infra (runtime=container)"
    compose stop >/dev/null 2>&1 || true
  fi
  if infra_up_container; then
    echo container >"$state_dir/runtime"
    exit 0
  fi
  if command -v docker >/dev/null 2>&1; then
    echo "warning: Apple container failed — falling back to Docker" >&2
    for name in "$infra-mongo" "$infra-redis" "$infra-rabbitmq" "$infra-mailpit"; do
      container stop "$name" >/dev/null 2>&1 || true
    done
  else
    echo "Apple container runtime failed and docker is not installed." >&2
    exit 1
  fi
fi

if ! command -v docker >/dev/null 2>&1; then
  echo "No container runtime available — install Apple container or Docker." >&2
  exit 1
fi
docker info >/dev/null 2>&1 || { echo "docker daemon is not running." >&2; exit 1; }
compose up --detach --wait
echo docker >"$state_dir/runtime"
SCRIPT
  chmod +x "$dest"
}

ipalpha_write_bin_infra_logs() {
  local dest="$1"
  cat >"$dest" <<'SCRIPT'
#!/usr/bin/env bash
set -euo pipefail

ipalpha_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
infra="$(sed -n 's/^IPALPHA_INFRA_NAME=//p' "$ipalpha_dir/ports.env" 2>/dev/null | head -n1)"
infra="${infra:-ipalpha}"
runtime="$(cat "$ipalpha_dir/.state/runtime" 2>/dev/null || echo docker)"

if [[ "$runtime" == "container" ]]; then
  pids=()
  trap 'kill "${pids[@]}" 2>/dev/null || true' EXIT INT TERM
  for name in "$infra-mongo" "$infra-redis" "$infra-rabbitmq" "$infra-mailpit"; do
    container logs --follow -n 100 "$name" 2>&1 | sed -u "s/^/$(printf '%-17s' "$name")| /" &
    pids+=("$!")
  done
  wait
  exit 0
fi

exec docker compose --env-file "$ipalpha_dir/.env" --env-file "$ipalpha_dir/ports.env" \
  -f "$ipalpha_dir/compose.yaml" logs --follow --tail=100
SCRIPT
  chmod +x "$dest"
}

ipalpha_write_bin_infra_down() {
  local dest="$1"
  cat >"$dest" <<'SCRIPT'
#!/usr/bin/env bash
set -euo pipefail

ipalpha_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
infra="$(sed -n 's/^IPALPHA_INFRA_NAME=//p' "$ipalpha_dir/ports.env" 2>/dev/null | head -n1)"
infra="${infra:-ipalpha}"
purge=false volumes=false
for arg in "$@"; do
  case "$arg" in
    --purge) purge=true ;;
    --volumes) volumes=true ;;
  esac
done

if command -v docker >/dev/null 2>&1 && docker info >/dev/null 2>&1 && [[ -f "$ipalpha_dir/compose.yaml" ]]; then
  compose=(docker compose --env-file "$ipalpha_dir/.env" --env-file "$ipalpha_dir/ports.env" -f "$ipalpha_dir/compose.yaml")
  if [[ "$purge" == true && "$volumes" == true ]]; then
    "${compose[@]}" down --volumes >/dev/null 2>&1 || true
  elif [[ "$purge" == true ]]; then
    "${compose[@]}" down >/dev/null 2>&1 || true
  else
    "${compose[@]}" stop >/dev/null 2>&1 || true
  fi
fi

if command -v container >/dev/null 2>&1; then
  for name in "$infra-mongo" "$infra-redis" "$infra-rabbitmq" "$infra-mailpit"; do
    container stop "$name" >/dev/null 2>&1 || true
    if [[ "$purge" == true ]]; then
      container delete --force "$name" >/dev/null 2>&1 || true
      rm -f "$ipalpha_dir/.state/$name.spec"
      [[ "$volumes" == true ]] && container volume delete "$name-data" >/dev/null 2>&1 || true
    fi
  done
fi
echo "infra stopped"
SCRIPT
  chmod +x "$dest"
}

ipalpha_write_bin_wait_for_http() {
  local dest="$1"
  cat >"$dest" <<'SCRIPT'
#!/usr/bin/env bash
set -euo pipefail

url="${1:?usage: wait-for-http URL [timeout-sec]}"
timeout="${2:-120}"

probe() {
  if command -v curl >/dev/null 2>&1; then
    curl -s -o /dev/null --max-time 2 "$url"
    return $?
  fi
  local host port
  host="$(sed -E 's|^https?://([^:/]+).*|\1|' <<<"$url")"
  port="$(sed -E 's|^https?://[^:/]+:([0-9]+).*|\1|' <<<"$url")"
  [[ "$url" == https://* ]] && port="${port:-443}" || port="${port:-80}"
  (exec 3<>"/dev/tcp/$host/$port") 2>/dev/null && { exec 3>&- 3<&-; return 0; }
  return 1
}

for _ in $(seq 1 "$timeout"); do
  if probe; then
    exit 0
  fi
  sleep 1
done
echo "wait-for-http: timeout waiting for $url" >&2
exit 1
SCRIPT
  chmod +x "$dest"
}

ipalpha_write_mprocs_yaml() {
  local root="$1" dest="$2"
  local projects_port notification_port repo cwd
  projects_port="$(ipalpha_settings_ms_port projects-api)"
  notification_port="$(ipalpha_settings_ms_port notifications-api)"
  {
    echo "proc_list_width: 36"
    echo "scrollback: 10000"
    echo "procs:"
    for repo in "${ipalpha_ms_order[@]}"; do
      echo "  \"MS · $repo\":"
      echo "    cwd: \"$(ipalpha_repo_path "$root" "$repo")\""
      case "$repo" in
        projects-api) echo "    shell: \"$root/.ipalpha/bin/node-dev $repo\"" ;;
        auth-api) echo "    shell: \"$root/.ipalpha/bin/node-dev $repo $projects_port $notification_port\"" ;;
        *) echo "    shell: \"$root/.ipalpha/bin/node-dev $repo $projects_port\"" ;;
      esac
    done
    for repo in "${ipalpha_web_repos[@]}"; do
      [[ -f "$(ipalpha_repo_path "$root" "$repo")/package.json" ]] || continue
      echo "  \"Web · $repo\":"
      echo "    cwd: \"$(ipalpha_repo_path "$root" "$repo")\""
      echo "    shell: \"$root/.ipalpha/bin/web-dev $repo\""
      if ipalpha_app_selected "$repo" "$root"; then
        echo "    autostart: true"
      else
        echo "    autostart: false"
      fi
    done
  } >"$dest"
}

# Standalone web app (core/<repo> or apps/<app>/<repo>, own vite.config): Vite on its port, /api proxied.
ipalpha_write_bin_web_dev() {
  local dest="$1"
  cat >"$dest" <<'SCRIPT'
#!/usr/bin/env bash
set -euo pipefail

repo="${1:?usage: web-dev <repo>}"

ipalpha_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
ipalpha_root="$(cd "$ipalpha_dir/.." && pwd)"
# shellcheck disable=SC1091
source "$ipalpha_dir/lib/common.sh"
app="$(ipalpha_repo_path "$ipalpha_root" "$repo")"

if [[ ! -f "$app/package.json" ]]; then
  echo "web-dev: $repo is not cloned yet — nothing to run"
  exit 0
fi

setting() {
  sed -n "s/^$1=//p" "$ipalpha_dir/settings" 2>/dev/null | head -n1
}

backend="$(ipalpha_web_api_backend "$repo")"
api_port="$(setting "${backend}_port")"
api_port="${api_port:-$(ipalpha_default_ms_port "$backend")}"
web_port="$(setting "${repo}_port")"
web_port="${web_port:-$(ipalpha_default_web_port "$repo")}"

cd "$app"
if [[ ! -d node_modules || package-lock.json -nt node_modules ]]; then
  npm install --no-audit --no-fund --silent
  touch node_modules
fi

# Every core API URL, so any standalone app's Vite proxy can mount /api/<name> (same-origin).
ms_url() {
  local port
  port="$(setting "${1}_port")"
  echo "http://127.0.0.1:${port:-$(ipalpha_default_ms_port "$1")}"
}
export AUTH_API_URL="$(ms_url auth-api)" PROJECTS_API_URL="$(ms_url projects-api)" PERSONS_API_URL="$(ms_url persons-api)"
export ORGANIZATIONS_API_URL="$(ms_url organizations-api)" NOTIFICATIONS_API_URL="$(ms_url notifications-api)"
export FORMS_API_URL="$(ms_url forms-api)" DISPATCH_API_URL="$(ms_url dispatch-api)"
export AI_API_URL="$(ms_url ai-api)" DEVELOPERS_API_URL="$(ms_url developers-api)"
[[ "$repo" != auth-webapp ]] || export AUTH_API_URL="http://127.0.0.1:$api_port"
exec node "$ipalpha_dir/bin/web-dev.mjs" "$web_port" "${@:2}"
SCRIPT
  chmod +x "$dest"
}

ipalpha_write_bin_node_dev() {
  local dest="$1"
  cat >"$dest" <<'SCRIPT'
#!/usr/bin/env bash
set -euo pipefail

repo="${1:?usage: node-dev <repo> [dep-port ...]}"
[[ $# -gt 0 ]] && shift

ipalpha_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
ipalpha_root="$(cd "$ipalpha_dir/.." && pwd)"
# shellcheck disable=SC1091
source "$ipalpha_dir/lib/common.sh"
repo_dir="$(ipalpha_repo_path "$ipalpha_root" "$repo")"

if [[ ! -f "$repo_dir/package.json" ]]; then
  echo "node-dev: $repo has no package.json yet — nothing to run"
  exit 0
fi

cd "$repo_dir"
if [[ ! -d node_modules ]]; then
  npm install --no-audit --no-fund --silent
fi

if [[ -f .env ]]; then
  while IFS= read -r line || [[ -n "$line" ]]; do
    [[ "$line" =~ ^[A-Za-z_][A-Za-z0-9_]*= ]] || continue
    key="${line%%=*}"
    val="${line#*=}"
    if [[ "$val" =~ ^\'(.*)\'$ || "$val" =~ ^\"(.*)\"$ ]]; then
      val="${BASH_REMATCH[1]}"
    fi
    export "$key=$val"
  done <.env
fi

# Dependency ports may still be passed for context, but nothing waits: every MS boots at
# once and answers GET /ready with 503 until its infra is good.
[[ $# -gt 0 ]] && echo "node-dev: $repo uses ports $* — starting without waiting (see /ready)"

script="$(node -p "const s=require('./package.json').scripts||{}; s['start:dev']?'start:dev':(s['dev']?'dev':'start')")"
exec npm run --silent "$script"
SCRIPT
  chmod +x "$dest"
}

ipalpha_write_bin_install_deps() {
  local dest="$1"
  cat >"$dest" <<'SCRIPT'
#!/usr/bin/env bash
set -euo pipefail

ipalpha_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
ipalpha_root="$(cd "$ipalpha_dir/.." && pwd)"
jobs="${IPALPHA_INSTALL_JOBS:-6}"
shared=("$ipalpha_root/core/shared-js" "$ipalpha_root/core/shared-ui")

needs_install() {
  [[ -f "$1/package.json" ]] || return 1
  [[ ! -d "$1/node_modules" || "$1/package-lock.json" -nt "$1/node_modules" ]] && return 0
  local lib
  for lib in shared-js shared-ui; do
    [[ -d "$1/node_modules/@ipalpha/$lib" && ! -d "$1/node_modules/@ipalpha/$lib/dist" ]] && return 0
  done
  return 1
}

needs_build() {
  [[ -f "$1/package.json" ]] && [[ ! -d "$1/dist" || -n "$(find "$1/src" -newer "$1/dist" -type f 2>/dev/null | head -n1)" ]]
}

log_dir="$ipalpha_dir/.state/install-deps"
mkdir -p "$log_dir"

run_tasks() {
  (( $# )) || return 0
  printf '%s\0' "$@" | IPALPHA_ROOT="$ipalpha_root" LOG_DIR="$log_dir" xargs -0 -n1 -P "$jobs" bash -c '
    task="${1%% *}" dir="${1#* }"
    name="${dir#"$IPALPHA_ROOT"/}"
    log="$LOG_DIR/${name//\//_}.log"
    echo "install-deps: $task $name"
    if [[ "$task" == install ]]; then
      rm -rf "$dir/node_modules/@ipalpha/shared-js" "$dir/node_modules/@ipalpha/shared-ui"
      (cd "$dir" && npm install --no-audit --no-fund && touch node_modules) >"$log" 2>&1
    else
      (cd "$dir" && npm run build && touch dist) >"$log" 2>&1
    fi && exit 0
    printf "\ninstall-deps: %s %s FAILED\n%s\n  full log: %s\n\n" "$task" "$name" \
      "$(grep -v "^npm error *$" "$log" | tail -n 25 | sed "s/^/  │ /")" "$log" >&2
    exit 1
  ' _ || { echo "install-deps: stopped — fix the error above and run ./run again." >&2; exit 1; }
}

# Consumers copy shared-js/shared-ui (install-links=true), so both must be built before any consumer installs.
tasks=()
for dir in "${shared[@]}"; do
  if needs_install "$dir"; then tasks+=("install $dir"); fi
done
run_tasks ${tasks[@]+"${tasks[@]}"}

tasks=()
for dir in "${shared[@]}"; do
  if needs_build "$dir"; then tasks+=("build $dir"); fi
done
run_tasks ${tasks[@]+"${tasks[@]}"}

tasks=()
for dir in "$ipalpha_root"/core/*/ "$ipalpha_root"/apps/*/*/; do
  dir="${dir%/}"
  [[ "$dir" == "${shared[0]}" || "$dir" == "${shared[1]}" ]] && continue
  if needs_install "$dir"; then tasks+=("install $dir"); fi
done
run_tasks ${tasks[@]+"${tasks[@]}"}

# A rebuilt shared library reaches consumers that copied it without a full reinstall.
for lib in "${shared[@]}"; do
  [[ -d "$lib/dist" ]] || continue
  for copy in "$ipalpha_root"/core/*/node_modules/@ipalpha/"$(basename "$lib")" "$ipalpha_root"/apps/*/*/node_modules/@ipalpha/"$(basename "$lib")"; do
    [[ -d "$copy/dist" && ! -L "$copy" ]] || continue
    [[ -n "$(find "$lib/dist" -newer "$copy/dist" -type f 2>/dev/null | head -n1)" ]] || continue
    rm -rf "$copy/dist" && cp -R "$lib/dist" "$copy/dist" && cp "$lib/package.json" "$copy/package.json"
    echo "install-deps: refresh $(basename "$lib") in ${copy#"$ipalpha_root"/}"
  done
done
SCRIPT
  chmod +x "$dest"
}

ipalpha_write_bin_fallback_run() {
  local dest="$1"
  cat >"$dest" <<'SCRIPT'
#!/usr/bin/env bash
set -euo pipefail

ipalpha_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
log_dir="${TMPDIR:-/tmp}/ipalpha-run-logs"
mkdir -p "$log_dir"

setting() {
  sed -n "s/^$1=//p" "$ipalpha_dir/settings" 2>/dev/null | head -n1
}

projects_port="$(setting 'projects-api_port')"
auth_port="$(setting 'auth-api_port')"

pids=()
cleanup() {
  local p
  trap - EXIT INT TERM
  for p in "${pids[@]:-}"; do
    [[ -n "$p" ]] && kill -TERM -- "-$p" 2>/dev/null || true
  done
  "$ipalpha_dir/bin/infra-down" >/dev/null 2>&1 || true
}
trap cleanup EXIT INT TERM
set -m

start() {
  local repo="$1"; shift
  (exec "$ipalpha_dir/bin/node-dev" "$repo" "$@") >"$log_dir/$repo.log" 2>&1 &
  pids+=("$!")
  echo "started $repo (pid $!) → $log_dir/$repo.log"
}

# shellcheck disable=SC1091
source "$ipalpha_dir/lib/common.sh"
for repo in "${ipalpha_ms_order[@]}"; do
  [[ -f "$(ipalpha_repo_path "$ipalpha_dir/.." "$repo")/package.json" ]] || continue
  case "$repo" in
    organizations-api) start "$repo" "$auth_port" "$projects_port" ;;
    *) start "$repo" ;;
  esac
done


selected_apps="$(setting browser_apps)"
if ! grep -q '^browser_apps=' "$ipalpha_dir/settings"; then
  selected_apps="$ipalpha_default_browser_apps"
fi
for repo in "${ipalpha_web_repos[@]}"; do
  [[ " $selected_apps " == *" $repo "* ]] || continue
  [[ -f "$(ipalpha_repo_path "$ipalpha_dir/.." "$repo")/package.json" ]] || continue
  web_port="$(setting "${repo}_port")"
  (exec "$ipalpha_dir/bin/web-dev" "$repo") >"$log_dir/$repo.log" 2>&1 &
  pids+=("$!")
  echo "started $repo → http://localhost:${web_port:-$(ipalpha_default_web_port "$repo")}/"
done

echo
echo "all services starting — logs in $log_dir"
echo "  tail -f $log_dir/*.log"
echo "press Ctrl+C to stop"
wait
SCRIPT
  chmod +x "$dest"
}

ipalpha_write_projects_json() {
  local root="$1" dest="$2"
  local repo first=true display port autostart group
  {
    echo "{"
    echo "  \"root\": \"$root\","
    echo "  \"lang\": \"${ipalpha_lang:-pt-BR}\","
    echo "  \"infra\": {"
    echo "    \"start\": \"$root/.ipalpha/bin/infra-up\","
    echo "    \"logs\": \"$root/.ipalpha/bin/infra-logs\","
    echo "    \"stop\": \"$root/.ipalpha/bin/infra-down\""
    echo "  },"
    echo "  \"projects\": ["
    for repo in "${ipalpha_ms_order[@]}"; do
      display="${repo%-api}"
      display="$(tr '[:lower:]' '[:upper:]' <<<"${display:0:1}")${display:1}"
      [[ "$repo" != ai-api ]] || display="AI"
      group="$(ipalpha_app_of "$repo" || true)"
      [[ -z "$group" ]] || display="$display API"
      port="$(ipalpha_settings_ms_port "$repo")"
      [[ "$first" == true ]] || echo "    ,"
      first=false
      echo "    {\"name\": \"$repo\", \"kind\": \"service\", \"group\": \"$group\", \"path\": \"$(ipalpha_repo_rel "$repo")\", \"display\": \"$display\", \"port\": \"$port\", \"autostart\": true}"
    done
    for repo in "${ipalpha_web_repos[@]}"; do
      [[ -f "$(ipalpha_repo_path "$root" "$repo")/package.json" ]] || continue
      display="${repo%-webapp}"
      display="$(tr '[:lower:]' '[:upper:]' <<<"${display:0:1}")${display:1} Web"
      port="$(ipalpha_settings_web_port "$repo")"
      autostart=false
      if ipalpha_app_selected "$repo" "$root"; then autostart=true; fi
      group="$(ipalpha_app_of "$repo" || true)"
      echo "    ,"
      echo "    {\"name\": \"$repo\", \"kind\": \"app\", \"group\": \"$group\", \"path\": \"$(ipalpha_repo_rel "$repo")\", \"display\": \"$display\", \"port\": \"$port\", \"autostart\": $autostart, \"cmd\": \"$root/.ipalpha/bin/web-dev $repo\", \"frontend\": \"http://localhost:$port/\"}"
    done
    echo "    ,"
    port="${ipalpha_port_mailpit:-$ipalpha_default_mailpit_port}"
    echo "    {\"name\": \"mailpit\", \"kind\": \"browser\", \"display\": \"Mailpit\", \"port\": \"$port\", \"autostart\": false, \"frontend\": \"http://127.0.0.1:$port/\"}"
    echo "  ]"
    echo "}"
  } >"$dest"
}

ipalpha_procs_asset() {
  local os arch
  os="$(uname -s | tr '[:upper:]' '[:lower:]')"
  arch="$(uname -m)"
  case "$arch" in
    x86_64|amd64) arch=amd64 ;;
    arm64|aarch64) arch=arm64 ;;
  esac
  echo "ipalpha-procs-$os-$arch"
}

ipalpha_install_procs() {
  local setup_root="$1" dest="$2"
  local url
  if [[ -n "${IPALPHA_PROCS_BINARY:-}" && -x "$IPALPHA_PROCS_BINARY" ]]; then
    cp "$IPALPHA_PROCS_BINARY" "$dest" && chmod +x "$dest" && return 0
  fi
  if [[ -f "$setup_root/templates/procs/main.go" ]] && command -v go >/dev/null 2>&1; then
    if (cd "$setup_root/templates/procs" && CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o "$dest" .) >/dev/null 2>&1; then
      return 0
    fi
  fi
  url="https://github.com/${ipalpha_org}/${ipalpha_tooling_repo}/releases/download/procs-latest/$(ipalpha_procs_asset)"
  if curl -fsSL -o "$dest.tmp" "$url" 2>/dev/null; then
    mv "$dest.tmp" "$dest"
    chmod +x "$dest"
    return 0
  fi
  rm -f "$dest.tmp"
  echo "  $(ipalpha_msg procs_missing)"
  return 1
}

ipalpha_write_root_run() {
  local root="$1"
  cat >"$root/run" <<'SCRIPT'
#!/usr/bin/env bash
set -euo pipefail

ipalpha_root="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ipalpha_dir="$ipalpha_root/.ipalpha"

# shellcheck disable=SC1091
source "$ipalpha_dir/lib/i18n.sh"
# shellcheck disable=SC1091
source "$ipalpha_dir/lib/common.sh"
# shellcheck disable=SC1091
source "$ipalpha_dir/lib/settings.sh"
# shellcheck disable=SC1091
source "$ipalpha_dir/lib/env.sh"
# shellcheck disable=SC1091
source "$ipalpha_dir/lib/ports.sh"
ipalpha_load_settings "$ipalpha_root" 2>/dev/null || ipalpha_i18n_init pt-BR

case "${1:-}" in
  -h|--help|help)
    ipalpha_msg help_run
    exit 0
    ;;
  stop)
    exec "$ipalpha_dir/bin/infra-down" "${@:2}"
    ;;
  apps|browsers)
    exec node "$ipalpha_dir/bin/browser-dev.mjs" "$ipalpha_dir" "${@:2}"
    ;;
esac

runtime="${ipalpha_runtime:-container}"
if [[ "$runtime" == "docker" ]]; then
  command -v docker >/dev/null 2>&1 || { echo "docker not installed." >&2; exit 1; }
  docker info >/dev/null 2>&1 || { echo "docker daemon is not running." >&2; exit 1; }
  docker compose version >/dev/null 2>&1 || { echo "docker compose v2 is required." >&2; exit 1; }
fi

echo "$(ipalpha_msg run_infra)"
"$ipalpha_dir/bin/infra-up"
"$ipalpha_dir/bin/install-deps"
ipalpha_prepare_local_envs "$ipalpha_root" "$ipalpha_dir/env-fallback"
"$ipalpha_dir/bin/auth-keys-bootstrap"
if synced="$(node "$ipalpha_dir/lib/local-clients.mjs" "$ipalpha_root" 2>&1)"; then
  [[ -z "$synced" ]] || echo "$(ipalpha_msg clients_synced) $synced"
else
  echo "$(ipalpha_msg clients_sync_failed) $synced" >&2
fi

# Browser readiness is independent of service boot and works with every runner.
# The bounded helper exits if this launcher (exec'd runner) exits.
node "$ipalpha_dir/bin/browser-dev.mjs" "$ipalpha_dir" watch >"$ipalpha_dir/.state/browser.log" 2>&1 &

runner="${IPALPHA_RUNNER:-${ipalpha_runner:-auto}}"
case "$runner" in
  auto|panel)
    if [[ -t 1 && -x "$ipalpha_dir/bin/ipalpha-procs" ]]; then
      exec "$ipalpha_dir/bin/ipalpha-procs" "$ipalpha_dir"
    fi
    ;;
  mprocs)
    if command -v mprocs >/dev/null 2>&1; then
      # Refresh remembered app autostart flags on every run, not just setup/pull.
      source "$ipalpha_dir/lib/generate.sh"
      ipalpha_write_mprocs_yaml "$ipalpha_root" "$ipalpha_dir/mprocs.yaml"
      exec mprocs --config "$ipalpha_dir/mprocs.yaml"
    fi
    ;;
esac

exec "$ipalpha_dir/bin/fallback-run"
SCRIPT
  chmod +x "$root/run"
}

ipalpha_write_root_pull() {
  local root="$1"
  cat >"$root/pull" <<'SCRIPT'
#!/usr/bin/env bash
set -euo pipefail

ipalpha_root="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ipalpha_dir="$ipalpha_root/.ipalpha"

if [[ ! -d "$ipalpha_dir/lib" ]]; then
  echo "missing $ipalpha_dir/lib — re-run setup." >&2
  exit 1
fi

# shellcheck disable=SC1091
source "$ipalpha_dir/lib/i18n.sh"
# shellcheck disable=SC1091
source "$ipalpha_dir/lib/common.sh"
# shellcheck disable=SC1091
source "$ipalpha_dir/lib/settings.sh"
# shellcheck disable=SC1091
source "$ipalpha_dir/lib/clone.sh"
# shellcheck disable=SC1091
source "$ipalpha_dir/lib/env.sh"
# shellcheck disable=SC1091
source "$ipalpha_dir/lib/ports.sh"
# shellcheck disable=SC1091
source "$ipalpha_dir/lib/generate.sh"
# shellcheck disable=SC1091
source "$ipalpha_dir/lib/update.sh"

ipalpha_load_settings "$ipalpha_root" 2>/dev/null || { echo "$(ipalpha_msg update_no_settings)" >&2; exit 1; }

case "${1:-}" in
  -h|--help|help)
    ipalpha_msg help_pull
    exit 0
    ;;
esac

ipalpha_update "$ipalpha_root"
SCRIPT
  chmod +x "$root/pull"
}

ipalpha_write_root_publish() {
  local root="$1"
  cat >"$root/publish" <<'SCRIPT'
#!/usr/bin/env bash
set -euo pipefail

ipalpha_root="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ipalpha_dir="$ipalpha_root/.ipalpha"

if [[ ! -d "$ipalpha_dir/lib" ]]; then
  echo "missing $ipalpha_dir/lib — re-run setup." >&2
  exit 1
fi

# shellcheck disable=SC1091
source "$ipalpha_dir/lib/i18n.sh"
# shellcheck disable=SC1091
source "$ipalpha_dir/lib/common.sh"
# shellcheck disable=SC1091
source "$ipalpha_dir/lib/settings.sh"
# shellcheck disable=SC1091
source "$ipalpha_dir/lib/publish.sh"

ipalpha_load_settings "$ipalpha_root" 2>/dev/null || ipalpha_i18n_init pt-BR

ipalpha_publish "$ipalpha_root" "$@"
SCRIPT
  chmod +x "$root/publish"
}

ipalpha_write_root_feature() {
  local root="$1"
  cat >"$root/feature" <<'SCRIPT'
#!/usr/bin/env bash
set -euo pipefail

ipalpha_root="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ipalpha_dir="$ipalpha_root/.ipalpha"

if [[ ! -f "$ipalpha_dir/lib/feature.sh" ]]; then
  echo "missing $ipalpha_dir/lib/feature.sh — run ./pull." >&2
  exit 1
fi

for lib in i18n common settings clone env ports generate publish feature; do
  # shellcheck disable=SC1090
  source "$ipalpha_dir/lib/$lib.sh"
done

ipalpha_load_settings "$ipalpha_root" 2>/dev/null || ipalpha_i18n_init pt-BR

ipalpha_feature "$ipalpha_root" "$@"
SCRIPT
  chmod +x "$root/feature"
}

ipalpha_materialize_workspace() {
  local setup_root="$1" target_root="$2"
  local dir="$target_root/.ipalpha"
  local lib f

  echo "$(ipalpha_msg writing_workspace)"
  mkdir -p "$dir/bin" "$dir/lib"
  # Setup re-runs and pulls also replace the generic name, so two workspaces never share one database.
  if [[ "${ipalpha_infra_name:-ipalpha}" == ipalpha ]]; then
    ipalpha_infra_name="$(ipalpha_workspace_infra_name "$target_root")"
  fi

  for lib in "$setup_root"/lib/*.sh; do
    [[ -f "$lib" ]] || continue
    cp "$lib" "$dir/lib/"
  done
  cp "$setup_root/lib/local-env.mjs" "$dir/lib/local-env.mjs"
  cp "$setup_root/lib/superuser.mjs" "$dir/lib/superuser.mjs"
  cp "$setup_root/lib/local-clients.mjs" "$dir/lib/local-clients.mjs"
  for f in "$dir"/lib/*.sh; do
    [[ -f "$f" ]] || continue
    chmod +x "$f"
  done

  rm -rf "$dir/env-fallback"
  cp -R "$setup_root/templates/env-fallback" "$dir/env-fallback"
  cp "$setup_root/templates/compose.yaml" "$dir/compose.yaml"
  cp "$setup_root/templates/system-clients.json" "$dir/system-clients.json"

  [[ -f "$dir/.env" ]] || ipalpha_write_compose_env "$dir/.env"
  ipalpha_write_ports_env "$dir/ports.env"

  ipalpha_write_bin_infra_up "$dir/bin/infra-up"
  ipalpha_write_bin_infra_logs "$dir/bin/infra-logs"
  ipalpha_write_bin_infra_down "$dir/bin/infra-down"
  ipalpha_write_bin_wait_for_http "$dir/bin/wait-for-http"
  ipalpha_write_bin_node_dev "$dir/bin/node-dev"
  rm -f "$dir/bin/vite-dev" "$dir/vite.dev.mjs"
  ipalpha_write_bin_web_dev "$dir/bin/web-dev"
  cp "$setup_root/templates/web-dev.mjs" "$dir/bin/web-dev.mjs"
  cp "$setup_root/templates/browser-dev.mjs" "$dir/bin/browser-dev.mjs"
  cp "$setup_root/templates/auth-keys-bootstrap" "$dir/bin/auth-keys-bootstrap"
  chmod +x "$dir/bin/auth-keys-bootstrap"
  ipalpha_write_bin_install_deps "$dir/bin/install-deps"
  rm -f "$dir/bin/build-shared-js"
  ipalpha_write_bin_fallback_run "$dir/bin/fallback-run"

  ipalpha_write_mprocs_yaml "$target_root" "$dir/mprocs.yaml"
  ipalpha_write_projects_json "$target_root" "$dir/projects.json"
  ipalpha_install_procs "$setup_root" "$dir/bin/ipalpha-procs" || true
  ipalpha_write_root_run "$target_root"
  ipalpha_write_root_pull "$target_root"
  ipalpha_write_root_publish "$target_root"
  ipalpha_write_root_feature "$target_root"

  echo "$(ipalpha_msg writing_settings)"
  ipalpha_write_settings "$target_root"
}
