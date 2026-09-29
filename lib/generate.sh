#!/usr/bin/env bash

ipalpha_write_ports_env() {
  local dest="$1"
  cat >"$dest" <<EOF
MONGO_HOST_PORT=${ipalpha_port_mongo:-$ipalpha_default_mongo_port}
REDIS_HOST_PORT=${ipalpha_port_redis:-$ipalpha_default_redis_port}
RABBITMQ_HOST_PORT=${ipalpha_port_rabbitmq:-$ipalpha_default_rabbitmq_port}
RABBITMQ_MGMT_HOST_PORT=${ipalpha_port_rabbitmq_mgmt:-$ipalpha_default_rabbitmq_mgmt_port}
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
  container network create ipalpha >/dev/null 2>&1 || true
  container volume create ipalpha-mongo-data >/dev/null 2>&1 || true
  container volume create ipalpha-redis-data >/dev/null 2>&1 || true
  container volume create ipalpha-rabbitmq-data >/dev/null 2>&1 || true

  ensure_container ipalpha-mongo \
    --network ipalpha \
    --publish "${MONGO_HOST_PORT:-27017}:27017" \
    --env "MONGO_INITDB_ROOT_USERNAME=${MONGO_USERNAME:-ipalpha}" \
    --env "MONGO_INITDB_ROOT_PASSWORD=${MONGO_PASSWORD:-ipalpha}" \
    --volume ipalpha-mongo-data:/data/db \
    mongo:8 || return 1

  ensure_container ipalpha-redis \
    --network ipalpha \
    --publish "${REDIS_HOST_PORT:-6379}:6379" \
    --volume ipalpha-redis-data:/data \
    redis:7-alpine --appendonly yes || return 1

  ensure_container ipalpha-rabbitmq \
    --network ipalpha \
    --publish "${RABBITMQ_HOST_PORT:-5672}:5672" \
    --publish "${RABBITMQ_MGMT_HOST_PORT:-15672}:15672" \
    --env "RABBITMQ_DEFAULT_USER=${RABBITMQ_USERNAME:-ipalpha}" \
    --env "RABBITMQ_DEFAULT_PASS=${RABBITMQ_PASSWORD:-ipalpha}" \
    --volume ipalpha-rabbitmq-data:/var/lib/rabbitmq \
    rabbitmq:4-management || return 1

  wait_ready ipalpha-mongo mongosh --quiet \
    --username "${MONGO_USERNAME:-ipalpha}" --password "${MONGO_PASSWORD:-ipalpha}" \
    --authenticationDatabase admin --eval 'db.adminCommand({ ping: 1 })'
  wait_ready ipalpha-redis redis-cli ping
  wait_ready ipalpha-rabbitmq rabbitmq-diagnostics -q ping
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
    for name in ipalpha-mongo ipalpha-redis ipalpha-rabbitmq; do
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
runtime="$(cat "$ipalpha_dir/.state/runtime" 2>/dev/null || echo docker)"

if [[ "$runtime" == "container" ]]; then
  pids=()
  trap 'kill "${pids[@]}" 2>/dev/null || true' EXIT INT TERM
  for name in ipalpha-mongo ipalpha-redis ipalpha-rabbitmq; do
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
purge=false
[[ "${1:-}" == "--purge" ]] && purge=true

if command -v docker >/dev/null 2>&1 && docker info >/dev/null 2>&1 && [[ -f "$ipalpha_dir/compose.yaml" ]]; then
  compose=(docker compose --env-file "$ipalpha_dir/.env" --env-file "$ipalpha_dir/ports.env" -f "$ipalpha_dir/compose.yaml")
  if [[ "$purge" == true ]]; then
    "${compose[@]}" down >/dev/null 2>&1 || true
  else
    "${compose[@]}" stop >/dev/null 2>&1 || true
  fi
fi

if command -v container >/dev/null 2>&1; then
  for name in ipalpha-mongo ipalpha-redis ipalpha-rabbitmq; do
    container stop "$name" >/dev/null 2>&1 || true
    if [[ "$purge" == true ]]; then
      container delete --force "$name" >/dev/null 2>&1 || true
      rm -f "$ipalpha_dir/.state/$name.spec"
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
  notification_port="$(ipalpha_settings_ms_port notification-api)"
  {
    echo "proc_list_width: 36"
    echo "scrollback: 10000"
    echo "procs:"
    for repo in "${ipalpha_ms_order[@]}"; do
      echo "  \"MS · $repo\":"
      echo "    cwd: \"$root/core/$repo\""
      case "$repo" in
        projects-api) echo "    shell: \"$root/.ipalpha/bin/node-dev $repo\"" ;;
        auth-api) echo "    shell: \"$root/.ipalpha/bin/node-dev $repo $projects_port $notification_port\"" ;;
        *) echo "    shell: \"$root/.ipalpha/bin/node-dev $repo $projects_port\"" ;;
      esac
    done
    for repo in "${ipalpha_web_repos[@]}"; do
      [[ -f "$root/core/$repo/package.json" ]] || continue
      echo "  \"Web · $repo\":"
      echo "    cwd: \"$root/core/$repo\""
      echo "    shell: \"$root/.ipalpha/bin/web-dev $repo\""
    done
  } >"$dest"
}

# Standalone web app (core/<repo>, own vite.config): Vite on its port, /api proxied to its backend MS.
ipalpha_write_bin_web_dev() {
  local dest="$1"
  cat >"$dest" <<'SCRIPT'
#!/usr/bin/env bash
set -euo pipefail

repo="${1:?usage: web-dev <repo>}"

ipalpha_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
ipalpha_root="$(cd "$ipalpha_dir/.." && pwd)"
app="$ipalpha_root/core/$repo"

# shellcheck disable=SC1091
source "$ipalpha_dir/lib/common.sh"

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

export AUTH_API_URL="http://127.0.0.1:$api_port"
exec ./node_modules/.bin/vite --port "$web_port" --strictPort
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
repo_dir="$ipalpha_root/core/$repo"

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

if [[ -f frontend/package.json && -n "${PORT:-}" ]]; then
  export IPALPHA_FRONTEND_URL="http://localhost:$((PORT + 2000))/frontend/"
fi

if [[ -z "${IPALPHA_PANEL:-}" ]]; then
  for port in "$@"; do
    "$ipalpha_dir/bin/wait-for-http" "http://127.0.0.1:$port" 60 || \
      echo "node-dev: dependency on port $port not up — starting anyway" >&2
  done
fi

script="$(node -p "const s=require('./package.json').scripts||{}; s['start:dev']?'start:dev':(s['dev']?'dev':'start')")"
exec npm run --silent "$script"
SCRIPT
  chmod +x "$dest"
}

ipalpha_write_bin_vite_dev() {
  local dest="$1"
  cat >"$dest" <<'SCRIPT'
#!/usr/bin/env bash
set -euo pipefail

repo="${1:?usage: vite-dev <repo> <web-port> <api-port>}"
web_port="${2:?web port}"
api_port="${3:?api port}"

ipalpha_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
ipalpha_root="$(cd "$ipalpha_dir/.." && pwd)"
frontend="$ipalpha_root/core/$repo/frontend"

if [[ ! -f "$frontend/package.json" ]]; then
  echo "vite-dev: $repo has no frontend — nothing to run"
  exit 0
fi

cd "$frontend"
if [[ ! -d node_modules || package-lock.json -nt node_modules ]]; then
  npm install --no-audit --no-fund --silent
  touch node_modules
fi

export IPALPHA_FRONTEND_ROOT="$frontend" IPALPHA_WEB_PORT="$web_port" IPALPHA_API_PORT="$api_port"
auth_port="$(sed -n 's/^auth-api_port=//p' "$ipalpha_dir/settings" 2>/dev/null | head -n1)"
if [[ "$repo" != "auth-api" && -z "${VITE_AUTH_API_URL:-}" ]]; then
  export VITE_AUTH_API_URL="http://localhost:$(( ${auth_port:-3005} + 2000 ))"
fi
exec ./node_modules/.bin/vite --config "$ipalpha_dir/vite.dev.mjs"
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

for dir in "$ipalpha_root"/core/*/; do
  dir="${dir%/}"
  [[ -f "$dir/package.json" ]] || continue
  if [[ ! -d "$dir/node_modules" || "$dir/package-lock.json" -nt "$dir/node_modules" ]]; then
    echo "install-deps: $(basename "$dir")"
    (cd "$dir" && npm install --no-audit --no-fund --silent && touch node_modules)
  fi
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

start projects-api
start person-api
start notification-api
start auth-api
start organization-api "$auth_port" "$projects_port"

for repo in projects-api person-api organization-api notification-api auth-api; do
  api_port="$(setting "${repo}_port")"
  [[ -f "$ipalpha_dir/../core/$repo/frontend/package.json" ]] || continue
  (exec "$ipalpha_dir/bin/vite-dev" "$repo" "$((api_port + 2000))" "$api_port") >"$log_dir/$repo-web.log" 2>&1 &
  pids+=("$!")
  echo "started $repo frontend → http://localhost:$((api_port + 2000))/frontend/"
done

# shellcheck disable=SC1091
source "$ipalpha_dir/lib/common.sh"
for repo in "${ipalpha_web_repos[@]}"; do
  [[ -f "$ipalpha_dir/../core/$repo/package.json" ]] || continue
  web_port="$(setting "${repo}_port")"
  (exec "$ipalpha_dir/bin/web-dev" "$repo") >"$log_dir/$repo.log" 2>&1 &
  pids+=("$!")
  echo "started $repo → http://localhost:${web_port:-$(ipalpha_default_web_port "$repo")}/frontend/"
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
  local repo first=true display port
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
      port="$(ipalpha_settings_ms_port "$repo")"
      [[ "$first" == true ]] || echo "    ,"
      first=false
      echo "    {\"name\": \"$repo\", \"kind\": \"service\", \"path\": \"core/$repo\", \"display\": \"$display\", \"port\": \"$port\", \"autostart\": true, \"frontend\": \"http://localhost:$((port + 2000))/frontend/\"}"
    done
    for repo in "${ipalpha_ms_order[@]}"; do
      [[ -f "$root/core/$repo/frontend/package.json" ]] || continue
      display="${repo%-api}"
      display="$(tr '[:lower:]' '[:upper:]' <<<"${display:0:1}")${display:1}"
      port="$(ipalpha_settings_ms_port "$repo")"
      echo "    ,"
      echo "    {\"name\": \"$repo-web\", \"kind\": \"attached\", \"parent\": \"$repo\", \"path\": \"core/$repo/frontend\", \"display\": \"Frontend\", \"port\": \"$((port + 2000))\", \"autostart\": true, \"cmd\": \"$root/.ipalpha/bin/vite-dev $repo $((port + 2000)) $port\", \"frontend\": \"http://localhost:$((port + 2000))/frontend/\"}"
    done
    for repo in "${ipalpha_web_repos[@]}"; do
      [[ -f "$root/core/$repo/package.json" ]] || continue
      display="${repo%-webapp}"
      display="$(tr '[:lower:]' '[:upper:]' <<<"${display:0:1}")${display:1} Web"
      port="$(ipalpha_settings_web_port "$repo")"
      echo "    ,"
      echo "    {\"name\": \"$repo\", \"kind\": \"app\", \"path\": \"core/$repo\", \"display\": \"$display\", \"port\": \"$port\", \"autostart\": true, \"cmd\": \"$root/.ipalpha/bin/web-dev $repo\", \"frontend\": \"http://localhost:$port/frontend/\"}"
    done
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
ipalpha_load_settings "$ipalpha_root" 2>/dev/null || ipalpha_i18n_init pt-BR

case "${1:-}" in
  -h|--help|help)
    ipalpha_msg help_run
    exit 0
    ;;
  stop)
    exec "$ipalpha_dir/bin/infra-down" "${@:2}"
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

runner="${IPALPHA_RUNNER:-${ipalpha_runner:-auto}}"
case "$runner" in
  auto|panel)
    if [[ -t 1 && -x "$ipalpha_dir/bin/ipalpha-procs" ]]; then
      exec "$ipalpha_dir/bin/ipalpha-procs" "$ipalpha_dir"
    fi
    ;;
  mprocs)
    if command -v mprocs >/dev/null 2>&1; then
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

ipalpha_materialize_workspace() {
  local setup_root="$1" target_root="$2"
  local dir="$target_root/.ipalpha"
  local lib f

  echo "$(ipalpha_msg writing_workspace)"
  mkdir -p "$dir/bin" "$dir/lib"

  for lib in "$setup_root"/lib/*.sh; do
    [[ -f "$lib" ]] || continue
    cp "$lib" "$dir/lib/"
  done
  for f in "$dir"/lib/*.sh; do
    [[ -f "$f" ]] || continue
    chmod +x "$f"
  done

  rm -rf "$dir/env-fallback"
  cp -R "$setup_root/templates/env-fallback" "$dir/env-fallback"
  cp "$setup_root/templates/compose.yaml" "$dir/compose.yaml"

  [[ -f "$dir/.env" ]] || ipalpha_write_compose_env "$dir/.env"
  ipalpha_write_ports_env "$dir/ports.env"

  ipalpha_write_bin_infra_up "$dir/bin/infra-up"
  ipalpha_write_bin_infra_logs "$dir/bin/infra-logs"
  ipalpha_write_bin_infra_down "$dir/bin/infra-down"
  ipalpha_write_bin_wait_for_http "$dir/bin/wait-for-http"
  ipalpha_write_bin_node_dev "$dir/bin/node-dev"
  ipalpha_write_bin_vite_dev "$dir/bin/vite-dev"
  ipalpha_write_bin_web_dev "$dir/bin/web-dev"
  cp "$setup_root/templates/vite.dev.mjs" "$dir/vite.dev.mjs"
  ipalpha_write_bin_install_deps "$dir/bin/install-deps"
  rm -f "$dir/bin/build-shared-js"
  ipalpha_write_bin_fallback_run "$dir/bin/fallback-run"

  ipalpha_write_mprocs_yaml "$target_root" "$dir/mprocs.yaml"
  ipalpha_write_projects_json "$target_root" "$dir/projects.json"
  ipalpha_install_procs "$setup_root" "$dir/bin/ipalpha-procs" || true
  ipalpha_write_root_run "$target_root"
  ipalpha_write_root_pull "$target_root"
  ipalpha_write_root_publish "$target_root"

  echo "$(ipalpha_msg writing_settings)"
  ipalpha_write_settings "$target_root"
}
