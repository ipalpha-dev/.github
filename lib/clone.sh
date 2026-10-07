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
  local root="$1" jobs="${IPALPHA_CLONE_JOBS:-6}" repo worker index status failed=0 saved_int saved_term
  local repos=()
  if [[ ! "$jobs" =~ ^([1-9]|[12][0-9]|3[0-2])$ ]]; then
    echo 'IPALPHA_CLONE_JOBS must be an integer within 1..32' >&2
    return 1
  fi
  while IFS= read -r repo; do repos+=("$repo"); done < <(ipalpha_all_repos)
  ((${#repos[@]} > 0)) || return 0
  ipalpha_clone_tmp="$(mktemp -d "${TMPDIR:-/tmp}/ipalpha-clone.XXXXXX")"
  ipalpha_clone_pids=()
  saved_int="$(trap -p INT)"; saved_term="$(trap -p TERM)"
  trap 'ipalpha_clone_cleanup; exit 130' INT
  trap 'ipalpha_clone_cleanup; exit 143' TERM
  # Fixed worker pool works on macOS Bash 3.2 too (no wait -n). Six concurrent
  # GitHub connections avoid flooding SSH/auth while keeping downloads overlapped.
  for ((worker=0; worker<jobs && worker<${#repos[@]}; worker++)); do
    (
      # Only the parent owns cleanup; an inherited EXIT trap must not restore UI.
      trap - EXIT INT TERM
      for ((index=worker; index<${#repos[@]}; index+=jobs)); do
        repo="${repos[$index]}"
        echo "  $(ipalpha_msg cloning) $repo"
        status=0
        ipalpha_clone_repo "$repo" "$(ipalpha_repo_path "$root" "$repo")" >"$ipalpha_clone_tmp/$index.log" 2>&1 || status=$?
        printf '%s\n' "$status" >"$ipalpha_clone_tmp/$index.status"
        if [[ "$status" == 0 ]]; then
          echo "  $repo: $(ipalpha_msg tool_ok)"
        elif ipalpha_is_app_repo "$repo"; then
          echo "  $repo: $(ipalpha_msg clone_fail) (optional app, skipped)"
        else
          echo "  $repo: $(ipalpha_msg clone_fail)"
        fi
      done
    ) &
    ipalpha_clone_pids+=("$!")
  done
  for worker in "${ipalpha_clone_pids[@]}"; do wait "$worker" || failed=1; done
  for index in "${!repos[@]}"; do
    repo="${repos[$index]}"
    status="$(cat "$ipalpha_clone_tmp/$index.status" 2>/dev/null || echo 1)"
    if [[ "$status" != 0 ]] && ! ipalpha_is_app_repo "$repo"; then
      cat "$ipalpha_clone_tmp/$index.log" >&2 2>/dev/null || true
      failed=1
    fi
  done
  ipalpha_clone_pids=()
  ipalpha_clone_cleanup
  # These strings come only from Bash's own trap -p, never repository output.
  if [[ -n "$saved_int" ]]; then eval "$saved_int"; else trap - INT; fi
  if [[ -n "$saved_term" ]]; then eval "$saved_term"; else trap - TERM; fi
  return "$failed"
}

ipalpha_clone_kill_tree() {
  local pid="$1" child
  for child in $(pgrep -P "$pid" 2>/dev/null || true); do ipalpha_clone_kill_tree "$child"; done
  kill -TERM "$pid" 2>/dev/null || true
}

ipalpha_clone_cleanup() {
  local pid
  for pid in "${ipalpha_clone_pids[@]:-}"; do
    [[ -n "$pid" ]] || continue
    ipalpha_clone_kill_tree "$pid"
    wait "$pid" 2>/dev/null || true
  done
  ipalpha_clone_pids=()
  if [[ -n "${ipalpha_clone_tmp:-}" ]]; then
    rm -rf "$ipalpha_clone_tmp"
    ipalpha_clone_tmp=''
  fi
}
