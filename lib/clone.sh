#!/usr/bin/env bash

ipalpha_clone_repo() {
  local repo="$1" dest="$2"
  if [[ -d "$dest" ]] && [[ -n "$(ls -A "$dest" 2>/dev/null)" ]]; then
    echo "  $(ipalpha_msg keeping_repo): $dest"
    return 0
  fi
  mkdir -p "$(dirname "$dest")"
  echo "  $(ipalpha_msg cloning) $repo → $dest"
  if [[ -n "${IPALPHA_CLONE_COMMAND:-}" ]]; then
    "$IPALPHA_CLONE_COMMAND" "git@github.com:${ipalpha_org}/${repo}.git" "$dest" || {
      echo "  $(ipalpha_msg clone_fail): $repo" >&2
      return 1
    }
  else
    if ! git clone "git@github.com:${ipalpha_org}/${repo}.git" "$dest" >/dev/null 2>&1; then
      if ! gh repo clone "${ipalpha_org}/${repo}" "$dest" >/dev/null 2>&1; then
        echo "  $(ipalpha_msg clone_fail): $repo ($(ipalpha_msg need_gh))" >&2
        return 1
      fi
    fi
  fi
}

ipalpha_clone_org_repos() {
  local root="$1" repo
  for repo in $(ipalpha_all_repos); do
    if ipalpha_is_app_repo "$repo"; then   # optional: not every account can read every app
      ipalpha_clone_repo "$repo" "$(ipalpha_repo_path "$root" "$repo")" 2>/dev/null \
        || echo "  $repo: $(ipalpha_msg clone_fail) (optional app, skipped)"
      continue
    fi
    ipalpha_clone_repo "$repo" "$(ipalpha_repo_path "$root" "$repo")" || return 1
  done
}
