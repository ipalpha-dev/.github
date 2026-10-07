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
  local row="$1" text="$2" width=$((ipalpha_ui_box_width - 8))
  ((width > 0)) || return 0
  printf '\033[%s;4H\033[0m%-*.*s' "$row" "$width" "$width" "$text" >&9
}

ipalpha_ui_frame() {
  local title="$1" height="${2:-16}" row border
  ipalpha_ui_size
  # Match Cross's setup: full-screen application, compact rounded blue panel.
  # The terminal keeps its normal background; blue belongs to the border/title.
  ipalpha_ui_box_width=$((ipalpha_ui_cols < 80 ? ipalpha_ui_cols : 80))
  ipalpha_ui_box_height=$((height < ipalpha_ui_rows ? height : ipalpha_ui_rows))
  printf '\033[?25l\033[0m\033[H\033[2J' >&9
  printf -v border '%*s' "$((ipalpha_ui_box_width - 2))" ''
  border="${border// /─}"
  printf '\033[1;1H\033[38;5;33m╭%s╮' "$border" >&9
  for ((row=2; row<ipalpha_ui_box_height; row++)); do
    printf '\033[%s;1H│\033[%s;%sH│' "$row" "$row" "$ipalpha_ui_box_width" >&9
  done
  printf '\033[%s;1H╰%s╯\033[0m' "$ipalpha_ui_box_height" "$border" >&9
  printf '\033[3;4H\033[1;38;5;15;48;5;33m IPAlpha - Setup \033[0m' >&9
  ipalpha_ui_line 5 "$title"
}

ipalpha_ui_select() {
  local title="$1" prompt="$2" selected=0 key suffix status row option
  shift 2
  local options=("$@")
  while true; do
    ipalpha_ui_frame "$title" "$((${#options[@]} + 10))"
    if [[ "$prompt" != "$title" && -n "$prompt" ]]; then
      ipalpha_ui_line 6 "$prompt"
    fi
    row=7
    for option in "${options[@]}"; do
      if ((row - 7 == selected)); then
        ipalpha_ui_line "$row" "› $option"
        printf '\033[%s;4H\033[1;38;5;39m› %.*s\033[0m' "$row" "$((ipalpha_ui_box_width - 10))" "$option" >&9
      else
        ipalpha_ui_line "$row" "  $option"
      fi
      row=$((row + 1))
    done
    printf '\033[%s;4H\033[38;5;245m%.*s\033[0m' "$((ipalpha_ui_box_height - 2))" "$((ipalpha_ui_box_width - 8))" "$(ipalpha_msg setup_keys)" >&9
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
  local title="$1" prompt="$2" default="$3" cursor key suffix width start visible
  ipalpha_ui_frame "$title" 15
  if [[ "$prompt" != "$title" ]]; then ipalpha_ui_line 6 "$prompt"; fi
  ipalpha_ui_line 9 "$(ipalpha_msg target_prompt)"
  ipalpha_ui_line 13 "$(ipalpha_msg setup_input_keys)"
  # read -i is unavailable in macOS's Bash 3.2. Keep a small in-place editor
  # rather than showing the default as a placeholder outside the input.
  ipalpha_ui_answer="$default"
  cursor=${#ipalpha_ui_answer}
  width=$((ipalpha_ui_box_width - 8))
  stty -echo -icanon min 1 time 0 <&9
  while true; do
    start=$((cursor >= width ? cursor - width + 1 : 0))
    visible="${ipalpha_ui_answer:$start:$width}"
    printf '\033[11;4H\033[0m%-*s\033[11;%sH\033[?25h' "$width" "$visible" "$((4 + cursor - start))" >&9
    key=''
    IFS= read -rsn1 key <&9 || return 130
    case "$key" in
      '') ipalpha_ui_answer="${ipalpha_ui_answer:-$default}"; stty "$ipalpha_ui_stty" <&9; printf '\033[?25l' >&9; return 0 ;;
      $'\177'|$'\010')
        if ((cursor > 0)); then
          ipalpha_ui_answer="${ipalpha_ui_answer:0:cursor-1}${ipalpha_ui_answer:cursor}"
          cursor=$((cursor - 1))
        fi ;;
      $'\001') cursor=0 ;; # Ctrl+A / Home
      $'\005') cursor=${#ipalpha_ui_answer} ;; # Ctrl+E / End
      $'\025') ipalpha_ui_answer=''; cursor=0 ;; # Ctrl+U
      $'\033')
        suffix=''
        IFS= read -rsn2 -t 1 suffix <&9 || true
        case "$suffix" in
          '[D'|'OD') if ((cursor > 0)); then cursor=$((cursor - 1)); fi ;;
          '[C'|'OC') if ((cursor < ${#ipalpha_ui_answer})); then cursor=$((cursor + 1)); fi ;;
          '[H'|'OH') cursor=0 ;;
          '[F'|'OF') cursor=${#ipalpha_ui_answer} ;;
          '[3')
            IFS= read -rsn1 -t 1 suffix <&9 || true
            if [[ "$suffix" == '~' ]]; then
              ipalpha_ui_answer="${ipalpha_ui_answer:0:cursor}${ipalpha_ui_answer:cursor+1}"
            fi ;;
          '') return 130 ;;
        esac ;;
      $'\t') ;; # never insert a tab into the filesystem path
      *)
        ipalpha_ui_answer="${ipalpha_ui_answer:0:cursor}$key${ipalpha_ui_answer:cursor}"
        cursor=$((cursor + 1)) ;;
    esac
  done
}

ipalpha_ui_progress() {
  local title="$1" row line
  ipalpha_ui_frame "$title" 20
  row=7
  while IFS= read -r line; do
    # Tool output is text, not terminal control (npm/git may include ANSI codes).
    line="$(printf '%s' "$line" | LC_ALL=C tr -d '\000-\010\013-\037\177')"
    ipalpha_ui_line "$row" "$line"
    row=$((row + 1))
  done < <(tail -n "$((ipalpha_ui_box_height > 10 ? ipalpha_ui_box_height - 10 : 1))" "$ipalpha_ui_log")
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
