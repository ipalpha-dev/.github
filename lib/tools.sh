#!/usr/bin/env bash

ipalpha_has_brew() {
  command -v brew >/dev/null 2>&1
}

ipalpha_brew_install() {
  local formula="$1"
  ipalpha_has_brew || return 1
  brew install "$formula" >/dev/null 2>&1 || brew upgrade "$formula" >/dev/null 2>&1 || return 1
}

ipalpha_tool_menu() {
  local name="$1" hint="$2"
  local choice
  while true; do
    echo
    echo "  $(ipalpha_msg tool_missing): $name"
    [[ -n "$hint" ]] && echo "  $hint"
    echo "  $(ipalpha_msg tool_how)"
    echo "    [1] $(ipalpha_msg tool_opt_auto)"
    echo "    [2] $(ipalpha_msg tool_opt_manual)"
    echo "    [3] $(ipalpha_msg tool_opt_quit)"
    read -r choice </dev/tty 2>/dev/null || choice=3
    case "$choice" in
      1) return 0 ;;
      2) return 2 ;;
      *) return 3 ;;
    esac
  done
}

ipalpha_ensure_tool() {
  local name="$1" check="$2" hint="$3" formula="$4"
  local mode
  while true; do
    if $check >/dev/null 2>&1; then
      echo "  $name: $(ipalpha_msg tool_ok)"
      return 0
    fi
    if [[ "${IPALPHA_SKIP_TOOLS:-}" == "1" ]]; then
      echo "  $name: $(ipalpha_msg tool_missing)"
      return 1
    fi
    if [[ "${IPALPHA_AUTO_INSTALL:-}" == "1" ]]; then
      if ipalpha_brew_install "$formula"; then
        $check >/dev/null 2>&1 && { echo "  $name: $(ipalpha_msg tool_ok)"; return 0; }
      fi
      echo "  $name: $(ipalpha_msg tool_install_failed)"
      return 1
    fi
    mode=0
    ipalpha_tool_menu "$name" "$hint" || mode=$?
    if [[ "$mode" == "0" ]]; then
      if ipalpha_brew_install "$formula" && $check >/dev/null 2>&1; then
        echo "  $name: $(ipalpha_msg tool_ok)"
        return 0
      fi
      echo "  $name: $(ipalpha_msg tool_install_failed)"
      return 1
    elif [[ "$mode" == "2" ]]; then
      continue
    else
      exit 3
    fi
  done
}

ipalpha_node_ok() {
  local ver
  command -v node >/dev/null 2>&1 || return 1
  ver="$(node -v 2>/dev/null | sed 's/^v//' | cut -d. -f1)"
  [[ "$ver" =~ ^[0-9]+$ ]] && [[ "$ver" -ge 20 ]]
}

ipalpha_gh_ok() {
  command -v gh >/dev/null 2>&1
}

ipalpha_detect_runtime() {
  if command -v container >/dev/null 2>&1; then
    ipalpha_runtime="container"
  elif command -v docker >/dev/null 2>&1; then
    ipalpha_runtime="docker"
  else
    ipalpha_runtime=""
  fi
}

ipalpha_ensure_runtime() {
  local mode choice
  while true; do
    ipalpha_detect_runtime
    if [[ -n "$ipalpha_runtime" ]]; then
      echo "  $(ipalpha_msg runtime_selected): $ipalpha_runtime ($(ipalpha_msg tool_ok))"
      return 0
    fi
    if [[ "${IPALPHA_SKIP_TOOLS:-}" == "1" ]]; then
      echo "  $(ipalpha_msg need_runtime)"
      return 1
    fi
    if [[ "${IPALPHA_AUTO_INSTALL:-}" == "1" ]]; then
      ipalpha_brew_install container && ipalpha_detect_runtime && [[ -n "$ipalpha_runtime" ]] && return 0
      ipalpha_brew_install docker && ipalpha_detect_runtime && [[ -n "$ipalpha_runtime" ]] && return 0
      echo "  $(ipalpha_msg need_runtime)"
      return 1
    fi
    mode=0
    ipalpha_tool_menu "container-runtime" "$(ipalpha_msg tool_hint_container)" || mode=$?
    if [[ "$mode" == "0" ]]; then
      ipalpha_brew_install container
      ipalpha_detect_runtime
      [[ -n "$ipalpha_runtime" ]] && { echo "  $(ipalpha_msg runtime_selected): $ipalpha_runtime ($(ipalpha_msg tool_ok))"; return 0; }
      ipalpha_brew_install docker
      ipalpha_detect_runtime
      [[ -n "$ipalpha_runtime" ]] && { echo "  $(ipalpha_msg runtime_selected): $ipalpha_runtime ($(ipalpha_msg tool_ok))"; return 0; }
      echo "  $(ipalpha_msg tool_install_failed)"
      return 1
    elif [[ "$mode" == "2" ]]; then
      continue
    else
      exit 3
    fi
  done
}

ipalpha_prompt_mprocs() {
  local choice
  if command -v mprocs >/dev/null 2>&1; then
    ipalpha_runner="auto"
    return 0
  fi
  ipalpha_runner="background"
  [[ "${IPALPHA_SKIP_TOOLS:-}" == "1" ]] && return 0
  echo
  echo "  $(ipalpha_msg choose_runner)"
  echo "    [1] $(ipalpha_msg tool_opt_auto) (brew install mprocs)"
  echo "    [2] $(ipalpha_msg tool_opt_manual) ($(ipalpha_msg tool_hint_mprocs))"
  read -r choice </dev/tty 2>/dev/null || choice=2
  if [[ "$choice" == "1" ]] && ipalpha_brew_install mprocs && command -v mprocs >/dev/null 2>&1; then
    ipalpha_runner="auto"
  else
    ipalpha_runner="background"
  fi
}

ipalpha_ensure_tools() {
  echo "$(ipalpha_msg checking_tools)"
  ipalpha_ensure_tool git "command -v git" "$(ipalpha_msg tool_hint_git)" git || exit 3
  ipalpha_ensure_tool node "ipalpha_node_ok" "$(ipalpha_msg tool_hint_node)" node@22 || exit 3
  ipalpha_ensure_tool npm "command -v npm" "$(ipalpha_msg tool_hint_node)" node@22 || exit 3
  ipalpha_ensure_tool gh "ipalpha_gh_ok" "$(ipalpha_msg tool_hint_gh)" gh || exit 3
  ipalpha_ensure_tool kubectl "command -v kubectl" "$(ipalpha_msg tool_hint_kubectl)" kubectl || exit 3
  ipalpha_ensure_runtime || exit 3
  ipalpha_prompt_mprocs
}
