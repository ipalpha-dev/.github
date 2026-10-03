#!/usr/bin/env bash
# Feature environments: one isolated workspace (git worktrees on feat/<slug>) + one preview
# namespace per feature. Design: deployment/docs/feature-environments.md (decisions are final).
# The cluster side only reacts to previews/<slug>/release.json pushed to deployment master; this
# file never talks to Kubernetes for writes and never holds a TeamCity token (TeamCity reports the
# outcome as a GitHub commit status and as its record commit on deployment master).
# shellcheck disable=SC2154,SC2034  # workspace globals live in common.sh / settings.sh / ports.sh

ipalpha_feature_domain="${IPALPHA_PREVIEW_DOMAIN:-kevyn.com.br}"
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
  # Spec rule: every whole host ≤ 63 chars (stricter than the per-label DNS limit).
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


ipalpha_feature_now() { date -u +%Y-%m-%dT%H:%M:%SZ; }

# Baseline = releases/core-latest.json on deployment master (last green Core Deploy), or as of an
# earlier deployment commit with --baseline <rev> (e.g. to test a branch in the world it was written
# for; `git log deployment/releases/core-latest.json` lists the recorded baselines).
ipalpha_feature_read_baseline() {
  local main="$1" out="$2" dep="$1/deployment" ref
  git -C "$dep" fetch -q origin master || { ipalpha_feature_fail "cannot fetch deployment master"; return 1; }
  ref="$(git -C "$dep" rev-parse --verify -q "${ipalpha_feature_baseline_ref:-origin/master}^{commit}")" \
    || { ipalpha_feature_fail "unknown deployment revision: $ipalpha_feature_baseline_ref"; return 1; }
  git -C "$dep" merge-base --is-ancestor "$ref" origin/master \
    || { ipalpha_feature_fail "--baseline must be a commit on deployment master"; return 1; }
  git -C "$dep" show "$ref:releases/core-latest.json" >"$out" 2>/dev/null \
    || { ipalpha_feature_fail "$(ipalpha_msg feature_no_baseline)"; return 1; }
  echo "$ref"
}

# Commit the baseline pins for a repository. shared-js is consumed from npm, so its commit is the
# release tag of the version locked by the APIs. deployment starts at the commit whose base/ the
# baseline images were deployed with (core-latest.json deployment.commit): previews render base/
# from feat/<slug>, so manifests always match the images (Kevyn).
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
    const s = r === "deployment" ? b.deployment : ((b.services || {})[r] || (b.libraries || {})[r]);
    process.stdout.write((s && (s.sourceCommit || s.commit)) || "");
  ' "$baseline" "$repo")"
  [[ -n "$commit" ]] || { ipalpha_feature_fail "baseline has no commit for $repo"; return 1; }
  echo "$commit"
}

# Fetch every main clone in parallel (baseline commits/tags must be local before resolving).
ipalpha_feature_fetch_all() {
  local main="$1" repo dir
  local -a pids=()
  for repo in $(ipalpha_all_repos); do
    dir="$(ipalpha_repo_path "$main" "$repo")"
    ipalpha_is_git_repo "$dir" || continue
    git -C "$dir" fetch -q --tags origin 2>/dev/null &
    pids+=("$!")
  done
  for repo in "${pids[@]}"; do wait "$repo" || echo "  warning: a fetch failed (offline?)" >&2; done
}

# Prints the commit to record as baseCommit. A reused feat/<slug> (kept by destroy, or pushed by a
# teammate) keeps its commits; its base becomes the merge-base so ./feature rebase moves it.
ipalpha_feature_add_worktree() {
  local src="$1" dest="$2" branch="$3" base="$4"
  ipalpha_is_git_repo "$src" || { ipalpha_feature_fail "missing repository: $src"; return 1; }
  git -C "$src" cat-file -e "$base^{commit}" 2>/dev/null \
    || { ipalpha_feature_fail "commit $base not found in $src"; return 1; }
  mkdir -p "$(dirname "$dest")"
  if git -C "$src" show-ref --verify --quiet "refs/heads/$branch"; then
    git -C "$src" worktree add -q "$dest" "$branch" >&2 || return 1
  elif git -C "$src" show-ref --verify --quiet "refs/remotes/origin/$branch"; then
    git -C "$src" worktree add -q --track -b "$branch" "$dest" "origin/$branch" >&2 || return 1
  else
    git -C "$src" worktree add -q --no-track -b "$branch" "$dest" "$base" >&2 || return 1
    echo "$base"; return 0
  fi
  if git -C "$dest" merge-base --is-ancestor HEAD "$base"; then
    git -C "$dest" merge -q --ff-only "$base" >&2 && { echo "$base"; return 0; }
  fi
  echo "  $(basename "$src"): $branch already has commits — base = merge-base (./feature rebase to move it)" >&2
  git -C "$dest" merge-base HEAD "$base"
}

# Undo a half-created feature: its worktrees, the branches this run created, the folder.
ipalpha_feature_rollback() {
  local main="$1" froot="$2" repo dir entry
  for repo in $(ipalpha_all_repos); do
    dir="$(ipalpha_repo_path "$froot" "$repo")"
    [[ -e "$dir/.git" ]] && git -C "$(ipalpha_repo_path "$main" "$repo")" worktree remove --force "$dir" 2>/dev/null
  done
  for entry in "${ipalpha_feature_created_branches[@]}"; do
    git -C "${entry% *}" branch -q -D "${entry#* }" 2>/dev/null || true
  done
  rm -rf "$froot"
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
    done < <(ipalpha_port_vars)
    [[ "$ok" == 1 ]] && break
  done
  [[ "$ok" == 1 ]] || { ipalpha_feature_fail "no free port block for another feature"; return 1; }
  local -a from=() to=()
  while read -r var default; do
    main_port="${!var:-$default}"
    ipalpha_resolve_port "$var" "$((default + offset))" >/dev/null
    from+=("$main_port"); to+=("${!var}")
  done < <(ipalpha_port_vars)
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
      const [repo, base, head] = line.split(" ");
      repositories[repo] = { url: `git@github.com:${org}/${repo}.git`, baseRef: "master",
        baseCommit: base, featureCommit: head };
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

# Local secrets copied into a worktree must never be committable: ignore them through the shared
# info/exclude (no tracked .gitignore change), whatever the branch's .gitignore says.
ipalpha_feature_exclude_env() {
  local dir="$1" exclude
  exclude="$(git -C "$dir" rev-parse --path-format=absolute --git-common-dir)/info/exclude"
  mkdir -p "$(dirname "$exclude")"
  grep -qxF '.env' "$exclude" 2>/dev/null || echo '.env' >>"$exclude"
}

ipalpha_feature_new() {
  local root="$1" slug="$2" main froot baseline baseline_commit repo base repos_file dest
  main="$(ipalpha_feature_main_root "$root")"
  ipalpha_feature_validate_slug "$slug" || return 1
  froot="$(ipalpha_feature_dir "$main" "$slug")"
  [[ ! -e "$froot" ]] || { ipalpha_feature_fail "$(ipalpha_msg feature_exists): $froot"; return 1; }
  ipalpha_load_settings "$main" >/dev/null 2>&1 || true

  baseline="$(mktemp)"; repos_file="$(mktemp)"
  ipalpha_feature_created_branches=()
  if ! ipalpha_feature_new_into "$main" "$froot" "$slug" "$baseline" "$repos_file"; then
    ipalpha_feature_rollback "$main" "$froot"
    rm -f "$baseline" "$repos_file"
    return 1
  fi
  rm -f "$baseline" "$repos_file"

  echo
  echo "$(ipalpha_msg feature_created): $froot"
  echo "  cd features/$slug && ./run        # mongo :$ipalpha_port_mongo · web :$ipalpha_port_mordomia_webapp"
  echo "  ./publish --feature $slug         # preview:"
  for host in $(ipalpha_feature_hosts "$slug"); do echo "    https://$host"; done
}

ipalpha_feature_new_into() {
  local main="$1" froot="$2" slug="$3" baseline="$4" repos_file="$5" baseline_commit repo base dest src had_branch
  baseline_commit="$(ipalpha_feature_read_baseline "$main" "$baseline")" || return 1
  echo "$(ipalpha_msg feature_creating) $slug ($(node -p 'JSON.parse(require("fs").readFileSync(process.argv[1],"utf8")).release' "$baseline"))"
  ipalpha_feature_fetch_all "$main"
  for repo in $(ipalpha_all_repos); do
    base="$(ipalpha_feature_base_commit "$main" "$baseline" "$repo")" || return 1
    dest="$(ipalpha_repo_path "$froot" "$repo")"
    src="$(ipalpha_repo_path "$main" "$repo")"
    git -C "$src" show-ref --verify --quiet "refs/heads/feat/$slug" && had_branch=true || had_branch=false
    base="$(ipalpha_feature_add_worktree "$src" "$dest" "feat/$slug" "$base")" || return 1
    # Runs in this shell (not the $(...) above) so a rollback knows which branches are ours.
    [[ "$had_branch" == true ]] || ipalpha_feature_created_branches+=("$src feat/$slug")
    ipalpha_feature_exclude_env "$dest"
    # The push lease starts from what origin already has (a teammate's branch we build on).
    if git -C "$dest" show-ref --verify --quiet "refs/remotes/origin/feat/$slug"; then
      mkdir -p "$froot/.ipalpha"
      echo "$repo $(git -C "$dest" rev-parse "refs/remotes/origin/feat/$slug")" >>"$froot/.ipalpha/pushed"
    fi
    echo "  $repo → feat/$slug @ ${base:0:12}"
    echo "$repo $base $(git -C "$dest" rev-parse HEAD)" >>"$repos_file"
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

# Outcome without any CI token on the laptop: success = CI's record commit on deployment master
# (lastResult.requestedAt == ours; destroy = record archived); failure = the GitHub commit status
# TeamCity publishes on our record commit (read with the developer's own gh login).
ipalpha_feature_wait() {
  local main="$1" slug="$2" sha="$3" requested="$4" action="$5" deadline record state last=""
  echo "$(ipalpha_msg feature_waiting) (${sha:0:12})"
  deadline=$(( $(date +%s) + ipalpha_feature_wait_minutes * 60 ))
  while (( $(date +%s) < deadline )); do
    git -C "$main/deployment" fetch -q origin master 2>/dev/null || true
    record="$(git -C "$main/deployment" show "origin/master:previews/$slug/release.json" 2>/dev/null || true)"
    if [[ "$action" == destroy && -z "$record" ]]; then
      echo "  $(ipalpha_msg feature_destroyed): $slug"
      return 0
    fi
    if [[ -n "$record" ]] && node -e '
      const r = JSON.parse(process.argv[1]), l = r.lastResult || {};
      process.exit(l.requestedAt === process.argv[2] && l.status === "success" ? 0 : 1);
    ' "$record" "$requested"; then
      node -e '
        const r = JSON.parse(process.argv[1]);
        for (const h of r.hosts || []) console.log("  https://" + h);
        console.log("  inbox: https://" + (r.hosts || [""])[0] + "/mailbox");
        console.log("  expires: " + (r.expiresAt || "-") + "   build: " + ((r.teamcityBuild || {}).url || "-"));
      ' "$record"
      return 0
    fi
    if [[ -z "${IPALPHA_TEST_NO_GH:-}" ]] && command -v gh >/dev/null 2>&1; then
      state="$(gh api "repos/${ipalpha_org}/deployment/commits/$sha/status" \
        --jq '[.state, ((.statuses // [])[0].description // ""), ((.statuses // [])[0].target_url // "")] | join("\t")' 2>/dev/null || true)"
      if [[ -n "$state" && "$state" != "$last" ]]; then
        last="$state"
        case "${state%%$'\t'*}" in
          failure|error)
            echo "  $(ipalpha_msg feature_failed): ${state#*$'\t'}" >&2
            return 1
            ;;
          pending) echo "  ${state#*$'\t'}" ;;
        esac
      fi
    fi
    sleep 15
  done
  echo "  $(ipalpha_msg feature_status_unavailable)"
  return 1
}

# ./publish --feature <slug>: commit (bump none) and push feat/<slug>, then the release record.
ipalpha_feature_pushed_get() {
  [[ -f "$1/.ipalpha/pushed" ]] || return 0
  sed -n "s/^$2 //p" "$1/.ipalpha/pushed" | tail -n1
}

ipalpha_feature_pushed_set() {
  local froot="$1" repo="$2" sha="$3" tmp
  tmp="$(mktemp)"
  { grep -v "^$repo " "$froot/.ipalpha/pushed" 2>/dev/null || true; echo "$repo $sha"; } >"$tmp"
  mv "$tmp" "$froot/.ipalpha/pushed"
}

ipalpha_feature_publish() {
  local root="$1" slug="$2" wait="$3" engine="$4" model="$5" dry_run="${6:-false}"
  local main froot record repo dir branch decision message base head sha changed lease
  main="$(ipalpha_feature_main_root "$root")"
  froot="$(ipalpha_feature_dir "$main" "$slug")"
  record="$froot/.ipalpha/release.json"
  [[ -f "$record" ]] || { ipalpha_feature_fail "$(ipalpha_msg feature_missing): $slug"; return 1; }
  ipalpha_feature_validate_slug "$slug" || return 1

  for repo in $(ipalpha_all_repos); do
    dir="$(ipalpha_repo_path "$froot" "$repo")"
    ipalpha_is_git_repo "$dir" || continue
    branch="$(git -C "$dir" symbolic-ref --short -q HEAD || true)"
    [[ "$branch" == "feat/$slug" ]] || { ipalpha_feature_fail "$repo is on '$branch', expected feat/$slug"; return 1; }
    if [[ "$dry_run" == true ]]; then
      [[ -z "$(git -C "$dir" status --porcelain)" ]] || echo "  $repo: uncommitted changes would be committed"
      continue
    fi
    if [[ -n "$(git -C "$dir" status --porcelain)" ]]; then
      decision="$(ipalpha_publish_ask_ai "$froot" "$repo" "$engine" "$model")"
      message="$(sed -n 3p <<<"$decision")"
      git -C "$dir" add -A
      if git -C "$dir" diff --cached --name-only | grep -qE '(^|/)\.env$'; then
        git -C "$dir" reset -q
        ipalpha_feature_fail "$repo: refusing to commit a .env file"; return 1
      fi
      git -C "$dir" commit -q -m "${message:-Update $repo}"
      echo "  $repo: ${message:-Update $repo}"
    fi
    base="$(ipalpha_feature_json "$record" 'return (r.repositories[args[0]]||{}).baseCommit||""' "$repo")"
    head="$(git -C "$dir" rev-parse HEAD)"
    ipalpha_feature_json "$record" 'r.repositories[args[0]].featureCommit = args[1]' "$repo" "$head"
    lease="$(ipalpha_feature_pushed_get "$froot" "$repo")"
    if [[ "$head" != "$base" || -n "$lease" ]] && [[ "$head" != "$lease" ]]; then
      # Rebase rewrites feat/<slug>, hence force — but only over the tip this workspace last
      # pushed (empty lease = the branch must not exist yet), never over a teammate's push.
      git -C "$dir" push -q --force-with-lease="feat/$slug:$lease" origin "feat/$slug" \
        || { ipalpha_feature_fail "push rejected for $repo (someone else pushed feat/$slug?)"; return 1; }
      ipalpha_feature_pushed_set "$froot" "$repo" "$head"
    fi
  done
  if [[ "$dry_run" == true ]]; then
    echo "$(ipalpha_msg feature_changed) (committed): $(ipalpha_feature_changed_services "$record" | tr '\n' ' ')"
    return 0
  fi

  changed="$(ipalpha_feature_changed_services "$record")"
  echo "$(ipalpha_msg feature_changed): ${changed//$'\n'/ }"
  requested="$(ipalpha_feature_now)"
  sha="$(ipalpha_feature_push_record "$main" "$slug" "$record" "[preview] $slug publish" '
    r.generation = (r.generation || 0) + 1;
    r.action = "publish";
    r.requestedAt = args[0];
    r.images = r.images || {};
    // Keep the previous digest/inputHash so CI skips rebuilding what did not change since.
    for (const s of args[1].split("\n").filter(Boolean)) r.images[s] = r.images[s] || {};
  ' "$requested" "$changed")" || return 1
  echo "  record: ${sha:0:12} (deployment master previews/$slug/)"
  [[ "$wait" == true ]] || return 0
  ipalpha_feature_wait "$main" "$slug" "$sha" "$requested" publish
}

ipalpha_feature_confirm() {
  local slug="$1" yes="$2" answer
  [[ "$yes" == true ]] && return 0
  printf '%s %s: ' "$(ipalpha_msg feature_confirm)" "$slug"
  read -r answer || answer=""
  [[ "$answer" == "$slug" ]] || { echo "$(ipalpha_msg publish_aborted)"; return 1; }
}

ipalpha_feature_request() {
  local root="$1" slug="$2" action="$3" wait="$4" main froot record sha requested
  main="$(ipalpha_feature_main_root "$root")"
  froot="$(ipalpha_feature_dir "$main" "$slug")"
  record="$froot/.ipalpha/release.json"
  [[ -f "$record" ]] || { ipalpha_feature_fail "$(ipalpha_msg feature_missing): $slug"; return 1; }
  requested="$(ipalpha_feature_now)"
  sha="$(ipalpha_feature_push_record "$main" "$slug" "$record" "[preview] $slug $action" '
    r.action = args[0];
    r.requestedAt = args[1];
    if (args[0] === "destroy") r.expiresAt = args[1];
  ' "$action" "$requested")" || return 1
  echo "  record: ${sha:0:12} ($action)"
  [[ "$wait" == true ]] || return 0
  ipalpha_feature_wait "$main" "$slug" "$sha" "$requested" "$action"
}

ipalpha_feature_destroy() {
  local root="$1" slug="$2" yes="$3" wait="$4" force="$5" main froot repo dir
  main="$(ipalpha_feature_main_root "$root")"
  froot="$(ipalpha_feature_dir "$main" "$slug")"
  [[ "$root" != "$froot" ]] || { ipalpha_feature_fail "run destroy from the main workspace"; return 1; }
  for repo in $(ipalpha_all_repos); do
    dir="$(ipalpha_repo_path "$froot" "$repo")"
    ipalpha_is_git_repo "$dir" || continue
    if [[ "$force" != true && -n "$(git -C "$dir" status --porcelain)" ]]; then
      ipalpha_feature_fail "$repo has uncommitted changes (commit, ./publish --feature, or --force)"; return 1
    fi
  done
  ipalpha_feature_confirm "$slug" "$yes" || return 1
  ipalpha_feature_request "$root" "$slug" destroy "$wait" || return 1
  # Branches stay (local and pushed); only the worktrees and the folder go.
  for repo in $(ipalpha_all_repos); do
    dir="$(ipalpha_repo_path "$froot" "$repo")"
    ipalpha_is_git_repo "$dir" || continue
    git -C "$(ipalpha_repo_path "$main" "$repo")" worktree remove --force "$dir"
  done
  [[ -x "$froot/.ipalpha/bin/infra-down" ]] && "$froot/.ipalpha/bin/infra-down" --purge --volumes >/dev/null 2>&1 || true
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
  for repo in $(ipalpha_all_repos); do
    dir="$(ipalpha_repo_path "$froot" "$repo")"
    ipalpha_is_git_repo "$dir" || continue
    [[ -z "$(git -C "$dir" status --porcelain)" ]] || { rm -f "$baseline"; ipalpha_feature_fail "$repo has uncommitted changes"; return 1; }
    old="$(ipalpha_feature_json "$record" 'return r.repositories[args[0]].baseCommit' "$repo")"
    new="$(ipalpha_feature_base_commit "$main" "$baseline" "$repo")" || { rm -f "$baseline"; return 1; }
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
  ipalpha_feature_baseline_ref=""
  while [[ $# -gt 0 ]]; do
    arg="$1"
    case "$arg" in
      -y|--yes) yes=true ;;
      --no-wait) wait=false ;;
      --force) force=true ;;
      --baseline) shift; ipalpha_feature_baseline_ref="${1:?--baseline needs a deployment commit}" ;;
      --baseline=*) ipalpha_feature_baseline_ref="${arg#*=}" ;;
      -*) ipalpha_feature_fail "unknown flag: $arg"; return 1 ;;
      *) slug="$arg" ;;
    esac
    shift
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
