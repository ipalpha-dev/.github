#!/usr/bin/env bash

# Dependency-free setup UI: also works before the developer tools are installed.
# All terminal output goes to /dev/tty, never into a prompt's returned value.
ipalpha_ui_active=false

ipalpha_ui_start() {
  [[ -t 0 && -t 1 && "${TERM:-dumb}" != dumb && "${IPALPHA_PLAIN:-0}" != 1 ]] || return 0
  exec 9<>/dev/tty
  ipalpha_ui_stty="$(stty -g <&9)"
  ipalpha_ui_log="$(mktemp "${TMPDIR:-/tmp}/ipalpha-setup-ui.XXXXXX")"
  exec 7>&1 8>&2
  exec >>"$ipalpha_ui_log" 2>&1
  ipalpha_ui_active=true
  printf '\033[?1049h\033[?25l' >&9
}

ipalpha_ui_stop() {
  [[ "$ipalpha_ui_active" == true ]] || return 0
  printf '\033[0m\033[?25h\033[?1049l' >&9
  stty "$ipalpha_ui_stty" <&9
  exec 1>&7 2>&8 7>&- 8>&- 9>&-
  ipalpha_ui_active=false
}

ipalpha_ui_cleanup() {
  local status=$?
  if [[ -n "${ipalpha_ui_painter:-}" ]]; then
    kill "$ipalpha_ui_painter" 2>/dev/null || true
    wait "$ipalpha_ui_painter" 2>/dev/null || true
    ipalpha_ui_painter=''
  fi
  ipalpha_ui_stop
  if [[ -n "${ipalpha_ui_log:-}" ]]; then
    if [[ "$status" != 0 ]]; then tail -n 12 "$ipalpha_ui_log" >&2; fi
    rm -f "$ipalpha_ui_log"
    ipalpha_ui_log=''
  fi
  return "$status"
}

ipalpha_ui_size() {
  local size
  size="$(stty size <&9)"
  read -r ipalpha_ui_rows ipalpha_ui_cols <<<"$size"
  [[ "$ipalpha_ui_rows" -gt 0 ]] || ipalpha_ui_rows=24
  [[ "$ipalpha_ui_cols" -gt 0 ]] || ipalpha_ui_cols=80
}

ipalpha_ui_line() {
  local row="$1" text="$2" width=$((ipalpha_ui_cols - 4))
  ((width > 0)) || return 0
  printf '\033[%s;3H\033[97;44m%-*.*s' "$row" "$width" "$width" "$text" >&9
}

ipalpha_ui_frame() {
  local title="$1" row border
  ipalpha_ui_size
  # Paint each row explicitly: erase-screen uses the terminal's default background
  # on some emulators, which would leave the old black terminal around the dialog.
  printf '\033[?25l\033[97;44m\033[H' >&9
  for ((row=1; row<=ipalpha_ui_rows; row++)); do
    printf '\033[%s;1H%*s' "$row" "$ipalpha_ui_cols" '' >&9
  done
  printf -v border '%*s' "$((ipalpha_ui_cols - 4))" ''
  border="${border// /-}"
  ipalpha_ui_line 2 "+${border:2}+"
  ipalpha_ui_line 3 "IPAlpha | $title"
  ipalpha_ui_line 4 "+${border:2}+"
  ipalpha_ui_line "$((ipalpha_ui_rows - 2))" "+${border:2}+"
}

ipalpha_ui_select() {
  local title="$1" prompt="$2" selected=0 key suffix status row option
  shift 2
  local options=("$@")
  while true; do
    ipalpha_ui_frame "$title"
    ipalpha_ui_line 6 "$prompt"
    row=8
    for option in "${options[@]}"; do
      if ((row - 8 == selected)); then
        ipalpha_ui_line "$row" " > $option"
        printf '\033[%s;4H\033[1;34;47m %-*.*s\033[0;97;44m' "$row" "$((ipalpha_ui_cols - 7))" "$((ipalpha_ui_cols - 7))" "$option" >&9
      else
        ipalpha_ui_line "$row" "   $option"
      fi
      row=$((row + 1))
    done
    ipalpha_ui_line "$((ipalpha_ui_rows - 1))" "$(ipalpha_msg setup_keys)"
    key=''; status=0
    IFS= read -rsn1 key <&9 || status=$?
    ((status == 0)) || return 130
    case "$key" in
      '') ipalpha_ui_answer=$((selected + 1)); return 0 ;;
      $'\033')
        suffix=''
        # macOS ships Bash 3.2, whose read timeout only accepts whole seconds.
        IFS= read -rsn2 -t 1 suffix <&9 || true
        case "$suffix" in
          '[A'|'OA') selected=$(((selected + ${#options[@]} - 1) % ${#options[@]})) ;;
          '[B'|'OB') selected=$(((selected + 1) % ${#options[@]})) ;;
          '') return 130 ;;
        esac ;;
      [1-9])
        if ((key <= ${#options[@]})); then ipalpha_ui_answer="$key"; return 0; fi ;;
    esac
  done
}

ipalpha_ui_input() {
  local title="$1" prompt="$2" default="$3"
  ipalpha_ui_frame "$title"
  ipalpha_ui_line 6 "$prompt"
  ipalpha_ui_line 8 "$default"
  ipalpha_ui_line 10 "$(ipalpha_msg target_prompt)"
  ipalpha_ui_line "$((ipalpha_ui_rows - 1))" "$(ipalpha_msg setup_input_keys)"
  printf '\033[12;3H\033[?25h' >&9
  IFS= read -r ipalpha_ui_answer <&9 || return 130
  ipalpha_ui_answer="${ipalpha_ui_answer:-$default}"
  printf '\033[?25l' >&9
}

ipalpha_ui_progress() {
  local title="$1" row line
  ipalpha_ui_frame "$title"
  row=6
  while IFS= read -r line; do
    # Tool output is text, not terminal control (npm/git may include ANSI codes).
    line="$(printf '%s' "$line" | LC_ALL=C tr -d '\000-\010\013-\037\177')"
    ipalpha_ui_line "$row" "$line"
    row=$((row + 1))
  done < <(tail -n "$((ipalpha_ui_rows > 9 ? ipalpha_ui_rows - 9 : 1))" "$ipalpha_ui_log")
}

ipalpha_ui_run() {
  local title="$1" status
  shift
  if [[ "$ipalpha_ui_active" != true ]]; then "$@"; return $?; fi
  ipalpha_ui_progress "$title"
  # Keep functions in the main shell: port/runtime/repository choices must persist.
  (
    # Bash subshells inherit EXIT traps; only the owner may restore the terminal.
    trap - EXIT
    trap 'exit 0' INT TERM
    while true; do sleep 0.3; ipalpha_ui_progress "$title"; done
  ) &
  ipalpha_ui_painter=$!
  # Do not put this in an ||/if condition: that disables errexit inside functions.
  "$@"
  status=$?
  kill "$ipalpha_ui_painter" 2>/dev/null || true
  wait "$ipalpha_ui_painter" 2>/dev/null || true
  ipalpha_ui_painter=''
  return "$status"
}
