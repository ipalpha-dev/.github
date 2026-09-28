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

if [[ "$runtime" == "container" ]] && command -v container >/dev/null 2>&1; then
  infra_up_container() {
    container system start >/dev/null 2>&1 || true
    container network create ipalpha >/dev/null 2>&1 || true
    container volume create ipalpha-mongo-data >/dev/null 2>&1 || true
    container volume create ipalpha-redis-data >/dev/null 2>&1 || true
    container volume create ipalpha-rabbitmq-data >/dev/null 2>&1 || true

    reset_container() {
      container kill "$1" >/dev/null 2>&1 || true
      container rm "$1" >/dev/null 2>&1 || true
    }

    reset_container ipalpha-mongo
    container run --detach --name ipalpha-mongo \
      --network ipalpha \
      --publish "${MONGO_HOST_PORT:-27017}:27017" \
      --env "MONGO_INITDB_ROOT_USERNAME=${MONGO_USERNAME:-ipalpha}" \
      --env "MONGO_INITDB_ROOT_PASSWORD=${MONGO_PASSWORD:-ipalpha}" \
      --volume ipalpha-mongo-data:/data/db \
      mongo:8 || return 1

    reset_container ipalpha-redis
    container run --detach --name ipalpha-redis \
      --network ipalpha \
      --publish "${REDIS_HOST_PORT:-6379}:6379" \
      --volume ipalpha-redis-data:/data \
      redis:7-alpine --appendonly yes || return 1

    reset_container ipalpha-rabbitmq
    container run --detach --name ipalpha-rabbitmq \
      --network ipalpha \
      --publish "${RABBITMQ_HOST_PORT:-5672}:5672" \
      --publish "${RABBITMQ_MGMT_HOST_PORT:-15672}:15672" \
      --env "RABBITMQ_DEFAULT_USER=${RABBITMQ_USERNAME:-ipalpha}" \
      --env "RABBITMQ_DEFAULT_PASS=${RABBITMQ_PASSWORD:-ipalpha}" \
      --volume ipalpha-rabbitmq-data:/var/lib/rabbitmq \
      rabbitmq:4-management || return 1

    wait_ready() {
      local name="$1"; shift
      local i
      for i in $(seq 1 60); do
        if container exec "$name" "$@" >/dev/null 2>&1; then
          echo "  $name ready"
          return 0
        fi
        sleep 2
      done
      echo "  warning: $name not healthy yet (continuing)" >&2
      return 0
    }

    wait_ready ipalpha-mongo mongosh --quiet \
      --username "${MONGO_USERNAME:-ipalpha}" --password "${MONGO_PASSWORD:-ipalpha}" \
      --authenticationDatabase admin --eval 'db.adminCommand({ ping: 1 })'
    wait_ready ipalpha-redis redis-cli ping
    wait_ready ipalpha-rabbitmq rabbitmq-diagnostics -q ping
  }

  if infra_up_container; then
    exit 0
  fi
  if command -v docker >/dev/null 2>&1; then
    echo "warning: Apple container failed — falling back to Docker" >&2
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
docker compose --env-file "$ipalpha_dir/.env" --env-file "$ipalpha_dir/ports.env" \
  -f "$ipalpha_dir/compose.yaml" up --detach --wait
SCRIPT
  chmod +x "$dest"
}

ipalpha_write_bin_infra_down() {
  local dest="$1"
  cat >"$dest" <<'SCRIPT'
#!/usr/bin/env bash
set -euo pipefail

ipalpha_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

if command -v docker >/dev/null 2>&1 && [[ -f "$ipalpha_dir/compose.yaml" ]]; then
  docker compose --env-file "$ipalpha_dir/.env" --env-file "$ipalpha_dir/ports.env" \
    -f "$ipalpha_dir/compose.yaml" down >/dev/null 2>&1 || true
fi

if command -v container >/dev/null 2>&1; then
  for name in ipalpha-mongo ipalpha-redis ipalpha-rabbitmq; do
    container kill "$name" >/dev/null 2>&1 || true
    container rm "$name" >/dev/null 2>&1 || true
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

for port in "$@"; do
  "$ipalpha_dir/bin/wait-for-http" "http://127.0.0.1:$port" 180 || \
    echo "node-dev: dependency on port $port not up — starting anyway" >&2
done

script="$(node -p "const s=require('./package.json').scripts||{}; s['start:dev']?'start:dev':(s['dev']?'dev':'start')")"
exec npm run --if-present "$script"
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

for repo in projects-api person-api organization-api notification-api auth-api; do
  dir="$ipalpha_root/core/$repo"
  [[ -f "$dir/package.json" ]] || continue
  if [[ ! -d "$dir/node_modules" || "$dir/package-lock.json" -nt "$dir/node_modules" ]]; then
    echo "install-deps: $repo"
    (cd "$dir" && npm install --no-audit --no-fund --silent)
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
ipalpha_root="$(cd "$ipalpha_dir/.." && pwd)"
log_dir="${TMPDIR:-/tmp}/ipalpha-run-logs"
mkdir -p "$log_dir"

setting() {
  sed -n "s/^$1=//p" "$ipalpha_dir/settings" 2>/dev/null | head -n1
}

projects_port="$(setting 'projects-api_port')"
notification_port="$(setting 'notification-api_port')"

"$ipalpha_dir/bin/infra-up"
"$ipalpha_dir/bin/install-deps"

pids=()
cleanup() {
  local p
  for p in "${pids[@]:-}"; do
    kill "$p" 2>/dev/null || true
  done
}
trap cleanup EXIT INT TERM

start() {
  local repo="$1"; shift
  (exec "$ipalpha_dir/bin/node-dev" "$repo" "$@") >"$log_dir/$repo.log" 2>&1 &
  pids+=("$!")
  echo "started $repo (pid $!) → $log_dir/$repo.log"
}

start projects-api
start person-api "$projects_port"
start organization-api "$projects_port"
start notification-api "$projects_port"
start auth-api "$projects_port" "$notification_port"

echo
echo "all services starting — logs in $log_dir"
echo "press Ctrl+C to stop"
wait
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
  } >"$dest"
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
esac

runtime="${ipalpha_runtime:-container}"
if [[ "$runtime" == "docker" ]]; then
  command -v docker >/dev/null 2>&1 || { echo "docker not installed." >&2; exit 1; }
  docker info >/dev/null 2>&1 || { echo "docker daemon is not running." >&2; exit 1; }
  docker compose version >/dev/null 2>&1 || { echo "docker compose v2 is required." >&2; exit 1; }
fi

"$ipalpha_dir/bin/infra-up"
"$ipalpha_dir/bin/install-deps"

runner="${ipalpha_runner:-background}"
runner="${IPALPHA_RUNNER:-$runner}"
case "$runner" in
  mprocs|background) ;;
  *) runner="background" ;;
esac

# mprocs is opt-in only: install it yourself and run with IPALPHA_RUNNER=mprocs.
# Default is the embedded background runner (logs under $TMPDIR/ipalpha-run-logs).
if [[ "$runner" == "mprocs" ]] && command -v mprocs >/dev/null 2>&1; then
  exec mprocs --config "$ipalpha_dir/mprocs.yaml"
fi

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
  ipalpha_write_bin_infra_down "$dir/bin/infra-down"
  ipalpha_write_bin_wait_for_http "$dir/bin/wait-for-http"
  ipalpha_write_bin_node_dev "$dir/bin/node-dev"
  ipalpha_write_bin_install_deps "$dir/bin/install-deps"
  rm -f "$dir/bin/build-shared-js"
  ipalpha_write_bin_fallback_run "$dir/bin/fallback-run"

  ipalpha_write_mprocs_yaml "$target_root" "$dir/mprocs.yaml"
  ipalpha_write_root_run "$target_root"
  ipalpha_write_root_pull "$target_root"
  ipalpha_write_root_publish "$target_root"

  echo "$(ipalpha_msg writing_settings)"
  ipalpha_write_settings "$target_root"
}
