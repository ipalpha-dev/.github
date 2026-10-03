#!/usr/bin/env bash

ipalpha_publish_cache_dir=""

ipalpha_publish_help() {
  ipalpha_msg help_publish
  echo "  --ci             publish sources for TeamCity image builds"
  echo "  --initialize     initialize a new private service repository"
  echo "  --resume         resume a tagged image release without another bump"
  echo "  --npm-only       publish the existing shared-js release to npm"
  echo "  --tooling        publish workspace publisher changes"
  echo "  --deployment-path PATH  publish a selected non-secret deployment file"
  echo "  --feature SLUG   commit + push feat/<slug> (no version bump) and deploy its preview"
  echo "  --no-wait        with --feature: do not wait for the TeamCity Preview build"
}

# "Dirty" = uncommitted changes OR commits since the last release tag (v<version>).
ipalpha_publish_repo_dirty() {
  local dir="$1" tag
  ipalpha_is_git_repo "$dir" || return 1
  [[ -n "$(git -C "$dir" status --porcelain 2>/dev/null)" ]] && return 0
  tag="$(git -C "$dir" describe --tags --abbrev=0 --match 'v*' 2>/dev/null)" || return 0
  [[ -n "$(git -C "$dir" log --oneline "${tag}..HEAD" 2>/dev/null)" ]]
}

ipalpha_publish_dirty_repos() {
  local root="$1" repo dir
  for repo in "${ipalpha_ms_repos[@]}" "${ipalpha_web_repos[@]}"; do
    dir="$(ipalpha_repo_path "$root" "$repo")"
    ipalpha_publish_repo_dirty "$dir" && echo "$repo"
  done
}

ipalpha_publish_parse_decision() {
  node -e '
    let s = "";
    process.stdin.on("data", d => (s += d));
    process.stdin.on("end", () => {
      s = s.split("\n").filter(l => !l.trim().startsWith("```")).join("\n").trim();
      const a = s.indexOf("{"), b = s.lastIndexOf("}");
      if (a < 0 || b <= a) process.exit(1);
      try {
        const d = JSON.parse(s.slice(a, b + 1));
        const bump = ["none", "patch", "minor", "major"].includes(d.bump) ? d.bump : "patch";
        const clean = v => String(v || "").replace(/\s+/g, " ").trim();
        const msg = [clean(d.reason), bump, clean(d.message)].join("\n");
        process.stdout.write(msg);
      } catch (e) {
        process.exit(1);
      }
    });
  '
}

# Bound release-analysis CLIs so an unavailable engine cannot stall publication.
ipalpha_publish_ai_run() {
  python3 - "$@" <<'PYRUN'
import os, signal, subprocess, sys, tempfile
process = subprocess.Popen(sys.argv[1:], cwd=tempfile.gettempdir(), stdout=subprocess.PIPE,
                           stderr=subprocess.DEVNULL, text=True, start_new_session=True)
try:
    output, _ = process.communicate(timeout=60)
except subprocess.TimeoutExpired:
    os.killpg(process.pid, signal.SIGKILL)
    process.communicate()
    sys.exit(1)
sys.stdout.write(output)
sys.exit(process.returncode)
PYRUN
}

ipalpha_ai_decide() {
  local engine="$1" model="$2" system="$3" body="$4"
  local full out
  full="$system

$body

Respond with JSON only: {\"reason\",\"bump\",\"message\"}."
  case "$engine" in
    pi)
      if [[ -n "$model" ]]; then
        out="$(ipalpha_publish_ai_run pi -p "$full" --model "$model" 2>/dev/null)" || \
          out="$(ipalpha_publish_ai_run pi "$full" 2>/dev/null)" || out=""
      else
        out="$(ipalpha_publish_ai_run pi -p "$full" 2>/dev/null)" || \
          out="$(ipalpha_publish_ai_run pi "$full" 2>/dev/null)" || out=""
      fi
      ;;
    claude)
      out="$(ipalpha_publish_ai_run claude -p "$full" ${model:+--model "$model"} 2>/dev/null)" || out=""
      ;;
    grok)
      out="$(ipalpha_publish_ai_run grok -p "$full" ${model:+--model "$model"} 2>/dev/null)" || \
        out="$(ipalpha_publish_ai_run grok "$full" 2>/dev/null)" || out=""
      ;;
    codex)
      out="$(ipalpha_publish_ai_run codex exec --skip-git-repo-check "$full" 2>/dev/null)" || out=""
      ;;
    *)
      out=""
      ;;
  esac
  [[ -z "$out" ]] && return 1
  ipalpha_publish_parse_decision <<<"$out"
}

ipalpha_publish_scope_clamp() {
  local dir="$1" bump="$2"
  local files docs_only=1 tag
  files="$(git -C "$dir" status --porcelain 2>/dev/null | sed -E 's/^...//; s/.* -> //' | tr -d '"')"
  if [[ -z "$files" ]]; then
    tag="$(git -C "$dir" describe --tags --abbrev=0 --match 'v*' 2>/dev/null)" || tag=""
    [[ -n "$tag" ]] && files="$(git -C "$dir" diff --name-only "${tag}..HEAD" 2>/dev/null)"
  fi
  [[ -z "$files" ]] && { echo "$bump"; return; }
  while IFS= read -r f; do
    [[ -z "$f" ]] && continue
    case "$f" in
      *.md|*.MD|docs/*|*.txt|LICENSE*) ;;
      *) docs_only=0; break ;;
    esac
  done <<<"$files"
  if [[ "$docs_only" == "1" ]]; then
    echo "none"
  else
    echo "$bump"
  fi
}

ipalpha_publish_current_version() {
  local dir="$1"
  if [[ -f "$dir/package.json" ]]; then
    node -p 'try { require(process.argv[1] + "/package.json").version || "0.0.0" } catch (e) { "0.0.0" }' "$dir"
  else
    git -C "$dir" describe --tags --abbrev=0 2>/dev/null | sed 's/^v//' || echo "0.0.0"
  fi
}

ipalpha_publish_bump_version() {
  local version="$1" bump="$2"
  case "$bump" in
    major) node -p 'const v=process.argv[1].split(".").map(Number); `${v[0]+1}.0.0`' "$version" ;;
    minor) node -p 'const v=process.argv[1].split(".").map(Number); `${v[0]}.${v[1]+1}.0`' "$version" ;;
    patch) node -p 'const v=process.argv[1].split(".").map(Number); `${v[0]}.${v[1]}.${(v[2]||0)+1}`' "$version" ;;
    *) echo "$version" ;;
  esac
}

ipalpha_publish_set_version() {
  local dir="$1" version="$2"
  [[ -f "$dir/package.json" ]] || return 0
  node -e '
    const fs = require("fs");
    const p = process.argv[1] + "/package.json";
    const pkg = JSON.parse(fs.readFileSync(p, "utf8"));
    pkg.version = process.argv[2];
    fs.writeFileSync(p, JSON.stringify(pkg, null, 2) + "\n");
    const lockPath = process.argv[1] + "/package-lock.json";
    if (fs.existsSync(lockPath)) {
      const lock = JSON.parse(fs.readFileSync(lockPath, "utf8"));
      lock.version = process.argv[2];
      if (lock.packages && lock.packages[""]) lock.packages[""].version = process.argv[2];
      fs.writeFileSync(lockPath, JSON.stringify(lock, null, 2) + "\n");
    }
  ' "$dir" "$version"
}

ipalpha_publish_ask_ai() {
  local root="$1" repo="$2" engine="$3" model="$4"
  local dir ctx key cache_file decision
  dir="$(ipalpha_repo_path "$root" "$repo")"
  ctx="Repository: $repo (current version $(ipalpha_publish_current_version "$dir"))

git status:
$(git -C "$dir" status --porcelain 2>/dev/null)

git diff --stat (working tree):
$(git -C "$dir" diff --stat HEAD 2>/dev/null | tail -n 20)

changes since last release tag:
$(tag="$(git -C "$dir" describe --tags --abbrev=0 --match 'v*' 2>/dev/null)"; [[ -n "$tag" ]] && git -C "$dir" diff --stat "${tag}..HEAD" 2>/dev/null | tail -n 20)

recent commits:
$(git -C "$dir" log --oneline -8 2>/dev/null)"
  key="$(printf '%s' "$ctx" | ipalpha_shasum)"
  ipalpha_publish_cache_dir="${ipalpha_publish_cache_dir:-$root/.ipalpha/.publish-cache}"
  cache_file="$ipalpha_publish_cache_dir/$repo-$key.json"
  if [[ -f "$cache_file" ]]; then
    cat "$cache_file"
    return 0
  fi
  decision="$(ipalpha_ai_decide "$engine" "$model" \
    "You are a release assistant for a TypeScript/NestJS monorepo. Given a repository's git status, diff stat and recent commits, pick the semantic version bump and write a one-line commit message (English, conventional-commit style, imperative mood)." \
    "$ctx")" || decision=""
  if [[ -z "$decision" ]]; then
    echo "$(ipalpha_msg publish_ai_down)" >&2
    decision="$(printf 'LLM unavailable, defaulted to patch\npatch\nUpdate %s' "$repo")"
  fi
  mkdir -p "$ipalpha_publish_cache_dir"
  printf '%s' "$decision" >"$cache_file"
  echo "$decision"
}

ipalpha_publish_select_repos() {
  local repos=("$@")
  local -a selected=()
  local i choice n
  for i in "${!repos[@]}"; do
    selected[$i]=1
  done
  if [[ ${#repos[@]} -eq 1 ]]; then
    printf '%s\n' "${repos[0]}"
    return 0
  fi
  while true; do
    echo "$(ipalpha_msg publish_select)"
    for i in "${!repos[@]}"; do
      if [[ "${selected[$i]}" == "1" ]]; then
        echo "  [x] $((i + 1)). ${repos[$i]}"
      else
        echo "  [ ] $((i + 1)). ${repos[$i]}"
      fi
    done
    echo "  (a=all · q=quit · Enter=confirm)"
    read -r choice || choice="q"
    [[ -z "$choice" ]] && break
    case "$choice" in
      q|Q) return 1 ;;
      a|A) for i in "${!repos[@]}"; do selected[$i]=1; done ;;
      *)
        for n in $choice; do
          if [[ "$n" =~ ^[0-9]+$ ]] && (( n >= 1 && n <= ${#repos[@]} )); then
            i=$((n - 1))
            if [[ "${selected[$i]}" == "1" ]]; then selected[$i]=0; else selected[$i]=1; fi
          fi
        done
        ;;
    esac
  done
  for i in "${!repos[@]}"; do
    [[ "${selected[$i]}" == "1" ]] && printf '%s\n' "${repos[$i]}"
  done
}

ipalpha_publish_build_image() {
  local repo="$1" version="$2" dir="$3"
  local image="${ipalpha_registry}/${repo}:${version}"
  if ! ipalpha_is_image_repo "$repo"; then
    echo "  $repo: $(ipalpha_msg publish_no_dockerfile)"
    return 0
  fi
  if [[ ! -f "$dir/Dockerfile" ]]; then
    echo "  $repo: $(ipalpha_msg publish_no_dockerfile)"
    return 0
  fi
  if command -v docker >/dev/null 2>&1; then
    docker build -t "$image" "$dir" || return 1
    docker push "$image" || { echo "  docker push failed — run: docker login ${ipalpha_registry%%/*}" >&2; return 1; }
  elif command -v container >/dev/null 2>&1; then
    container build --tag "$image" "$dir" || return 1
    container image push "$image" || return 1
  else
    echo "  no container runtime to build $image" >&2
    return 1
  fi
}

ipalpha_publish_npm() {
  local dir="$1"
  if ! (cd "$dir" && npm whoami >/dev/null 2>&1); then
    echo "  shared-js: not logged in to npm — run: npm login" >&2
    return 1
  fi
  # Deleted source modules must not survive in the published tarball.
  rm -rf "$dir/dist"
  (cd "$dir" && npm publish) || { echo "  shared-js: npm publish failed" >&2; return 1; }
}

ipalpha_publish_bump_deployment() {
  local root="$1" repo="$2" version="$3"
  local dep="$root/deployment" pattern file
  local -a image_files=()
  ipalpha_is_git_repo "$dep" || { echo "  $(ipalpha_msg publish_deployment_missing)"; return 0; }
  pattern="${ipalpha_registry}/${repo}:"
  while IFS= read -r file; do
    [[ -n "$file" ]] || continue
    image_files+=("$file")
    ipalpha_sed_inplace "$file" -E "s|${pattern}[^[:space:]\"']*|${pattern}${version}|g"
  done < <(grep -rl -F "$pattern" "$dep" --include='*.yaml' --include='*.yml' 2>/dev/null)
  if [[ -z "$(git -C "$dep" status --porcelain 2>/dev/null)" ]]; then
    return 0
  fi
  [[ ${#image_files[@]} -gt 0 ]] || return 0
  git -C "$dep" add -- "${image_files[@]}"
  git -C "$dep" diff --cached --quiet && return 0
  git -C "$dep" commit -q -m "bump ${repo} to ${version}"
  git -C "$dep" push
}

ipalpha_publish_repo() {
  local root="$1" repo="$2" version="$3" message="$4" bump="$5"
  local dir
  dir="$(ipalpha_repo_path "$root" "$repo")"
  # Check npm before committing a new release, so an expired token leaves Git untouched.
  if [[ "$repo" == shared-js && "$bump" != none ]]; then
    (cd "$dir" && npm whoami >/dev/null 2>&1) || {
      echo "shared-js: not logged in to npm — run: npm login" >&2; return 1;
    }
  fi
  if [[ "$bump" != "none" ]]; then
    ipalpha_publish_set_version "$dir" "$version"
  fi
  git -C "$dir" add -A
  if ! git -C "$dir" diff --cached --quiet; then
    git -C "$dir" commit -q -m "$message"
  fi
  if [[ "$bump" != "none" ]]; then
    git -C "$dir" tag "v${version}"
  fi
  git -C "$dir" push --set-upstream origin HEAD
  if [[ "$bump" != "none" ]]; then
    git -C "$dir" push origin "v${version}"
  fi
  if [[ "$bump" == "none" ]]; then
    return 0
  fi
  if [[ "$repo" == "shared-js" ]]; then
    ipalpha_publish_npm "$dir" || return 1
    return 0
  fi
  if [[ "$repo" == "shared-ui" ]]; then
    return 0
  fi
  if [[ "${ipalpha_publish_ci:-false}" != true ]]; then
    ipalpha_publish_build_image "$repo" "$version" "$dir" || return 1
  fi
  ipalpha_publish_bump_deployment "$root" "$repo" "$version"
}

ipalpha_publish_initialize() {
  local root="$1" repo="$2" dry_run="$3" dir
  ipalpha_is_ms_repo "$repo" || { echo "Unknown repository: $repo" >&2; return 1; }
  dir="$(ipalpha_repo_path "$root" "$repo")"
  [[ -d "$dir" && -f "$dir/package.json" && -f "$dir/.gitignore" ]] || {
    echo "New service needs package.json and .gitignore: $repo" >&2; return 1;
  }
  ! ipalpha_is_git_repo "$dir" || { echo "$repo: Git is already initialized"; return 0; }
  if [[ "$dry_run" == true ]]; then
    echo "Initialize $repo on master and create private repository $ipalpha_org/$repo (no commit or push)"
    return 0
  fi
  git init -q -b master "$dir"
  if gh repo view "$ipalpha_org/$repo" --json name >/dev/null 2>&1; then
    git -C "$dir" remote add origin "git@github.com:$ipalpha_org/$repo.git"
  else
    gh repo create "$ipalpha_org/$repo" --private --source "$dir" --remote origin
    git -C "$dir" remote set-url origin "git@github.com:$ipalpha_org/$repo.git"
  fi
}

ipalpha_publish() {
  local root="$1"; shift
  local dry_run=false initialize=false npm_only=false resume=false tooling=false folder="" engine="${ipalpha_ai_cli:-pi}" model="${ipalpha_ai_model:-}"
  local arg deployment_message="Update deployment configuration" feature="" feature_wait=true
  local -a deployment_paths=()
  ipalpha_publish_ci=false
  while [[ $# -gt 0 ]]; do
    arg="$1"
    case "$arg" in
      --ci) ipalpha_publish_ci=true ;;
      --initialize) initialize=true ;;
      --npm-only) npm_only=true ;;
      --resume) resume=true ;;
      --tooling) tooling=true ;;
      --deployment-path) shift; deployment_paths+=("${1:?path required}") ;;
      --feature) shift; feature="${1:?slug required}" ;;
      --feature=*) feature="${arg#*=}" ;;
      --no-wait) feature_wait=false ;;
      --message) shift; deployment_message="${1:?message required}" ;;
      -d|--dry-run) dry_run=true ;;
      -f|--folder) shift; folder="${1:-}" ;;
      -f*) folder="${arg#-f}" ;;
      --folder=*) folder="${arg#*=}" ;;
      --engine) shift; engine="${1:-}" ;;
      --engine=*) engine="${arg#*=}" ;;
      clean)
        rm -rf "$root/.ipalpha/.publish-cache"
        echo "$(ipalpha_msg publish_cache_clean)"
        return 0
        ;;
      -h|--help|help)
        ipalpha_publish_help
        return 0
        ;;
      *) ;;
    esac
    shift
  done

  # A feature workspace never cuts releases: plain ./publish there means --feature <its slug>.
  if [[ -z "$feature" && -f "$root/.ipalpha/feature.env" ]]; then
    feature="$(sed -n 's/^slug=//p' "$root/.ipalpha/feature.env")"
  fi
  if [[ -n "$feature" ]]; then
    if [[ "$tooling" == true || "$npm_only" == true || "$resume" == true || "$initialize" == true \
          || -n "$folder" || ${#deployment_paths[@]} -gt 0 ]]; then
      echo "--feature cannot be combined with release flags (--folder/--tooling/--npm-only/--resume/--initialize/--deployment-path)" >&2
      return 1
    fi
    # shellcheck source=lib/feature.sh
    source "$(dirname "${BASH_SOURCE[0]}")/feature.sh"
    ipalpha_feature_publish "$root" "$feature" "$feature_wait" "$engine" "$model" "$dry_run"
    return
  fi

  if [[ "$tooling" == true ]]; then
    local tooling_dir="$root/.ipalpha/tooling"
    ipalpha_is_git_repo "$tooling_dir" || { echo "Workspace tooling repository missing" >&2; return 1; }
    if [[ "$dry_run" == true ]]; then
      git -C "$tooling_dir" diff --stat -- lib/common.sh lib/publish.sh
      return 0
    fi
    [[ -z "$(git -C "$tooling_dir" diff --cached --name-only)" ]] || { echo "Tooling index is not empty" >&2; return 1; }
    git -C "$tooling_dir" add -- lib/common.sh lib/publish.sh
    git -C "$tooling_dir" diff --cached --quiet && return 0
    git -C "$tooling_dir" commit -m "$deployment_message"
    git -C "$tooling_dir" push --set-upstream origin HEAD
    return
  fi

  if [[ "$resume" == true ]]; then
    ipalpha_is_ms_repo "$folder" && ipalpha_is_image_repo "$folder" || {
      echo "--resume requires an image repository folder" >&2; return 1;
    }
    local resume_dir resume_version
    resume_dir="$(ipalpha_repo_path "$root" "$folder")"
    resume_version="$(ipalpha_publish_current_version "$resume_dir")"
    [[ -z "$(git -C "$resume_dir" status --porcelain)" ]] || { echo "$folder must be clean" >&2; return 1; }
    [[ "$(git -C "$resume_dir" rev-parse "v$resume_version^{commit}")" == "$(git -C "$resume_dir" rev-parse HEAD)" ]] || {
      echo "$folder HEAD must match its release tag" >&2; return 1;
    }
    echo "Resume $folder $resume_version (no version bump or new commit)"
    [[ "$dry_run" == true ]] && return 0
    git -C "$resume_dir" push --set-upstream origin HEAD
    git -C "$resume_dir" push origin "v$resume_version"
    if [[ "$ipalpha_publish_ci" != true ]]; then
      ipalpha_publish_build_image "$folder" "$resume_version" "$resume_dir" || return 1
    fi
    ipalpha_publish_bump_deployment "$root" "$folder" "$resume_version"
    return
  fi

  # Resume npm publication of an already committed/tagged release without another bump.
  if [[ "$npm_only" == true ]]; then
    [[ "$folder" == shared-js ]] || { echo "--npm-only requires --folder shared-js" >&2; return 1; }
    local npm_dir npm_version
    npm_dir="$(ipalpha_repo_path "$root" "$folder")"
    npm_version="$(node -p 'require(process.argv[1]).version' "$npm_dir/package.json")"
    [[ -z "$(git -C "$npm_dir" status --porcelain)" ]] || { echo "shared-js must be clean" >&2; return 1; }
    [[ "$(git -C "$npm_dir" rev-parse "v$npm_version^{commit}")" == "$(git -C "$npm_dir" rev-parse HEAD)" ]] || {
      echo "shared-js HEAD must match its release tag" >&2; return 1;
    }
    echo "Publish existing shared-js $npm_version to npm (no Git changes)"
    [[ "$dry_run" == true ]] && return 0
    ipalpha_publish_npm "$npm_dir"
    return
  fi

  if [[ "$initialize" == true ]]; then
    [[ -n "$folder" ]] || { echo "--initialize requires --folder" >&2; return 1; }
    ipalpha_publish_initialize "$root" "$folder" "$dry_run"
    return
  fi

  if [[ ${#deployment_paths[@]} -gt 0 ]]; then
    local dep="$root/deployment" path
    if [[ -n "$(git -C "$dep" diff --cached --name-only)" ]]; then
      echo "Deployment index is not empty; review staged changes first" >&2; return 1
    fi
    for path in "${deployment_paths[@]}"; do
      case "$path" in /*|../*|*/../*|*secret*|*Secret*)
        echo "Unsafe deployment publication path: $path" >&2; return 1 ;;
      esac
      [[ -e "$dep/$path" ]] || { echo "Missing deployment path: $path" >&2; return 1; }
    done
    if [[ "$dry_run" == true ]]; then
      printf 'Deployment files to publish: %s\n' "${deployment_paths[*]}"
      git -C "$dep" diff --stat -- "${deployment_paths[@]}"
      return 0
    fi
    git -C "$dep" add -- "${deployment_paths[@]}"
    git -C "$dep" diff --cached --quiet && return 0
    git -C "$dep" commit -m "$deployment_message"
    git -C "$dep" push
    return
  fi

  if [[ -n "$folder" ]]; then
    if ! ipalpha_is_ms_repo "$folder"; then
      echo "$(ipalpha_msg publish_folder_unknown) $folder" >&2
      return 1
    fi
    if ! ipalpha_publish_repo_dirty "$(ipalpha_repo_path "$root" "$folder")"; then
      echo "$folder: $(ipalpha_msg publish_no_dirty)"
      return 0
    fi
  fi

  local -a dirty=()
  while IFS= read -r repo; do
    [[ -n "$repo" ]] && dirty+=("$repo")
  done < <(ipalpha_publish_dirty_repos "$root")

  if [[ ${#dirty[@]} -eq 0 ]]; then
    echo "$(ipalpha_msg publish_no_dirty)"
    return 0
  fi

  if [[ -n "$folder" ]]; then
    local -a selected=("$folder")
  else
    local -a selected=()
    while IFS= read -r repo; do
      [[ -n "$repo" ]] && selected+=("$repo")
    done < <(ipalpha_publish_select_repos "${dirty[@]}") || { echo "$(ipalpha_msg publish_aborted)"; return 0; }
  fi
  if [[ ${#selected[@]} -eq 0 ]]; then
    echo "$(ipalpha_msg publish_aborted)"
    return 0
  fi

  local -a plan_bump=() plan_version=() plan_message=()
  local i=0 decision reason bump message version
  for repo in "${selected[@]}"; do
    decision="$(ipalpha_publish_ask_ai "$root" "$repo" "$engine" "$model")"
    reason="$(sed -n 1p <<<"$decision")"
    bump="$(sed -n 2p <<<"$decision")"
    message="$(sed -n 3p <<<"$decision")"
    bump="$(ipalpha_publish_scope_clamp "$(ipalpha_repo_path "$root" "$repo")" "$bump")"
    version="$(ipalpha_publish_current_version "$(ipalpha_repo_path "$root" "$repo")")"
    version="$(ipalpha_publish_bump_version "$version" "$bump")"
    [[ -z "$message" ]] && message="Update $repo"
    plan_bump[$i]="$bump"
    plan_version[$i]="$version"
    plan_message[$i]="$message"
    i=$((i + 1))
  done

  echo
  echo "$(ipalpha_msg publish_plan):"
  i=0
  for repo in "${selected[@]}"; do
    echo "  $repo: ${plan_bump[$i]} → v${plan_version[$i]} — ${plan_message[$i]}"
    i=$((i + 1))
  done

  if [[ "$dry_run" == true ]]; then
    echo
    printf '%s ' "$(ipalpha_msg publish_apply)"
    read -r answer || answer=""
    case "$answer" in
      y|Y|s|S) ;;
      *) echo "$(ipalpha_msg publish_aborted)"; return 0 ;;
    esac
  fi

  i=0
  for repo in "${selected[@]}"; do
    ipalpha_publish_repo "$root" "$repo" "${plan_version[$i]}" "${plan_message[$i]}" "${plan_bump[$i]}"
    i=$((i + 1))
  done
  echo "$(ipalpha_msg publish_done)"
}
