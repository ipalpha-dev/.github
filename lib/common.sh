#!/usr/bin/env bash

ipalpha_org="ipalpha-dev"
ipalpha_registry="ghcr.io/${ipalpha_org}"

ipalpha_ms_repos=(shared-js projects-api person-api organization-api notification-api auth-api)
ipalpha_ms_order=(projects-api person-api organization-api notification-api auth-api)
ipalpha_root_repos=(deployment)
ipalpha_tooling_repo=".github"

ipalpha_default_mongo_port=27017
ipalpha_default_redis_port=6379
ipalpha_default_rabbitmq_port=5672
ipalpha_default_rabbitmq_mgmt_port=15672

ipalpha_default_ms_port() {
  case "$1" in
    projects-api) echo 3001 ;;
    person-api) echo 3002 ;;
    organization-api) echo 3003 ;;
    notification-api) echo 3004 ;;
    auth-api) echo 3005 ;;
    *) echo 3000 ;;
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

ipalpha_is_ms_repo() {
  local repo="$1"
  case " ${ipalpha_ms_repos[*]} " in
    *" $repo "*) return 0 ;;
    *) return 1 ;;
  esac
}

ipalpha_is_image_repo() {
  case "$1" in
    shared-js) return 1 ;;
    *) return 0 ;;
  esac
}

ipalpha_all_repos() {
  local repo
  for repo in "${ipalpha_root_repos[@]}" "${ipalpha_ms_repos[@]}"; do
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
