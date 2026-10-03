#!/usr/bin/env bash

ipalpha_org="ipalpha-dev"
ipalpha_registry="${IPALPHA_REGISTRY:-registry.kevyn.com.br/ip-alpha/core}"

ipalpha_ms_repos=(shared-js shared-ui projects-api persons-api organizations-api notifications-api auth-api forms-api dispatch-api)
ipalpha_ms_order=(projects-api persons-api organizations-api notifications-api auth-api forms-api dispatch-api)
# Standalone web apps under core/: cloned, deps installed, image published, no .env, run with Vite.
ipalpha_web_repos=(auth-webapp forms-webapp mordomia-webapp)
ipalpha_root_repos=(deployment)
ipalpha_tooling_repo=".github"
# Compose project / container prefix; feature workspaces use ipalpha-<slug> so they run side by side.
ipalpha_infra_name="${ipalpha_infra_name:-ipalpha}"

ipalpha_default_mongo_port=27017
ipalpha_default_redis_port=6379
ipalpha_default_rabbitmq_port=5672
ipalpha_default_rabbitmq_mgmt_port=15672

ipalpha_default_ms_port() {
  case "$1" in
    projects-api) echo 3001 ;;
    persons-api) echo 3002 ;;
    organizations-api) echo 3003 ;;
    notifications-api) echo 3004 ;;
    auth-api) echo 3005 ;;
    forms-api) echo 3006 ;;
    dispatch-api) echo 3007 ;;
    *) echo 3000 ;;
  esac
}

# Vite port for a standalone web app; its /api is proxied to this backend.
ipalpha_default_web_port() {
  case "$1" in
    auth-webapp) echo 5100 ;;
    forms-webapp) echo 5106 ;;
    mordomia-webapp) echo 5110 ;;
    *) echo 5199 ;;
  esac
}

ipalpha_web_api_backend() {
  case "$1" in
    auth-webapp) echo auth-api ;;
    forms-webapp) echo forms-api ;;
    mordomia-webapp) echo persons-api ;;
    *) echo "" ;;
  esac
}

ipalpha_settings_name=".ipalpha/settings"

ipalpha_settings_file() {
  echo "$1/${ipalpha_settings_name}"
}

ipalpha_repo_path() {
  local root="$1" repo="$2"
  case " ${ipalpha_root_repos[*]} " in
    *" $repo "*) echo "$root/$repo" ;;
    *) echo "$root/core/$repo" ;;
  esac
}

# Worktrees have a .git *file*, so never test -d "$dir/.git". The -e guard keeps a plain folder
# nested in some other repository from answering for its parent.
ipalpha_is_git_repo() {
  [[ -e "$1/.git" ]] && git -C "$1" rev-parse --git-dir >/dev/null 2>&1
}

ipalpha_is_ms_repo() {
  local repo="$1"
  case " ${ipalpha_ms_repos[*]} ${ipalpha_web_repos[*]} " in
    *" $repo "*) return 0 ;;
    *) return 1 ;;
  esac
}

ipalpha_is_web_repo() {
  local repo="$1"
  case " ${ipalpha_web_repos[*]} " in
    *" $repo "*) return 0 ;;
    *) return 1 ;;
  esac
}

ipalpha_is_image_repo() {
  case "$1" in
    shared-js|shared-ui) return 1 ;;
    *) return 0 ;;
  esac
}

ipalpha_all_repos() {
  local repo
  for repo in "${ipalpha_root_repos[@]}" "${ipalpha_ms_repos[@]}" "${ipalpha_web_repos[@]}"; do
    echo "$repo"
  done
}

ipalpha_shasum() {
  if command -v shasum >/dev/null 2>&1; then
    shasum "$@" | awk '{print $1}'
  else
    sha256sum "$@" | awk '{print $1}'
  fi
}

ipalpha_sed_inplace() {
  local file="$1"; shift
  if sed --version >/dev/null 2>&1; then
    sed -i'' "$@" "$file"
  else
    sed -i '' "$@" "$file"
  fi
}
