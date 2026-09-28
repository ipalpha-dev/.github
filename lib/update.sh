#!/usr/bin/env bash

ipalpha_update_repo() {
  local dir="$1" name="$2"
  if [[ ! -d "$dir/.git" ]]; then
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
  mkdir -p "$root/.ipalpha/lib"
  for f in "$src"/lib/*.sh; do
    [[ -f "$f" ]] || continue
    cp "$f" "$root/.ipalpha/lib/"
  done
  if [[ -d "$src/templates/env-fallback" ]]; then
    rm -rf "$root/.ipalpha/env-fallback"
    cp -R "$src/templates/env-fallback" "$root/.ipalpha/env-fallback"
  fi
  if [[ -f "$src/templates/compose.yaml" ]]; then
    cp "$src/templates/compose.yaml" "$root/.ipalpha/compose.yaml"
  fi
  if [[ -f "$src/set-keys" ]]; then
    cp "$src/set-keys" "$root/set-keys" && chmod +x "$root/set-keys"
  fi
  rm -rf "$tmp"
  echo "  $(ipalpha_msg update_tooling_ok)"
}

ipalpha_update() {
  local root="$1"
  local repo dir

  echo "$(ipalpha_msg update_pulling)"
  for repo in $(ipalpha_all_repos); do
    dir="$(ipalpha_repo_path "$root" "$repo")"
    if [[ -d "$dir" ]]; then
      ipalpha_update_repo "$dir" "$repo"
    else
      ipalpha_clone_repo "$repo" "$dir" || echo "  $repo: $(ipalpha_msg update_clone_fail)"
    fi
  done

  local fallback_dir="$root/.ipalpha/env-fallback"
  for repo in "${ipalpha_ms_repos[@]}"; do
    dir="$(ipalpha_repo_path "$root" "$repo")"
    if [[ -d "$dir" ]]; then
      ipalpha_install_repo_env "$dir" "$repo" "$fallback_dir"
    fi
  done

  ipalpha_rewrites_from_settings
  ipalpha_apply_port_rewrites "$root"

  ipalpha_update_refresh_libs "$root"
  ipalpha_write_ports_env "$root/.ipalpha/ports.env"
  ipalpha_write_mprocs_yaml "$root" "$root/.ipalpha/mprocs.yaml"

  echo "$(ipalpha_msg update_done)"
}
