#!/usr/bin/env bash
# ./feature + ./publish --feature against local bare origins (no network, no TeamCity).
set -euo pipefail

ipalpha_repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
ipalpha_tmp="$(mktemp -d)"
trap '[[ -n "${KEEP:-}" ]] || rm -rf "$ipalpha_tmp"' EXIT

export IPALPHA_TEST_NO_PORT_PROBE=1
export IPALPHA_CLONE_COMMAND="$ipalpha_repo_root/tests/local-clone"
export IPALPHA_TEST_ORIGINS="$ipalpha_tmp/origins"
export IPALPHA_LANG=en-US
export IPALPHA_TARGET_DIR="$ipalpha_tmp/IpAlpha"
export IPALPHA_SKIP_INSTALL=1
export IPALPHA_NO_SHELL=1
export GIT_AUTHOR_NAME=test GIT_AUTHOR_EMAIL=test@example.invalid
export GIT_COMMITTER_NAME=test GIT_COMMITTER_EMAIL=test@example.invalid

ipalpha_fail() { echo "FAIL: $1" >&2; exit 1; }

repos=(deployment shared-js shared-ui projects-api persons-api organizations-api notifications-api auth-api
  forms-api dispatch-api auth-webapp forms-webapp mordomia-webapp)
apis=(projects-api persons-api organizations-api notifications-api auth-api forms-api dispatch-api)

echo "== origins"
seed="$ipalpha_tmp/seed"
for repo in "${repos[@]}"; do
  git init -q --bare -b master "$IPALPHA_TEST_ORIGINS/$repo.git"
  git clone -q "$IPALPHA_TEST_ORIGINS/$repo.git" "$seed/$repo" 2>/dev/null
  echo "# $repo" >"$seed/$repo/README.md"
  # persons-api's .gitignore lacks .env on purpose: the feature worktree must still never stage it.
  if [[ "$repo" == persons-api ]]; then echo node_modules >"$seed/$repo/.gitignore"; else printf '.env\nnode_modules\n' >"$seed/$repo/.gitignore"; fi
  case "$repo" in
    *-api)
      printf '{"name":"%s","version":"0.1.0"}\n' "$repo" >"$seed/$repo/package.json"
      printf '{"packages":{"node_modules/@ipalpha/shared-js":{"version":"1.0.0"}}}\n' >"$seed/$repo/package-lock.json"
      ;;
    shared-js) printf '{"name":"@ipalpha/shared-js","version":"1.0.0"}\n' >"$seed/$repo/package.json" ;;
  esac
  git -C "$seed/$repo" add -A
  git -C "$seed/$repo" commit -q -m init
  [[ "$repo" != shared-js ]] || git -C "$seed/$repo" tag v1.0.0
  git -C "$seed/$repo" push -q --tags origin master
done
# Baseline pins the init commits; master moves on afterwards, the feature must not follow it.
node -e '
  const [seed, ...apis] = process.argv.slice(1), cp = require("child_process");
  const head = r => cp.execSync(`git -C ${seed}/${r} rev-parse HEAD`).toString().trim();
  const services = {};
  for (const r of [...apis, "auth-webapp", "forms-webapp", "mordomia-webapp", "ai-api"])
    services[r] = { image: `registry.kevyn.com.br/ip-alpha/core/${r}@sha256:${"a".repeat(64)}`,
      sourceCommit: r === "ai-api" ? "0".repeat(40) : head(r) };
  require("fs").mkdirSync(`${seed}/deployment/releases`, { recursive: true });
  require("fs").writeFileSync(`${seed}/deployment/releases/core-latest.json`, JSON.stringify({
    schemaVersion: 1, release: "core-deploy-1", services,
    libraries: { "shared-ui": { commit: head("shared-ui") } } }, null, 2));
' "$seed" "${apis[@]}"
git -C "$seed/deployment" add -A && git -C "$seed/deployment" commit -q -m baseline && git -C "$seed/deployment" push -q origin master
auth_base="$(git -C "$seed/auth-api" rev-parse HEAD)"
echo more >>"$seed/auth-api/README.md"
git -C "$seed/auth-api" commit -qam "after baseline" && git -C "$seed/auth-api" push -q origin master

echo "== setup"
"$ipalpha_repo_root/setup" --skip-tools --keep-setup >"$ipalpha_tmp/setup.log" 2>&1 \
  || { cat "$ipalpha_tmp/setup.log"; ipalpha_fail "setup failed"; }
root="$IPALPHA_TARGET_DIR"
[[ -x "$root/feature" ]] || ipalpha_fail "./feature not generated"
grep -q '^infra_name=ipalpha$' "$root/.ipalpha/settings" || ipalpha_fail "main infra_name missing"
grep -q '^IPALPHA_INFRA_NAME=ipalpha$' "$root/.ipalpha/ports.env" || ipalpha_fail "main ports.env infra name"

echo "== slug validation"
for bad in ab Bad_Slug -lead trail- "$(printf 'a%.0s' {1..31})" "has.dot"; do
  if (cd "$root" && ./feature new "$bad" >/dev/null 2>&1); then ipalpha_fail "accepted bad slug: $bad"; fi
done
[[ ! -d "$root/features" ]] || ipalpha_fail "bad slug created a folder"

echo "== feature new"
out="$(cd "$root" && ./feature new hello-test 2>&1)" || ipalpha_fail "feature new failed: $out"
froot="$root/features/hello-test"
for repo in "${repos[@]}"; do
  dir="$froot/core/$repo"; [[ "$repo" == deployment ]] && dir="$froot/deployment"
  [[ -f "$dir/.git" ]] || ipalpha_fail "$repo is not a worktree (.git file)"
  [[ "$(git -C "$dir" symbolic-ref --short HEAD)" == feat/hello-test ]] || ipalpha_fail "$repo not on feat/hello-test"
done
[[ "$(git -C "$froot/core/auth-api" rev-parse HEAD)" == "$auth_base" ]] || ipalpha_fail "auth-api not pinned to baseline"
[[ "$(git -C "$froot/core/shared-js" rev-parse HEAD)" == "$(git -C "$root/core/shared-js" rev-parse v1.0.0)" ]] \
  || ipalpha_fail "shared-js not pinned to the locked version tag"
[[ -z "$(git -C "$root/core/auth-api" status --porcelain)" ]] || ipalpha_fail "main checkout touched"
[[ -f "$froot/core/persons-api/.env" && -z "$(git -C "$froot/core/persons-api" status --porcelain)" ]] \
  || ipalpha_fail "worktree .env is not excluded"
[[ "$(git -C "$root/core/auth-api" symbolic-ref --short HEAD)" == master ]] || ipalpha_fail "main branch changed"
for f in run publish pull feature; do [[ -x "$froot/$f" ]] || ipalpha_fail "feature ./$f missing"; done
grep -q '^IPALPHA_INFRA_NAME=ipalpha-hello-test$' "$froot/.ipalpha/ports.env" || ipalpha_fail "feature infra name"
grep -q '^slug=hello-test$' "$froot/.ipalpha/feature.env" || ipalpha_fail "feature.env slug"
grep -q '^mongo_port=27117$' "$froot/.ipalpha/settings" || ipalpha_fail "feature mongo port not offset"
grep -q '^forms-webapp_port=5206$' "$froot/.ipalpha/settings" || ipalpha_fail "feature web port not offset"
grep -q '^PORT=3101$' "$froot/core/projects-api/.env" || ipalpha_fail "feature .env PORT not rewritten"
grep -q '127.0.0.1:3105' "$froot/core/persons-api/.env" || ipalpha_fail "feature .env peer URL not rewritten"
grep -q '^PORT=3001$' "$root/core/projects-api/.env" || ipalpha_fail "main .env changed"
node -e '
  const r = require(process.argv[1]);
  const ok = r.schemaVersion === 1 && r.feature === "hello-test" && r.namespace === "ipalpha-feat-hello-test"
    && r.generation === 0 && r.expiresAt === null && r.baselineRelease.id === "core-deploy-1"
    && r.hosts.join(" ") === "ipalpha-hello-test.kevyn.com.br forms-ipalpha-hello-test.kevyn.com.br auth-ipalpha-hello-test.kevyn.com.br"
    && Object.values(r.repositories).every(x => x.baseCommit === x.featureCommit && x.baseRef === "master");
  if (!ok) { console.error(JSON.stringify(r, null, 2)); process.exit(1); }
' "$froot/.ipalpha/release.json" || ipalpha_fail "draft release.json wrong"

echo "== second feature gets another port block"
(cd "$root" && ./feature new second-one >/dev/null 2>&1) || ipalpha_fail "second feature failed"
grep -q '^mongo_port=27217$' "$root/features/second-one/.ipalpha/settings" || ipalpha_fail "second feature port block"
(cd "$root" && ./feature new second-one >/dev/null 2>&1) && ipalpha_fail "duplicate feature accepted"

echo "== failed new rolls back"
git -C "$root/core/mordomia-webapp" checkout -q -b feat/rollback-me
(cd "$root" && ./feature new rollback-me >/dev/null 2>&1) && ipalpha_fail "new succeeded with a branch checked out elsewhere"
[[ ! -e "$root/features/rollback-me" ]] || ipalpha_fail "rollback left the folder"
git -C "$root/core/auth-api" rev-parse -q --verify refs/heads/feat/rollback-me >/dev/null && ipalpha_fail "rollback left a created branch"
[[ -z "$(git -C "$root/core/auth-api" worktree list | grep rollback-me)" ]] || ipalpha_fail "rollback left a worktree"
git -C "$root/core/mordomia-webapp" rev-parse -q --verify refs/heads/feat/rollback-me >/dev/null || ipalpha_fail "rollback deleted a pre-existing branch"
git -C "$root/core/mordomia-webapp" checkout -q master

echo "== publish --feature"
inplace() { if sed --version >/dev/null 2>&1; then sed -i "$@"; else sed -i '' "$@"; fi; }
inplace 's/^ai_cli=.*/ai_cli=bogus/' "$froot/.ipalpha/settings"
echo "change" >>"$froot/core/forms-webapp/README.md"
out="$(cd "$froot" && ./publish --dry-run </dev/null 2>&1)" || ipalpha_fail "dry-run failed: $out"
[[ -n "$(git -C "$froot/core/forms-webapp" status --porcelain)" ]] || ipalpha_fail "dry-run committed"
git -C "$IPALPHA_TEST_ORIGINS/deployment.git" show master:previews/hello-test/release.json >/dev/null 2>&1 \
  && ipalpha_fail "dry-run pushed a record"
(cd "$froot" && ./publish --feature hello-test --folder forms-webapp </dev/null >/dev/null 2>&1) \
  && ipalpha_fail "--feature accepted release flags"
out="$(cd "$froot" && ./publish --no-wait </dev/null 2>&1)" || ipalpha_fail "publish --feature failed: $out"
grep -q 'Changed services: forms-webapp$' <<<"$out" || ipalpha_fail "changed services wrong: $out"
git -C "$IPALPHA_TEST_ORIGINS/forms-webapp.git" rev-parse -q --verify refs/heads/feat/hello-test >/dev/null \
  || ipalpha_fail "feat/hello-test not pushed for forms-webapp"
if git -C "$IPALPHA_TEST_ORIGINS/auth-api.git" rev-parse -q --verify refs/heads/feat/hello-test >/dev/null; then
  ipalpha_fail "unchanged auth-api branch pushed"
fi
[[ "$(node -p 'require(process.argv[1]).version' "$froot/core/auth-api/package.json")" == 0.1.0 ]] || ipalpha_fail "version bumped"
[[ -z "$(git -C "$froot/core/forms-webapp" tag)" ]] || ipalpha_fail "prerelease tag created"
record="$(git -C "$IPALPHA_TEST_ORIGINS/deployment.git" show master:previews/hello-test/release.json)" \
  || ipalpha_fail "record not on deployment master"
node -e '
  const r = JSON.parse(process.argv[1]), f = r.repositories["forms-webapp"], a = r.repositories["auth-api"];
  if (!(r.generation === 1 && r.action === "publish" && r.requestedAt && f.featureCommit !== f.baseCommit
        && a.featureCommit === a.baseCommit && "forms-webapp" in r.images)) { console.error(r); process.exit(1); }
' "$record" || ipalpha_fail "published record wrong"
[[ "$(git -C "$IPALPHA_TEST_ORIGINS/deployment.git" diff --name-only master~1 master)" == previews/hello-test/release.json ]] \
  || ipalpha_fail "record commit touched more than previews/hello-test/"

echo "== dependency closure"
echo "x" >>"$froot/core/shared-js/README.md"
(cd "$froot" && ./publish --feature hello-test --no-wait </dev/null >/dev/null 2>&1) || ipalpha_fail "shared-js publish failed"
record="$(git -C "$IPALPHA_TEST_ORIGINS/deployment.git" show master:previews/hello-test/release.json)"
node -e '
  const r = JSON.parse(process.argv[1]);
  const need = ["auth-api","dispatch-api","forms-api","notifications-api","organizations-api","persons-api","projects-api","forms-webapp"];
  if (r.generation !== 2 || !need.every(s => s in r.images) || "auth-webapp" in r.images) { console.error(r); process.exit(1); }
' "$record" || ipalpha_fail "shared-js closure wrong"

echo "== CI fields on master survive the next publish"
ci="$ipalpha_tmp/ci"; git clone -q "$IPALPHA_TEST_ORIGINS/deployment.git" "$ci"
node -e '
  const fs = require("fs"), f = process.argv[1], r = JSON.parse(fs.readFileSync(f));
  r.expiresAt = "2099-01-01T00:00:00Z"; r.images["auth-webapp"] = { digest: "sha256:ci" };
  fs.writeFileSync(f, JSON.stringify(r, null, 2));
' "$ci/previews/hello-test/release.json"
git -C "$ci" commit -qam "[preview] hello-test gen 2" && git -C "$ci" push -q origin master
(cd "$root" && ./feature extend hello-test --no-wait >/dev/null 2>&1) || ipalpha_fail "extend failed"
record="$(git -C "$IPALPHA_TEST_ORIGINS/deployment.git" show master:previews/hello-test/release.json)"
node -e '
  const r = JSON.parse(process.argv[1]);
  if (r.action !== "extend" || r.expiresAt !== "2099-01-01T00:00:00Z" || r.images["auth-webapp"].digest !== "sha256:ci" || r.generation !== 2)
    { console.error(r); process.exit(1); }
' "$record" || ipalpha_fail "extend lost CI fields"

echo "== a publish keeps the previous image entries (CI reuses unchanged builds)"
echo again >>"$froot/core/forms-webapp/README.md"
(cd "$froot" && ./publish --no-wait </dev/null >/dev/null 2>&1) || ipalpha_fail "republish failed"
git -C "$IPALPHA_TEST_ORIGINS/deployment.git" show master:previews/hello-test/release.json | node -e '
  let s="";process.stdin.on("data",d=>s+=d).on("end",()=>{const r=JSON.parse(s); process.exit(r.images["auth-webapp"] && r.images["auth-webapp"].digest==="sha256:ci"?0:1)})' \
  || ipalpha_fail "publish clobbered previous image entries"

echo "== teammate push is never overwritten"
f2="$root/features/second-one"
inplace 's/^ai_cli=.*/ai_cli=bogus/' "$f2/.ipalpha/settings"
echo a >>"$f2/core/forms-webapp/README.md"
(cd "$f2" && ./publish --no-wait </dev/null >/dev/null 2>&1) || ipalpha_fail "second-one publish failed"
mate="$ipalpha_tmp/mate"; git clone -q -b feat/second-one "$IPALPHA_TEST_ORIGINS/forms-webapp.git" "$mate"
echo mate >>"$mate/README.md"; git -C "$mate" commit -qam "teammate work"; git -C "$mate" push -q origin feat/second-one
git -C "$f2/core/forms-webapp" fetch -q origin   # a fetch must not turn into permission to overwrite
echo b >>"$f2/core/forms-webapp/README.md"
(cd "$f2" && ./publish --no-wait </dev/null >/dev/null 2>&1) && ipalpha_fail "publish overwrote a teammate push"
[[ "$(git -C "$IPALPHA_TEST_ORIGINS/forms-webapp.git" log -1 --format=%s feat/second-one)" == "teammate work" ]] \
  || ipalpha_fail "teammate commit lost"

echo "== waits for the CI record (success via Git, no CI token)"
export IPALPHA_TEST_NO_GH=1 IPALPHA_FEATURE_WAIT_MINUTES=1
(
  for _ in $(seq 1 40); do
    git -C "$ci" fetch -q origin master && git -C "$ci" reset -q --hard origin/master
    if node -e 'const r=require(process.argv[1]); process.exit(r.action==="extend" && r.requestedAt && (r.lastResult||{}).requestedAt!==r.requestedAt?0:1)' "$ci/previews/hello-test/release.json"; then
      node -e '
        const fs=require("fs"), f=process.argv[1], r=JSON.parse(fs.readFileSync(f));
        r.lastResult={action:r.action, requestedAt:r.requestedAt, status:"success"}; r.expiresAt="2099-02-01T00:00:00Z";
        fs.writeFileSync(f, JSON.stringify(r, null, 2));' "$ci/previews/hello-test/release.json"
      git -C "$ci" commit -qam "[preview-ci] hello-test extend" && git -C "$ci" push -q origin master && exit 0
    fi
    sleep 0.5
  done
) &
ci_sim=$!
out="$(cd "$root" && ./feature extend hello-test 2>&1)" || ipalpha_fail "extend did not see the CI record: $out"
wait "$ci_sim" || true
grep -q 'https://ipalpha-hello-test.kevyn.com.br' <<<"$out" && grep -q '2099-02-01T00:00:00Z' <<<"$out" \
  || ipalpha_fail "extend output lacks URLs/expiry: $out"
unset IPALPHA_TEST_NO_GH IPALPHA_FEATURE_WAIT_MINUTES

echo "== list"
out="$(cd "$root" && ./feature list 2>&1)" || ipalpha_fail "list failed"
grep -q 'hello-test' <<<"$out" && grep -q 'live' <<<"$out" || ipalpha_fail "list output: $out"

echo "== destroy"
echo dirty >>"$froot/core/forms-webapp/README.md"
(cd "$root" && ./feature destroy hello-test --yes --no-wait >/dev/null 2>&1) && ipalpha_fail "destroy ignored local changes"
git -C "$froot/core/forms-webapp" checkout -q -- README.md
(cd "$root" && ./feature destroy hello-test </dev/null >/dev/null 2>&1) && ipalpha_fail "destroy without confirmation"
out="$(cd "$root" && echo hello-test | ./feature destroy hello-test --no-wait 2>&1)" || ipalpha_fail "destroy failed: $out"
[[ ! -e "$froot" ]] || ipalpha_fail "feature folder still there"
git -C "$root/core/forms-webapp" rev-parse -q --verify refs/heads/feat/hello-test >/dev/null || ipalpha_fail "branch deleted"
[[ -z "$(git -C "$root/core/forms-webapp" worktree list | grep hello-test)" ]] || ipalpha_fail "worktree still registered"
record="$(git -C "$IPALPHA_TEST_ORIGINS/deployment.git" show master:previews/hello-test/release.json)"
node -e '
  const r = JSON.parse(process.argv[1]);
  if (r.action !== "destroy" || new Date(r.expiresAt) > new Date()) { console.error(r); process.exit(1); }
' "$record" || ipalpha_fail "destroy record wrong"

echo "== new reuses a kept branch"
out="$(cd "$root" && ./feature new hello-test 2>&1)" || ipalpha_fail "re-new failed: $out"
node -e '
  const r = require(process.argv[1]), f = r.repositories["forms-webapp"];
  if (f.featureCommit === f.baseCommit) { console.error(f); process.exit(1); }
' "$froot/.ipalpha/release.json" || ipalpha_fail "reused branch commits not reflected"
[[ "$(git -C "$froot/core/forms-webapp" log -1 --format=%s)" != init ]] || ipalpha_fail "reused branch lost its commits"

echo "feature-test: all assertions passed"
