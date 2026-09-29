#!/usr/bin/env bash

ipalpha_write_settings() {
  local root="$1"
  local dest repo var
  dest="$(ipalpha_settings_file "$root")"
  mkdir -p "$(dirname "$dest")"
  {
    echo "# IPAlpha local workspace settings (created by setup; used by ./run ./publish ./pull)"
    echo "lang=${ipalpha_lang:-pt-BR}"
    echo "org=${ipalpha_org}"
    echo "runtime=${ipalpha_runtime:-container}"
    echo "ai_cli=${ipalpha_ai_cli:-pi}"
    echo "ai_model=${ipalpha_ai_model:-cpamc/muse-spark-1.3-contributor}"
    echo "runner=${ipalpha_runner:-auto}"
    echo "mongo_port=${ipalpha_port_mongo:-$ipalpha_default_mongo_port}"
    echo "redis_port=${ipalpha_port_redis:-$ipalpha_default_redis_port}"
    echo "rabbitmq_port=${ipalpha_port_rabbitmq:-$ipalpha_default_rabbitmq_port}"
    echo "rabbitmq_mgmt_port=${ipalpha_port_rabbitmq_mgmt:-$ipalpha_default_rabbitmq_mgmt_port}"
    for repo in "${ipalpha_ms_order[@]}"; do
      var="ipalpha_port_${repo//-/_}"
      echo "${repo}_port=${!var:-$(ipalpha_default_ms_port "$repo")}"
    done
    for repo in "${ipalpha_web_repos[@]}"; do
      var="ipalpha_port_${repo//-/_}"
      echo "${repo}_port=${!var:-$(ipalpha_default_web_port "$repo")}"
    done
  } >"$dest"
  chmod 600 "$dest"
}

ipalpha_settings_web_port() {
  local repo="$1" var
  var="ipalpha_port_${repo//-/_}"
  if [[ -n "${!var:-}" ]]; then
    echo "${!var}"
  else
    echo "$(ipalpha_default_web_port "$repo")"
  fi
}

ipalpha_settings_ms_port() {
  local repo="$1" var
  var="ipalpha_port_${repo//-/_}"
  if [[ -n "${!var:-}" ]]; then
    echo "${!var}"
  else
    echo "$(ipalpha_default_ms_port "$repo")"
  fi
}

ipalpha_load_settings() {
  local root="$1"
  local file line key val ms
  file="$(ipalpha_settings_file "$root")"
  [[ -f "$file" ]] || return 1
  while IFS= read -r line || [[ -n "$line" ]]; do
    [[ -z "$line" || "$line" == \#* ]] && continue
    key="${line%%=*}"
    val="${line#*=}"
    case "$key" in
      lang) ipalpha_lang="$val" ;;
      org) ipalpha_org="$val" ;;
      runtime) ipalpha_runtime="$val" ;;
      ai_cli) ipalpha_ai_cli="$val" ;;
      ai_model) ipalpha_ai_model="$val" ;;
      runner) ipalpha_runner="$val" ;;
      mongo_port) ipalpha_port_mongo="$val" ;;
      redis_port) ipalpha_port_redis="$val" ;;
      rabbitmq_port) ipalpha_port_rabbitmq="$val" ;;
      rabbitmq_mgmt_port) ipalpha_port_rabbitmq_mgmt="$val" ;;
      *) 
        case "$key" in
          *_port)
            ms="${key%_port}"
            if ipalpha_is_ms_repo "$ms"; then
              printf -v "ipalpha_port_${ms//-/_}" '%s' "$val"
            fi
            ;;
        esac
        ;;
    esac
  done <"$file"
  ipalpha_i18n_init "${ipalpha_lang:-pt-BR}"
  return 0
}

ipalpha_settings_set_key() {
  local root="$1" key="$2" value="$3"
  local file tmp line
  file="$(ipalpha_settings_file "$root")"
  mkdir -p "$(dirname "$file")"
  tmp="$(mktemp)"
  if [[ -f "$file" ]] && grep -qE "^${key}=" "$file"; then
    while IFS= read -r line || [[ -n "$line" ]]; do
      case "$line" in
        "${key}="*) echo "${key}=${value}" ;;
        *) echo "$line" ;;
      esac
    done <"$file" >"$tmp"
  else
    cat "$file" 2>/dev/null >"$tmp" || true
    echo "${key}=${value}" >>"$tmp"
  fi
  mv "$tmp" "$file"
}
