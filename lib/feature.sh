#!/usr/bin/env bash
# Feature environments: one isolated workspace (git worktrees on feat/<slug>) + one preview
# namespace per feature. Design: deployment/docs/feature-environments.md (decisions are final).
# The cluster side only reacts to previews/<slug>/release.json pushed to deployment master; this
# file never talks to Kubernetes for writes and never holds a TeamCity token.
# shellcheck disable=SC2154,SC2034  # workspace globals live in common.sh / settings.sh / ports.sh

ipalpha_feature_domain="${IPALPHA_PREVIEW_DOMAIN:-kevyn.com.br}"
ipalpha_teamcity_url="${IPALPHA_TEAMCITY_URL:-https://devops.kevyn.com.br}"
ipalpha_preview_build_type="${IPALPHA_PREVIEW_BUILD_TYPE:-IpAlpha_Core_Previews_Preview}"
ipalpha_feature_wait_minutes="${IPALPHA_FEATURE_WAIT_MINUTES:-60}"

ipalpha_feature_fail() { echo "feature: $*" >&2; return 1; }

ipalpha_feature_hosts() {
  local slug="$1"
  echo "ipalpha-$slug.$ipalpha_feature_domain forms-ipalpha-$slug.$ipalpha_feature_domain auth-ipalpha-$slug.$ipalpha_feature_domain"
}

ipalpha_feature_validate_slug() {
  local slug="$1" host
  [[ "$slug" =~ ^[a-z0-9-]{3,30}$ ]] || { ipalpha_feature_fail "$(ipalpha_msg feature_bad_slug): $slug"; return 1; }
  # DNS labels cannot start or end with a hyphen.
  [[ "$slug" != -* && "$slug" != *- ]] || { ipalpha_feature_fail "$(ipalpha_msg feature_bad_slug): $slug"; return 1; }
  for host in $(ipalpha_feature_hosts "$slug"); do
    (( ${#host} <= 63 )) || { ipalpha_feature_fail "host longer than 63 chars: $host"; return 1; }
  done
}

# A feature workspace knows its slug and the main workspace it was created from.
ipalpha_feature_env_get() {
  local root="$1" key="$2"
  [[ -f "$root/.ipalpha/feature.env" ]] || return 1
  sed -n "s/^${key}=//p" "$root/.ipalpha/feature.env" | head -n1
}

ipalpha_feature_main_root() {
  local root="$1"
  ipalpha_feature_env_get "$root" main_root || echo "$root"
}

ipalpha_feature_dir() { echo "$1/features/$2"; }

# Repositories that live in a feature workspace (everything the main workspace clones).
ipalpha_feature_repos() {
  ipalpha_all_repos
}

ipalpha_feature_now() { date -u +%Y-%m-%dT%H:%M:%SZ; }

# Baseline = releases/core-latest.json on deployment master (last green Core Deploy).
ipalpha_feature_read_baseline() {
  local main="$1" out="$2" dep="$1/deployment"
  git -C "$dep" fetch -q origin master || { ipalpha_feature_fail "cannot fetch deployment master"; return 1; }
  git -C "$dep" show origin/master:releases/core-latest.json >"$out" 2>/dev/null \
    || { ipalpha_feature_fail "$(ipalpha_msg feature_no_baseline)"; return 1; }
  git -C "$dep" rev-parse origin/master
}

# Commit the baseline pins for a repository. shared-js is consumed from npm, so its commit is the
# release tag of the version locked by the APIs; deployment follows master (manifests, not images).
ipalpha_feature_base_commit() {
  local main="$1" baseline="$2" repo="$3"
  local commit version="" api api_commit
  case "$repo" in
    shared-js)
      for api in "${ipalpha_ms_order[@]}"; do
        api_commit="$(node -e 'const b=JSON.parse(require("fs").readFileSync(process.argv[1],"utf8")); const s=(b.services||{})[process.argv[2]]; process.stdout.write((s&&s.sourceCommit)||"")' "$baseline" "$api")"
        [[ -n "$api_commit" ]] || continue
        version="$(git -C "$main/core/$api" show "$api_commit:package-lock.json" 2>/dev/null \
          | node -e 'let s="";process.stdin.on("data",d=>s+=d).on("end",()=>{try{const p=JSON.parse(s).packages["node_modules/@ipalpha/shared-js"];process.stdout.write((p&&p.version)||"")}catch{}})')"
        [[ -n "$version" ]] && break
      done
      [[ -n "$version" ]] || { ipalpha_feature_fail "cannot resolve the baseline shared-js version"; return 1; }
      git -C "$main/core/shared-js" fetch -q --tags origin || true
      git -C "$main/core/shared-js" rev-parse --verify -q "v$version^{commit}" \
        || { ipalpha_feature_fail "shared-js tag v$version not found"; return 1; }
      return 0
      ;;
  esac
  commit="$(node -e '
    const b = JSON.parse(require("fs").readFileSync(process.argv[1], "utf8")), r = process.argv[2];
    const s = (b.services || {})[r] || (b.libraries || {})[r];
    process.stdout.write((s && (s.sourceCommit || s.commit)) || "");
  ' "$baseline" "$repo")"
  [[ -n "$commit" ]] || { ipalpha_feature_fail "baseline has no commit for $repo"; return 1; }
  echo "$commit"
}

ipalpha_feature_add_worktree() {
  local src="$1" dest="$2" branch="$3" base="$4"
  ipalpha_is_git_repo "$src" || { ipalpha_feature_fail "missing repository: $src"; return 1; }
  git -C "$src" fetch -q origin || echo "  warning: fetch failed for $src" >&2
  git -C "$src" cat-file -e "$base^{commit}" 2>/dev/null \
    || { ipalpha_feature_fail "commit $base not found in $src"; return 1; }
  mkdir -p "$(dirname "$dest")"
  if git -C "$src" show-ref --verify --quiet "refs/heads/$branch"; then
    git -C "$src" worktree add -q "$dest" "$branch"
  elif git -C "$src" show-ref --verify --quiet "refs/remotes/origin/$branch"; then
    git -C "$src" worktree add -q --track -b "$branch" "$dest" "origin/$branch"
  else
    git -C "$src" worktree add -q --no-track -b "$branch" "$dest" "$base"
  fi
}

# Every port variable a workspace owns, with its default.
ipalpha_feature_port_vars() {
  local repo
  echo "ipalpha_port_mongo $ipalpha_default_mongo_port"
  echo "ipalpha_port_redis $ipalpha_default_redis_port"
  echo "ipalpha_port_rabbitmq $ipalpha_default_rabbitmq_port"
  echo "ipalpha_port_rabbitmq_mgmt $ipalpha_default_rabbitmq_mgmt_port"
  for repo in "${ipalpha_ms_order[@]}"; do echo "ipalpha_port_${repo//-/_} $(ipalpha_default_ms_port "$repo")"; done
  for repo in "${ipalpha_web_repos[@]}"; do echo "ipalpha_port_${repo//-/_} $(ipalpha_default_web_port "$repo")"; done
}

# Distinct local ports: the first +100·k offset that no other workspace (main or feature) has
# recorded, then the usual busy-port probe on top. Fills ipalpha_remap_* main port → feature port.
ipalpha_feature_assign_ports() {
  local main="$1" used k offset ok var default main_port
  used=" $(cat "$main/.ipalpha/settings" "$main"/features/*/.ipalpha/settings 2>/dev/null \
    | sed -n 's/^[a-z_-]*port=//p' | tr '\n' ' ') "
  for k in $(seq 1 60); do
    offset=$((k * 100)); ok=1
    while read -r var default; do
      [[ "$used" == *" $((default + offset)) "* ]] && { ok=0; break; }
    done < <(ipalpha_feature_port_vars)
    [[ "$ok" == 1 ]] && break
  done
  [[ "$ok" == 1 ]] || { ipalpha_feature_fail "no free port block for another feature"; return 1; }
  local -a from=() to=()
  while read -r var default; do
    main_port="${!var:-$default}"
    ipalpha_resolve_port "$var" "$((default + offset))" >/dev/null
    from+=("$main_port"); to+=("${!var}")
  done < <(ipalpha_feature_port_vars)
  ipalpha_remap_from=("${from[@]}")
  ipalpha_remap_to=("${to[@]}")
}

# Release record (schema approved in the spec, §1 decision 11). Read/patch with node: laptops
# already need node for the services.
ipalpha_feature_json() {
  local file="$1" script="$2"; shift 2
  node -e '
    const fs = require("fs");
    const file = process.argv[1];
    const r = fs.existsSync(file) ? JSON.parse(fs.readFileSync(file, "utf8")) : {};
    const args = process.argv.slice(3);
    const out = (new Function("r", "args", process.argv[2]))(r, args);
    if (out === undefined) fs.writeFileSync(file, JSON.stringify(r, null, 2) + "\n");
    else if (out !== null) process.stdout.write(String(out));
  ' "$file" "$script" "$@"
}

ipalpha_feature_write_draft() {
  local froot="$1" slug="$2" baseline="$3" baseline_commit="$4" repos_file="$5" owner
  owner="$(git config user.email 2>/dev/null || echo unknown)"
  ipalpha_feature_json "$froot/.ipalpha/release.json" '
    const fs = require("fs");
    const [slug, owner, baselineFile, baselineCommit, reposFile, now, hosts, org] = args;
    const baseline = JSON.parse(fs.readFileSync(baselineFile, "utf8"));
    const repositories = {};
    for (const line of fs.readFileSync(reposFile, "utf8").split("\n").filter(Boolean)) {
      const [repo, commit] = line.split(" ");
      repositories[repo] = { url: `git@github.com:${org}/${repo}.git`, baseRef: "master",
        baseCommit: commit, featureCommit: commit };
    }
    Object.assign(r, { schemaVersion: 1, feature: slug, namespace: `ipalpha-feat-${slug}`, owner,
      baselineRelease: { id: baseline.release || null, deploymentCommit: baselineCommit },
      repositories, images: {}, seed: { version: 1, digest: null },
      hosts: hosts.split(" "), generation: 0, createdAt: now, expiresAt: null, teamcityBuild: null });
  ' "$slug" "$owner" "$baseline" "$baseline_commit" "$repos_file" "$(ipalpha_feature_now)" \
    "$(ipalpha_feature_hosts "$slug")" "$ipalpha_org"
}

ipalpha_feature_materialize() {
  local main="$1" froot="$2" slug="$3" shim
  shim="$(mktemp -d "${TMPDIR:-/tmp}/ipalpha-feature.XXXXXX")"
  mkdir -p "$shim/lib" "$shim/templates"
  cp "$main"/.ipalpha/lib/*.sh "$shim/lib/"
  cp -R "$main/.ipalpha/env-fallback" "$shim/templates/env-fallback"
  cp "$main/.ipalpha/compose.yaml" "$shim/templates/compose.yaml"
  mkdir -p "$froot/.ipalpha"
  [[ -f "$main/.ipalpha/.env" ]] && cp "$main/.ipalpha/.env" "$froot/.ipalpha/.env"
  ipalpha_infra_name="ipalpha-$slug"
  IPALPHA_PROCS_BINARY="$main/.ipalpha/bin/ipalpha-procs" ipalpha_materialize_workspace "$shim" "$froot"
  rm -rf "$shim"
  printf 'slug=%s\nmain_root=%s\n' "$slug" "$main" >"$froot/.ipalpha/feature.env"
}

ipalpha_feature_new() {
  local root="$1" slug="$2" main froot baseline baseline_commit repo base repos_file dest
  main="$(ipalpha_feature_main_root "$root")"
  ipalpha_feature_validate_slug "$slug" || return 1
  froot="$(ipalpha_feature_dir "$main" "$slug")"
  [[ ! -e "$froot" ]] || { ipalpha_feature_fail "$(ipalpha_msg feature_exists): $froot"; return 1; }
  ipalpha_load_settings "$main" >/dev/null 2>&1 || true

  baseline="$(mktemp)"; repos_file="$(mktemp)"
  baseline_commit="$(ipalpha_feature_read_baseline "$main" "$baseline")" || { rm -f "$baseline" "$repos_file"; return 1; }
  echo "$(ipalpha_msg feature_creating) $slug ($(node -p 'JSON.parse(require("fs").readFileSync(process.argv[1],"utf8")).release' "$baseline"))"
  for repo in $(ipalpha_feature_repos); do
    if [[ "$repo" == deployment ]]; then
      base="$(git -C "$main/deployment" rev-parse origin/master)"
    else
      base="$(ipalpha_feature_base_commit "$main" "$baseline" "$repo")" || { rm -f "$baseline" "$repos_file"; return 1; }
    fi
    dest="$(ipalpha_repo_path "$froot" "$repo")"
    ipalpha_feature_add_worktree "$(ipalpha_repo_path "$main" "$repo")" "$dest" "feat/$slug" "$base" \
      || { rm -f "$baseline" "$repos_file"; return 1; }
    echo "  $repo → feat/$slug @ ${base:0:12}"
    echo "$repo $base" >>"$repos_file"
  done

  # Local environment: the main workspace's .env files, rewritten to this feature's ports.
  ipalpha_feature_assign_ports "$main" || return 1
  for repo in "${ipalpha_ms_repos[@]}"; do
    dest="$(ipalpha_repo_path "$froot" "$repo")"
    if [[ -f "$(ipalpha_repo_path "$main" "$repo")/.env" ]]; then
      cp "$(ipalpha_repo_path "$main" "$repo")/.env" "$dest/.env"
    else
      ipalpha_install_repo_env "$dest" "$repo" "$main/.ipalpha/env-fallback" >/dev/null
    fi
  done
  ipalpha_apply_port_rewrites "$froot"
  ipalpha_feature_materialize "$main" "$froot" "$slug"

  ipalpha_feature_write_draft "$froot" "$slug" "$baseline" "$baseline_commit" "$repos_file"
  cp "$baseline" "$froot/.ipalpha/baseline.json"
  rm -f "$baseline" "$repos_file"

  echo
  echo "$(ipalpha_msg feature_created): $froot"
  echo "  cd features/$slug && ./run        # mongo :$ipalpha_port_mongo · web :$ipalpha_port_mordomia_webapp"
  echo "  ./publish --feature $slug         # preview:"
  for host in $(ipalpha_feature_hosts "$slug"); do echo "    https://$host"; done
}

# Changed = featureCommit != baseCommit, plus the dependency closure (decision 9):
# shared-js → every *-api, shared-ui → every *-webapp. ai-api is not cloned locally; CI applies
# the same closure to it.
ipalpha_feature_changed_services() {
  local record="$1"
  ipalpha_feature_json "$record" '
    const repos = r.repositories || {};
    const changed = new Set(Object.keys(repos).filter(k => repos[k].featureCommit !== repos[k].baseCommit));
    const all = Object.keys(repos);
    if (changed.has("shared-js")) all.filter(k => k.endsWith("-api")).forEach(k => changed.add(k));
    if (changed.has("shared-ui")) all.filter(k => k.endsWith("-webapp")).forEach(k => changed.add(k));
    return [...changed].filter(k => k !== "shared-js" && k !== "shared-ui" && k !== "deployment").sort().join("\n");
  '
}

# Push previews/<slug>/release.json to deployment master without touching the developer's checkouts:
# a throwaway detached worktree, rebased on the latest master, retried on races. The newest record on
# master is the base (CI fills images/expiresAt there); "patch" is applied on top of it.
ipalpha_feature_push_record() {
  local main="$1" slug="$2" local_record="$3" message="$4" patch="$5"; shift 5
  local dep="$main/deployment" tmp wt sha
  tmp="$(mktemp -d "${TMPDIR:-/tmp}/ipalpha-record.XXXXXX")"; wt="$tmp/wt"
  for _ in 1 2 3 4 5; do
    git -C "$dep" fetch -q origin master || { rm -rf "$tmp"; ipalpha_feature_fail "cannot fetch deployment"; return 1; }
    git -C "$dep" worktree add -q --detach "$wt" origin/master
    mkdir -p "$wt/previews/$slug"
    if [[ -f "$wt/previews/$slug/release.json" ]]; then
      # Keep CI-owned fields from master, take local repositories when the draft is newer.
      ipalpha_feature_json "$wt/previews/$slug/release.json" '
        const local = JSON.parse(require("fs").readFileSync(args[0], "utf8"));
        r.repositories = local.repositories || r.repositories;
        r.baselineRelease = local.baselineRelease || r.baselineRelease;
      ' "$local_record"
    else
      cp "$local_record" "$wt/previews/$slug/release.json"
    fi
    ipalpha_feature_json "$wt/previews/$slug/release.json" "$patch" "$@"
    git -C "$wt" add "previews/$slug/release.json"
    if git -C "$wt" diff --cached --quiet; then
      sha="$(git -C "$wt" rev-parse HEAD)"
    else
      git -C "$wt" commit -q -m "$message"
      sha="$(git -C "$wt" rev-parse HEAD)"
      if ! git -C "$wt" push -q origin HEAD:master 2>/dev/null; then
        git -C "$dep" worktree remove --force "$wt"; sha=""; continue
      fi
    fi
    cp "$wt/previews/$slug/release.json" "$local_record"
    git -C "$dep" worktree remove --force "$wt"
    rm -rf "$tmp"
    echo "$sha"
    return 0
  done
  rm -rf "$tmp"
  ipalpha_feature_fail "could not push the release record after 5 attempts"
}

# Status comes from TeamCity's public (guest) REST API; no token on laptops.
ipalpha_feature_wait() {
  local main="$1" slug="$2" sha="$3" url deadline state="" last="" body build_state status text web
  url="$ipalpha_teamcity_url/guestAuth/app/rest/builds?locator=buildType:(id:$ipalpha_preview_build_type),revision:$sha,state:any,defaultFilter:false,count:1&fields=build(id,state,status,statusText,webUrl)"
  echo "$(ipalpha_msg feature_waiting) ($ipalpha_teamcity_url)"
  deadline=$(( $(date +%s) + ipalpha_feature_wait_minutes * 60 ))
  while (( $(date +%s) < deadline )); do
    body="$(curl -fsS -H 'Accept: application/json' "$url" 2>/dev/null)" || {
      echo "  $(ipalpha_msg feature_status_unavailable)"; return 0; }
    state="$(node -e 'const b=(JSON.parse(process.argv[1]).build||[])[0]; process.stdout.write(b?`${b.state}\t${b.status||""}\t${(b.statusText||"").replace(/\s+/g," ")}\t${b.webUrl||""}`:"")' "$body")"
    if [[ -n "$state" && "$state" != "$last" ]]; then
      IFS=$'\t' read -r build_state status text web <<<"$state"
      echo "  ${build_state} ${status} ${text}"
      last="$state"
      if [[ "$build_state" == finished ]]; then
        if [[ "$status" == SUCCESS ]]; then
          git -C "$main/deployment" fetch -q origin master
          git -C "$main/deployment" show "origin/master:previews/$slug/release.json" 2>/dev/null \
            | node -e 'let s="";process.stdin.on("data",d=>s+=d).on("end",()=>{const r=JSON.parse(s);for(const h of r.hosts||[])console.log("  https://"+h);console.log("  expires: "+(r.expiresAt||"-"))})' \
            || true
          return 0
        fi
        echo "  $(ipalpha_msg feature_failed): $web" >&2
        return 1
      fi
    fi
    sleep 15
  done
  echo "  $(ipalpha_msg feature_status_unavailable)"
}

# ./publish --feature <slug>: commit (bump none) and push feat/<slug>, then the release record.
ipalpha_feature_publish() {
  local root="$1" slug="$2" wait="$3" engine="$4" model="$5"
  local main froot record repo dir branch decision message base head sha changed
  main="$(ipalpha_feature_main_root "$root")"
  froot="$(ipalpha_feature_dir "$main" "$slug")"
  record="$froot/.ipalpha/release.json"
  [[ -f "$record" ]] || { ipalpha_feature_fail "$(ipalpha_msg feature_missing): $slug"; return 1; }
  ipalpha_feature_validate_slug "$slug" || return 1

  for repo in $(ipalpha_feature_repos); do
    dir="$(ipalpha_repo_path "$froot" "$repo")"
    ipalpha_is_git_repo "$dir" || continue
    branch="$(git -C "$dir" symbolic-ref --short -q HEAD || true)"
    [[ "$branch" == "feat/$slug" ]] || { ipalpha_feature_fail "$repo is on '$branch', expected feat/$slug"; return 1; }
    if [[ -n "$(git -C "$dir" status --porcelain)" ]]; then
      decision="$(ipalpha_publish_ask_ai "$froot" "$repo" "$engine" "$model")"
      message="$(sed -n 3p <<<"$decision")"
      git -C "$dir" add -A
      git -C "$dir" commit -q -m "${message:-Update $repo}"
      echo "  $repo: ${message:-Update $repo}"
    fi
    base="$(ipalpha_feature_json "$record" 'return (r.repositories[args[0]]||{}).baseCommit||""' "$repo")"
    head="$(git -C "$dir" rev-parse HEAD)"
    ipalpha_feature_json "$record" 'r.repositories[args[0]].featureCommit = args[1]' "$repo" "$head"
    if [[ "$head" != "$base" ]] || git -C "$dir" show-ref --verify --quiet "refs/remotes/origin/feat/$slug"; then
      # Feature branches are rewritten by ./feature rebase; never force over someone else's push.
      git -C "$dir" push -q --force-with-lease origin "feat/$slug" \
        || { ipalpha_feature_fail "push failed: $repo"; return 1; }
    fi
  done

  changed="$(ipalpha_feature_changed_services "$record")"
  echo "$(ipalpha_msg feature_changed): ${changed//$'\n'/ }"
  sha="$(ipalpha_feature_push_record "$main" "$slug" "$record" "[preview] $slug publish" '
    r.generation = (r.generation || 0) + 1;
    r.action = "publish";
    r.requestedAt = args[0];
    r.images = r.images || {};
    for (const s of args[1].split("\n").filter(Boolean)) r.images[s] = {};
  ' "$(ipalpha_feature_now)" "$changed")" || return 1
  echo "  record: ${sha:0:12} (deployment master previews/$slug/)"
  [[ "$wait" == true ]] || return 0
  ipalpha_feature_wait "$main" "$slug" "$sha"
}

ipalpha_feature_confirm() {
  local slug="$1" yes="$2" answer
  [[ "$yes" == true ]] && return 0
  printf '%s %s: ' "$(ipalpha_msg feature_confirm)" "$slug"
  read -r answer || answer=""
  [[ "$answer" == "$slug" ]] || { echo "$(ipalpha_msg publish_aborted)"; return 1; }
}

ipalpha_feature_request() {
  local root="$1" slug="$2" action="$3" wait="$4" main froot record sha
  main="$(ipalpha_feature_main_root "$root")"
  froot="$(ipalpha_feature_dir "$main" "$slug")"
  record="$froot/.ipalpha/release.json"
  [[ -f "$record" ]] || { ipalpha_feature_fail "$(ipalpha_msg feature_missing): $slug"; return 1; }
  sha="$(ipalpha_feature_push_record "$main" "$slug" "$record" "[preview] $slug $action" '
    r.action = args[0];
    r.requestedAt = args[1];
    if (args[0] === "destroy") r.expiresAt = args[1];
  ' "$action" "$(ipalpha_feature_now)")" || return 1
  echo "  record: ${sha:0:12} ($action)"
  [[ "$wait" == true ]] || return 0
  ipalpha_feature_wait "$main" "$slug" "$sha"
}

ipalpha_feature_destroy() {
  local root="$1" slug="$2" yes="$3" wait="$4" force="$5" main froot repo dir
  main="$(ipalpha_feature_main_root "$root")"
  froot="$(ipalpha_feature_dir "$main" "$slug")"
  [[ "$root" != "$froot" ]] || { ipalpha_feature_fail "run destroy from the main workspace"; return 1; }
  for repo in $(ipalpha_feature_repos); do
    dir="$(ipalpha_repo_path "$froot" "$repo")"
    ipalpha_is_git_repo "$dir" || continue
    if [[ "$force" != true && -n "$(git -C "$dir" status --porcelain)" ]]; then
      ipalpha_feature_fail "$repo has uncommitted changes (commit, ./publish --feature, or --force)"; return 1
    fi
  done
  ipalpha_feature_confirm "$slug" "$yes" || return 1
  ipalpha_feature_request "$root" "$slug" destroy "$wait" || return 1
  # Branches stay (local and pushed); only the worktrees and the folder go.
  for repo in $(ipalpha_feature_repos); do
    dir="$(ipalpha_repo_path "$froot" "$repo")"
    ipalpha_is_git_repo "$dir" || continue
    git -C "$(ipalpha_repo_path "$main" "$repo")" worktree remove --force "$dir"
  done
  [[ -x "$froot/.ipalpha/bin/infra-down" ]] && "$froot/.ipalpha/bin/infra-down" --purge >/dev/null 2>&1 || true
  rm -rf "$froot"
  echo "$(ipalpha_msg feature_destroyed): $slug"
}

ipalpha_feature_rebase() {
  local root="$1" slug="$2" main froot record baseline baseline_commit repo dir old new
  main="$(ipalpha_feature_main_root "$root")"
  froot="$(ipalpha_feature_dir "$main" "$slug")"
  record="$froot/.ipalpha/release.json"
  [[ -f "$record" ]] || { ipalpha_feature_fail "$(ipalpha_msg feature_missing): $slug"; return 1; }
  baseline="$(mktemp)"
  baseline_commit="$(ipalpha_feature_read_baseline "$main" "$baseline")" || { rm -f "$baseline"; return 1; }
  for repo in $(ipalpha_feature_repos); do
    dir="$(ipalpha_repo_path "$froot" "$repo")"
    ipalpha_is_git_repo "$dir" || continue
    [[ -z "$(git -C "$dir" status --porcelain)" ]] || { rm -f "$baseline"; ipalpha_feature_fail "$repo has uncommitted changes"; return 1; }
    old="$(ipalpha_feature_json "$record" 'return r.repositories[args[0]].baseCommit' "$repo")"
    if [[ "$repo" == deployment ]]; then
      new="$(git -C "$dir" rev-parse origin/master)"
    else
      new="$(ipalpha_feature_base_commit "$main" "$baseline" "$repo")" || { rm -f "$baseline"; return 1; }
    fi
    [[ "$old" != "$new" ]] || continue
    git -C "$dir" fetch -q origin || true
    if ! git -C "$dir" rebase -q --onto "$new" "$old"; then
      git -C "$dir" rebase --abort || true
      rm -f "$baseline"
      ipalpha_feature_fail "$repo: rebase conflict — resolve manually (git -C $dir rebase --onto $new $old)"
      return 1
    fi
    ipalpha_feature_json "$record" '
      Object.assign(r.repositories[args[0]], { baseCommit: args[1], featureCommit: args[2] });
    ' "$repo" "$new" "$(git -C "$dir" rev-parse HEAD)"
    echo "  $repo: ${old:0:12} → ${new:0:12}"
  done
  ipalpha_feature_json "$record" '
    const b = JSON.parse(require("fs").readFileSync(args[0], "utf8"));
    r.baselineRelease = { id: b.release || null, deploymentCommit: args[1] };
  ' "$baseline" "$baseline_commit"
  cp "$baseline" "$froot/.ipalpha/baseline.json"
  rm -f "$baseline"
  echo "$(ipalpha_msg feature_rebased): $slug"
}

ipalpha_feature_list() {
  local root="$1" main dir slug remote
  main="$(ipalpha_feature_main_root "$root")"
  git -C "$main/deployment" fetch -q origin master 2>/dev/null || true
  printf '%-30s %-5s %-22s %s\n' feature gen expires state
  for dir in "$main"/features/*/; do
    [[ -f "$dir/.ipalpha/release.json" ]] || continue
    slug="$(basename "$dir")"
    remote="$(git -C "$main/deployment" show "origin/master:previews/$slug/release.json" 2>/dev/null || echo '{}')"
    node -e '
      const r = JSON.parse(process.argv[2]);
      const exp = r.expiresAt ? new Date(r.expiresAt) : null;
      const state = !r.generation ? "local only" : !exp ? "publishing" : exp < new Date() ? "expired" : "live";
      console.log(process.argv[1].padEnd(30), String(r.generation || 0).padEnd(5), (r.expiresAt || "-").padEnd(22), state);
    ' "$slug" "$remote"
  done
  if command -v kubectl >/dev/null 2>&1; then
    kubectl get ns -l ipalpha.dev/preview=true \
      -o custom-columns='NAMESPACE:.metadata.name,EXPIRES:.metadata.annotations.ipalpha\.dev/expires-at' 2>/dev/null || true
  fi
}

ipalpha_feature_help() {
  ipalpha_msg help_feature
}

ipalpha_feature() {
  local root="$1"; shift
  local cmd="${1:-}" slug="" yes=false wait=true force=false arg
  [[ $# -gt 0 ]] && shift
  for arg in "$@"; do
    case "$arg" in
      -y|--yes) yes=true ;;
      --no-wait) wait=false ;;
      --force) force=true ;;
      -*) ipalpha_feature_fail "unknown flag: $arg"; return 1 ;;
      *) slug="$arg" ;;
    esac
  done
  # Inside a feature workspace the slug defaults to that feature.
  [[ -n "$slug" ]] || slug="$(ipalpha_feature_env_get "$root" slug 2>/dev/null || true)"
  case "$cmd" in
    new) [[ -n "$slug" ]] || { ipalpha_feature_help; return 1; }; ipalpha_feature_new "$root" "$slug" ;;
    list|ls) ipalpha_feature_list "$root" ;;
    rebase) ipalpha_feature_rebase "$root" "$slug" ;;
    extend) ipalpha_feature_request "$root" "$slug" extend "$wait" ;;
    reset)
      ipalpha_feature_confirm "$slug" "$yes" || return 1
      ipalpha_feature_request "$root" "$slug" reset "$wait"
      ;;
    destroy) ipalpha_feature_destroy "$root" "$slug" "$yes" "$wait" "$force" ;;
    *) ipalpha_feature_help; [[ "$cmd" == -h || "$cmd" == --help || "$cmd" == help ]] ;;
  esac
}
