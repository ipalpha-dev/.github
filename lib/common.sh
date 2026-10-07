#!/usr/bin/env bash

ipalpha_org="ipalpha-dev"
ipalpha_registry="${IPALPHA_REGISTRY:-registry.kevyn.com.br/ip-alpha/core}"

ipalpha_ms_repos=(shared-js shared-ui projects-api persons-api organizations-api notifications-api auth-api forms-api ai-api developers-api dispatch-api)
# Run order (panel rows, fallback-run). Nothing waits on a peer (GET /ready), so it only reads peers-first:
# ai-api calls auth + projects; developers-api calls auth + persons + projects; dispatch-api calls developers-api.
ipalpha_ms_order=(projects-api persons-api organizations-api notifications-api auth-api forms-api ai-api developers-api dispatch-api)
# Standalone web apps: cloned, deps installed, image published, no .env, run with Vite.
ipalpha_web_repos=(auth-webapp forms-webapp mordomia-webapp developers-webapp)
ipalpha_default_browser_apps="${ipalpha_web_repos[*]} mailpit"
ipalpha_root_repos=(deployment)
# Apps outside core with their own repositories, registry path and TeamCity project (Acampa Kids): GitHub
# ipalpha-dev/<repo>, local apps/<app>/<repo without the app prefix> (acampa-kids-backend → apps/acampa-kids/
# backend). Optional: cloned when the account can read them (a failure only warns), skipped when absent.
# Not run by ./run and never part of a core release; feature previews include them when they change.
# The face service joined Acampa's previews (DECISIONS_ACAMPA 48), so a face change is publishable too.
ipalpha_app_repos=(acampa-kids-backend acampa-kids-frontend acampa-kids-face-service)
ipalpha_tooling_repo=".github"
# Compose project / container prefix; feature workspaces use ipalpha-<slug> so they run side by side.
ipalpha_infra_name="${ipalpha_infra_name:-ipalpha}"

ipalpha_default_mongo_port=27017
ipalpha_default_redis_port=6379
ipalpha_default_rabbitmq_port=5672
ipalpha_default_rabbitmq_mgmt_port=15672
ipalpha_default_mailpit_port=8025

ipalpha_default_ms_port() {
  case "$1" in
    projects-api) echo 3001 ;;
    persons-api) echo 3002 ;;
    organizations-api) echo 3003 ;;
    notifications-api) echo 3004 ;;
    auth-api) echo 3005 ;;
    forms-api) echo 3006 ;;
    dispatch-api) echo 3007 ;;
    ai-api) echo 3008 ;;
    developers-api) echo 3009 ;;
    *) echo 3000 ;;
  esac
}

# Vite port for a standalone web app; its /api is proxied to this backend.
ipalpha_default_web_port() {
  case "$1" in
    auth-webapp) echo 5100 ;;
    forms-webapp) echo 5106 ;;
    mordomia-webapp) echo 5110 ;;
    developers-webapp) echo 5111 ;;
    *) echo 5199 ;;
  esac
}

ipalpha_web_api_backend() {
  case "$1" in
    auth-webapp) echo auth-api ;;
    forms-webapp) echo forms-api ;;
    mordomia-webapp) echo persons-api ;;
    developers-webapp) echo developers-api ;;
    *) echo "" ;;
  esac
}

ipalpha_settings_name=".ipalpha/settings"

ipalpha_settings_file() {
  echo "$1/${ipalpha_settings_name}"
}

# Apps outside core (Kevyn): consumers of core with their own namespace in production. They live
# under apps/<app>/<repo>; core capabilities and core UIs stay under core/<repo>.
ipalpha_app_of() {
  case "$1" in
    forms-api|forms-webapp) echo forms ;;
    acampa-kids-backend|acampa-kids-frontend|acampa-kids-face-service) echo acampa-kids ;;
    *) return 1 ;;
  esac
}

# Optional app repositories (ipalpha_app_repos): their own registry and Deploy, cloned best effort.
ipalpha_is_app_repo() {
  case " ${ipalpha_app_repos[*]} " in
    *" $1 "*) return 0 ;;
    *) return 1 ;;
  esac
}

# Image name in registry.kevyn.com.br/ip-alpha/apps/<app>/<name> of an app repository (scripts/build-app-image.sh).
ipalpha_app_image() {
  case "$1" in
    acampa-kids-backend) echo backend ;;
    acampa-kids-frontend) echo frontend ;;
    acampa-kids-face-service) echo face ;;
    *) return 1 ;;
  esac
}

# Workspace-relative folder of a repository.
ipalpha_repo_rel() {
  local repo="$1" app
  case " ${ipalpha_root_repos[*]} " in
    *" $repo "*) echo "$repo"; return 0 ;;
  esac
  if app="$(ipalpha_app_of "$repo")"; then
    # Optional app repos (<app>-<component>) live in apps/<app>/<component>; forms-api keeps its name.
    if ipalpha_is_app_repo "$repo"; then echo "apps/$app/${repo#"$app"-}"; else echo "apps/$app/$repo"; fi
  else
    echo "core/$repo"
  fi
}

ipalpha_repo_path() {
  echo "$1/$(ipalpha_repo_rel "$2")"
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
  for repo in "${ipalpha_root_repos[@]}" "${ipalpha_ms_repos[@]}" "${ipalpha_web_repos[@]}" "${ipalpha_app_repos[@]}"; do
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
