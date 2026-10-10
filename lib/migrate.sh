#!/usr/bin/env bash
# Migration bridge for workspaces created by the old bash tooling. Their ./pull clones this
# repository, sources lib/*.sh and calls ipalpha_materialize_workspace. This file answers that
# call by installing the Go tool into .ipalpha/bin and letting it finish the pull (wrappers,
# legacy cleanup, .env completion). Remove once no bash workspace is left.

ipalpha_materialize_workspace() {
  local setup_root="$1" target_root="$2" os arch asset base bin tmp
  base="${IPALPHA_RELEASE_URL:-https://github.com/ipalpha-dev/.github/releases/latest/download}"
  case "$(uname -s)" in Darwin) os=darwin ;; *) os=linux ;; esac
  case "$(uname -m)" in x86_64|amd64) arch=amd64 ;; *) arch=arm64 ;; esac
  asset="ipalpha-$os-$arch"
  bin="$target_root/.ipalpha/bin/ipalpha"
  mkdir -p "$target_root/.ipalpha/bin"
  tmp="$(mktemp)"
  echo "  IPAlpha: installing the new ipalpha tool ($asset)…"
  if ! curl -fsSL --retry 3 -o "$tmp" "$base/$asset"; then
    rm -f "$tmp"
    echo "  could not download $base/$asset — run the setup command from the README again." >&2
    return 1
  fi
  chmod +x "$tmp" && mv "$tmp" "$bin"
  # The old pull keeps running after this function; the new tool does the real work now and
  # replaces ./run ./pull ./publish ./feature with wrappers.
  IPALPHA_NO_SELF_UPDATE=1 "$bin" pull --no-update </dev/null || true
  echo
  echo "  IPAlpha tools are now a single program. Next time just use ./run, ./pull, ./publish — or ./doctor if anything looks wrong."
  # Stop the old pull here: its remaining steps belong to the bash tooling that was just removed.
  exit 0
}
