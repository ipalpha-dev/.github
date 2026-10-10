#!/usr/bin/env bash

ipalpha_update_repo() {
  local dir="$1" name="$2"
  if ! ipalpha_is_git_repo "$dir"; then
    return 0
  fi
  if ! git -C "$dir" fetch --quiet >/dev/null 2>&1; then
    echo "  $name: $(ipalpha_msg update_pull_fail)"
    return 0
  fi
  if git -C "$dir" pull --ff-only --quiet >/dev/null 2>&1; then
    echo "  $name: $(ipalpha_msg update_pulled)"
  else
    echo "  $name: $(ipalpha_msg update_conflict)"
  fi
}

ipalpha_update_refresh_libs() {
  local root="$1" tmp src f
  tmp="$(mktemp -d "${TMPDIR:-/tmp}/ipalpha-tooling.XXXXXX")"
  src="$tmp/$ipalpha_tooling_repo"
  if ! ipalpha_clone_repo "$ipalpha_tooling_repo" "$src" >/dev/null 2>&1 || [[ ! -d "$src/lib" ]]; then
    echo "  $(ipalpha_msg update_tooling_fail)"
    rm -rf "$tmp"
    return 0
  fi
  for f in "$src"/lib/*.sh; do
    [[ -f "$f" ]] || continue
    # shellcheck disable=SC1090
    source "$f"
  done
  ipalpha_load_settings "$root" 2>/dev/null || true
  [[ "${ipalpha_runner:-}" == "background" ]] && ipalpha_runner="auto"
  ipalpha_materialize_workspace "$src" "$root"
  if [[ -f "$src/set-keys" ]]; then
    cp "$src/set-keys" "$root/set-keys" && chmod +x "$root/set-keys"
  fi
  rm -rf "$tmp"
  echo "  $(ipalpha_msg update_tooling_ok)"
}

# Repos that moved out of core/ (e.g. forms → apps/forms/): move the clone and every feature
# worktree of it, keeping local branches, .env files and uncommitted work.
ipalpha_migrate_layout() {
  local root="$1" root_cmp repo old new line wt wt_cmp dest rel_old rel_new
  # Git reports physical worktree paths (/private/var on macOS), even when the
  # workspace was opened via /var. Git for Windows reports C:/... while Git Bash
  # may report the same directory as /tmp/...; compare one canonical form.
  root="$(cd "$root" && pwd -P)"
  root_cmp="$root"
  if command -v cygpath >/dev/null 2>&1; then
    root_cmp="$(cygpath -m "$root")"
  fi
  root_cmp="${root_cmp%/}"
  for repo in $(ipalpha_all_repos); do
    ipalpha_app_of "$repo" >/dev/null || continue
    old="$root/core/$repo"; new="$(ipalpha_repo_path "$root" "$repo")"
    [[ -e "$old" && ! -e "$new" ]] || continue
    rel_old="core/$repo"; rel_new="$(ipalpha_repo_rel "$repo")"
    while IFS= read -r line; do
      wt="${line#worktree }"
      wt_cmp="$wt"
      if command -v cygpath >/dev/null 2>&1; then
        wt_cmp="$(cygpath -m "$wt")"
      fi
      wt_cmp="${wt_cmp%/}"
      [[ "$wt_cmp" == "$root_cmp"/features/*/"$rel_old" ]] || continue
      dest="${wt_cmp%/"$rel_old"}/$rel_new"
      mkdir -p "$(dirname "$dest")"
      git -C "$old" worktree move "$wt" "$dest"
    done < <(git -C "$old" worktree list --porcelain 2>/dev/null | grep '^worktree ')
    mkdir -p "$(dirname "$new")"
    mv "$old" "$new"
    git -C "$new" worktree repair >/dev/null 2>&1 || true
    echo "  $repo: $rel_old → $rel_new"
  done
}

ipalpha_update() {
  local root="$1"
  local repo dir

  ipalpha_migrate_layout "$root"

  # Feature workspaces hold worktrees on feat/<slug>: fetch only, ./feature rebase moves them.
  if [[ -f "$root/.ipalpha/feature.env" ]]; then
    for repo in $(ipalpha_all_repos); do
      dir="$(ipalpha_repo_path "$root" "$repo")"
      ipalpha_is_git_repo "$dir" && git -C "$dir" fetch -q origin 2>/dev/null && echo "  $repo: fetched"
    done
    ipalpha_update_refresh_libs "$root"
    echo "$(ipalpha_msg update_done)"
    return 0
  fi

  echo "$(ipalpha_msg update_pulling)"
  local out pids=() repos=()
  out="$(mktemp -d "${TMPDIR:-/tmp}/ipalpha-pull.XXXXXX")"
  for repo in $(ipalpha_all_repos); do
    dir="$(ipalpha_repo_path "$root" "$repo")"
    (
      if [[ -d "$dir" ]]; then
        ipalpha_update_repo "$dir" "$repo"
      elif ipalpha_is_app_repo "$repo"; then   # optional app: cloned when readable, quietly skipped otherwise
        ipalpha_clone_repo "$repo" "$dir" >/dev/null 2>&1 && echo "  $repo: $(ipalpha_msg update_pulled)" || true
      else
        ipalpha_clone_repo "$repo" "$dir" || echo "  $repo: $(ipalpha_msg update_clone_fail)"
      fi
    ) >"$out/$repo" 2>&1 &
    pids+=("$!")
    repos+=("$repo")
  done
  local i
  for i in "${!pids[@]}"; do
    wait "${pids[$i]}" || true
    cat "$out/${repos[$i]}"
  done
  rm -rf "$out"

  # Refresh BEFORE completing envs: old local tooling must not seed the legacy
  # {ms} format or apply yesterday's fallback defaults for one extra ./pull.
  ipalpha_update_refresh_libs "$root"
  local fallback_dir="$root/.ipalpha/env-fallback"
  for repo in "${ipalpha_ms_repos[@]}"; do
    dir="$(ipalpha_repo_path "$root" "$repo")"
    if [[ -d "$dir" ]]; then
      ipalpha_install_repo_env "$dir" "$repo" "$fallback_dir"
    fi
  done

  ipalpha_rewrites_from_settings
  ipalpha_apply_port_rewrites "$root"

  ipalpha_seed_local_clients "$root"

  echo "$(ipalpha_msg update_done)"
}
