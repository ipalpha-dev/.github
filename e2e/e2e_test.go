// Package e2e drives the real ipalpha binary against local bare repositories: setup, pull, feature
// environments, publish --feature, destroy. No network, no Docker, no TeamCity. Runs on macOS,
// Linux and Windows (CI matrix). Build tag-free: `go test ./e2e/`.
package e2e

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/ipalpha-dev/tooling/internal/catalog"
	"github.com/ipalpha-dev/tooling/internal/envfile"
)

var bin string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "ipalpha-e2e-bin-")
	if err != nil {
		panic(err)
	}
	bin = filepath.Join(dir, "ipalpha")
	if runtime.GOOS == "windows" {
		bin += ".exe"
	}
	if out, err := exec.Command("go", "build", "-o", bin, "../cmd/ipalpha").CombinedOutput(); err != nil {
		panic(string(out))
	}
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

type env struct {
	t       *testing.T
	tmp     string
	origins string
	seed    string
	root    string
	vars    []string
}

func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	c := exec.Command("git", append([]string{"-C", dir}, args...)...)
	out, err := c.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v in %s: %v\n%s", args, dir, err, out)
	}
	return strings.TrimSpace(string(out))
}

func gitOK(dir string, args ...string) bool {
	return exec.Command("git", append([]string{"-C", dir}, args...)...).Run() == nil
}

func write(t *testing.T, path, text string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

func appendFile(t *testing.T, path, text string) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	f.WriteString(text)
	f.Close()
}

// newEnv creates bare origins for every catalog repo with a baseline in deployment.
func newEnv(t *testing.T) *env {
	tmp := t.TempDir()
	e := &env{t: t, tmp: tmp, origins: filepath.Join(tmp, "origins"), seed: filepath.Join(tmp, "seed"), root: filepath.Join(tmp, "ipalpha-test")}
	e.vars = append(os.Environ(),
		"IPALPHA_CLONE_FROM="+e.origins, "IPALPHA_LANG=en-US", "IPALPHA_SKIP_TOOLS=1", "IPALPHA_SKIP_INSTALL=1",
		"IPALPHA_SUPERUSER_PHONE=11987654321", "IPALPHA_AI_CLI=none", "IPALPHA_NO_SELF_UPDATE=1", "IPALPHA_TEST_NO_PORT_PROBE=1",
		"GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@example.invalid", "GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@example.invalid",
		"CI=1")
	for _, k := range []string{"GIT_AUTHOR_NAME", "GIT_AUTHOR_EMAIL", "GIT_COMMITTER_NAME", "GIT_COMMITTER_EMAIL"} {
		os.Setenv(k, map[string]string{"GIT_AUTHOR_NAME": "test", "GIT_COMMITTER_NAME": "test"}[k]+map[string]string{"GIT_AUTHOR_EMAIL": "test@example.invalid", "GIT_COMMITTER_EMAIL": "test@example.invalid"}[k])
	}
	for _, r := range catalog.Repos {
		bare := filepath.Join(e.origins, r.Name+".git")
		git(t, tmp, "init", "-q", "--bare", "-b", "master", bare)
		s := filepath.Join(e.seed, r.Name)
		git(t, tmp, "clone", "-q", bare, s)
		git(t, s, "config", "user.email", "test@example.invalid")
		git(t, s, "config", "user.name", "test")
		write(t, filepath.Join(s, "README.md"), "# "+r.Name+"\n")
		gi := ".env\nnode_modules\n"
		if r.Name == "persons-api" {
			gi = "node_modules\n" // no .env on purpose: the feature worktree must still never stage it
		}
		write(t, filepath.Join(s, ".gitignore"), gi)
		switch {
		case r.Kind == catalog.KindAPI:
			write(t, filepath.Join(s, "package.json"), fmt.Sprintf(`{"name":%q,"version":"0.1.0"}`+"\n", r.Name))
			write(t, filepath.Join(s, "package-lock.json"), `{"packages":{"node_modules/@ipalpha/shared-js":{"version":"1.0.0"}}}`+"\n")
		case r.Name == "shared-js":
			write(t, filepath.Join(s, "package.json"), `{"name":"@ipalpha/shared-js","version":"1.0.0"}`+"\n")
		case r.Name == "acampa-kids-backend":
			write(t, filepath.Join(s, "package.json"), `{"name":"camping-backend","version":"0.19.0"}`+"\n")
		case r.Name == "deployment":
			write(t, filepath.Join(s, "base/apps/acampa-kids/acampa-kids-backend/backend.yaml"), "        - image: registry.kevyn.com.br/ip-alpha/apps/acampa-kids/backend:0.19.0\n")
			write(t, filepath.Join(s, "base/apps/acampa-kids/acampa-kids-frontend/frontend.yaml"), "        - image: registry.kevyn.com.br/ip-alpha/apps/acampa-kids/frontend:0.27.0\n")
			write(t, filepath.Join(s, "base/apps/acampa-kids/acampa-kids-face-service/face.yaml"), "        - image: registry.kevyn.com.br/ip-alpha/apps/acampa-kids/face:0.1.0\n")
		}
		git(t, s, "add", "-A")
		git(t, s, "commit", "-q", "-m", "init")
		if r.Name == "shared-js" {
			git(t, s, "tag", "v1.0.0")
		}
		if r.Name == "acampa-kids-backend" {
			git(t, s, "tag", "v0.19.0")
		}
		git(t, s, "push", "-q", "--tags", "origin", "master")
	}
	// Baseline pins the init commits; master moves on afterwards.
	services := map[string]any{}
	for _, r := range catalog.Repos {
		if r.Kind == catalog.KindAPI || r.Kind == catalog.KindWeb {
			services[r.Name] = map[string]string{"image": "x@sha256:" + strings.Repeat("a", 64), "sourceCommit": git(t, filepath.Join(e.seed, r.Name), "rev-parse", "HEAD")}
		}
	}
	dep := filepath.Join(e.seed, "deployment")
	baseline := map[string]any{"schemaVersion": 1, "release": "core-deploy-1", "services": services,
		"deployment": map[string]string{"commit": git(t, dep, "rev-parse", "HEAD")},
		"libraries":  map[string]any{"shared-ui": map[string]string{"commit": git(t, filepath.Join(e.seed, "shared-ui"), "rev-parse", "HEAD")}}}
	data, _ := json.MarshalIndent(baseline, "", "  ")
	write(t, filepath.Join(dep, "releases", "core-latest.json"), string(data))
	git(t, dep, "add", "-A")
	git(t, dep, "commit", "-q", "-m", "baseline")
	git(t, dep, "push", "-q", "origin", "master")
	appendFile(t, filepath.Join(e.seed, "auth-api", "README.md"), "more\n")
	git(t, filepath.Join(e.seed, "auth-api"), "commit", "-qam", "after baseline")
	git(t, filepath.Join(e.seed, "auth-api"), "push", "-q", "origin", "master")
	return e
}

func (e *env) run(dir string, args ...string) (string, error) {
	e.t.Helper()
	c := exec.Command(bin, args...)
	c.Dir = dir
	c.Env = e.vars
	c.Stdin = strings.NewReader("")
	out, err := c.CombinedOutput()
	return string(out), err
}

func (e *env) must(dir string, args ...string) string {
	e.t.Helper()
	out, err := e.run(dir, args...)
	if err != nil {
		e.t.Fatalf("ipalpha %v failed: %v\n%s", args, err, out)
	}
	return out
}

func (e *env) setup() {
	e.must(e.tmp, "setup", e.root, "--yes")
}

func TestSetupPullAndWrappers(t *testing.T) {
	e := newEnv(t)
	e.setup()
	for _, r := range catalog.Repos {
		if _, err := os.Stat(filepath.Join(r.Path(e.root), ".git")); err != nil {
			t.Errorf("%s not cloned", r.Rel())
		}
	}
	s := envfile.Read(filepath.Join(e.root, ".ipalpha", "settings"))
	if s["infra_name"] != "ipalpha-test" || s["auth-api_port"] != "3005" || s["browser_apps"] != "auth-webapp oikos-webapp mailpit" || s["ai_cli"] != "none" {
		t.Fatalf("settings: %v", s)
	}
	auth := envfile.Read(filepath.Join(e.root, "core", "auth-api", ".env"))
	if auth["SUPERUSER_PHONE"] != "+5511987654321" || !strings.Contains(auth["SEED_CLIENTS_JSON"], "persons-api") {
		t.Fatalf("auth .env: phone=%q clients=%q", auth["SUPERUSER_PHONE"], auth["SEED_CLIENTS_JSON"])
	}
	if _, err := os.Stat(filepath.Join(e.root, "core", "auth-webapp", ".env")); err == nil {
		t.Fatal("web apps must not get a .env")
	}
	wrapper := "run"
	if runtime.GOOS == "windows" {
		wrapper = "run.cmd"
	}
	for _, c := range []string{"run", "pull", "publish", "feature", "doctor", "status", "stop"} {
		name := c
		if runtime.GOOS == "windows" {
			name += ".cmd"
		}
		if _, err := os.Stat(filepath.Join(e.root, name)); err != nil {
			t.Errorf("wrapper %s missing", name)
		}
	}
	// The wrapper runs the workspace copy of the binary.
	var c *exec.Cmd
	if runtime.GOOS == "windows" {
		c = exec.Command("cmd", "/c", filepath.Join(e.root, wrapper), "--help")
	} else {
		c = exec.Command(filepath.Join(e.root, wrapper), "--help")
	}
	c.Env = e.vars
	if out, err := c.CombinedOutput(); err != nil || !strings.Contains(string(out), "--until-ready") {
		t.Fatalf("wrapper: %v\n%s", err, out)
	}
	// Re-running setup keeps local values.
	f := filepath.Join(e.root, "core", "persons-api", ".env")
	appendFile(t, f, "CUSTOM_VALUE=keep-me\n")
	e.setup()
	if envfile.Read(f)["CUSTOM_VALUE"] != "keep-me" {
		t.Fatal("setup overwrote a local value")
	}
	// pull: new key in .env.example arrives, local values stay.
	s1 := filepath.Join(e.seed, "projects-api")
	write(t, filepath.Join(s1, ".env.example"), "PORT=3001\nBRAND_NEW_KEY=hello\n")
	git(t, s1, "add", "-A")
	git(t, s1, "commit", "-qm", "example")
	git(t, s1, "push", "-q", "origin", "master")
	out := e.must(e.root, "pull")
	if !strings.Contains(out, "BRAND_NEW_KEY") {
		t.Fatalf("pull did not report the new key:\n%s", out)
	}
	if envfile.Read(filepath.Join(e.root, "core", "projects-api", ".env"))["BRAND_NEW_KEY"] != "hello" {
		t.Fatal("pull did not add the new key")
	}
	// doctor runs and reports (no Docker in CI is a failure, not a crash).
	out, _ = e.run(e.root, "doctor")
	if !strings.Contains(out, "System") || strings.Contains(out, "panic") {
		t.Fatalf("doctor:\n%s", out)
	}
	// config set/get
	e.must(e.root, "config", "set", "lang", "es")
	if got := strings.TrimSpace(e.must(e.root, "config", "get", "lang")); got != "es" {
		t.Fatalf("config get lang = %q", got)
	}
	e.must(e.root, "ai", "set", "claude", "sonnet")
	if got := strings.TrimSpace(e.must(e.root, "config", "get", "ai_model")); got != "sonnet" {
		t.Fatalf("ai model = %q", got)
	}
}

func TestMigratesBashWorkspace(t *testing.T) {
	e := newEnv(t)
	e.setup()
	// What the bash tooling left behind.
	for _, f := range []string{"lib/common.sh", "bin/infra-up", "bin/ipalpha-procs", "mprocs.yaml", "projects.json"} {
		write(t, filepath.Join(e.root, ".ipalpha", f), "old")
	}
	appendFile(t, filepath.Join(e.root, ".ipalpha", "settings"), "runner=auto\n")
	e.must(e.root, "pull")
	for _, f := range []string{"lib", "bin/infra-up", "bin/ipalpha-procs", "mprocs.yaml", "projects.json"} {
		if _, err := os.Stat(filepath.Join(e.root, ".ipalpha", f)); err == nil {
			t.Errorf("legacy %s not removed", f)
		}
	}
	if strings.Contains(envfile.Read(filepath.Join(e.root, ".ipalpha", "settings"))["runner"], "auto") {
		t.Error("legacy runner= setting kept")
	}
}

func TestFeatureLifecycle(t *testing.T) {
	e := newEnv(t)
	e.setup()
	root := e.root
	for _, bad := range []string{"ab", "Bad_Slug", "-lead", "trail-", strings.Repeat("a", 31), "has.dot"} {
		if _, err := e.run(root, "feature", "new", bad); err == nil {
			t.Fatalf("accepted bad slug %q", bad)
		}
	}
	if _, err := os.Stat(filepath.Join(root, "features")); err == nil {
		t.Fatal("bad slug created a folder")
	}

	out := e.must(root, "feature", "new", "hello-test")
	froot := filepath.Join(root, "features", "hello-test")
	for _, r := range catalog.Repos {
		dir := r.Path(froot)
		if st, err := os.Stat(filepath.Join(dir, ".git")); err != nil || st.IsDir() {
			t.Fatalf("%s is not a worktree", r.Name)
		}
		if b := git(t, dir, "symbolic-ref", "--short", "HEAD"); b != "feat/hello-test" {
			t.Fatalf("%s on %s", r.Name, b)
		}
	}
	authBase := git(t, filepath.Join(e.origins, "auth-api.git"), "rev-parse", "master~1")
	if git(t, filepath.Join(froot, "core", "auth-api"), "rev-parse", "HEAD") != authBase {
		t.Fatal("auth-api not pinned to the baseline")
	}
	if git(t, filepath.Join(froot, "core", "shared-js"), "rev-parse", "HEAD") != git(t, filepath.Join(root, "core", "shared-js"), "rev-parse", "v1.0.0") {
		t.Fatal("shared-js not pinned to its tag")
	}
	if git(t, filepath.Join(froot, "apps", "acampa-kids", "backend"), "rev-parse", "HEAD") != git(t, filepath.Join(root, "apps", "acampa-kids", "backend"), "rev-parse", "v0.19.0") {
		t.Fatal("acampa backend not pinned to its production tag")
	}
	if !strings.Contains(out, "acampa-kids-frontend: no v0.27.0 tag") || !strings.Contains(out, "https://acampa-ipalpha-hello-test.kevyn.com.br") {
		t.Fatalf("feature new output:\n%s", out)
	}
	fs := envfile.Read(filepath.Join(froot, ".ipalpha", "settings"))
	if fs["infra_name"] != "ipalpha-hello-test" || fs["mongo_port"] != "27117" || fs["auth-api_port"] != "3105" || fs["oikos-webapp_port"] != "5210" {
		t.Fatalf("feature settings: %v", fs)
	}
	if envfile.Read(filepath.Join(froot, ".ipalpha", "feature.env"))["slug"] != "hello-test" {
		t.Fatal("feature.env")
	}
	for _, dir := range []string{filepath.Join(froot, "core", "persons-api"), filepath.Join(froot, "core", "ai-api")} {
		if _, err := os.Stat(filepath.Join(dir, ".env")); err != nil {
			t.Fatalf("%s has no .env", dir)
		}
		if st := git(t, dir, "status", "--porcelain"); st != "" {
			t.Fatalf("worktree .env not excluded in %s: %s", dir, st)
		}
	}
	if st := git(t, filepath.Join(root, "core", "auth-api"), "status", "--porcelain"); st != "" {
		t.Fatal("main checkout touched")
	}
	var rec map[string]any
	data, _ := os.ReadFile(filepath.Join(froot, ".ipalpha", "release.json"))
	json.Unmarshal(data, &rec)
	if rec["namespace"] != "ipalpha-feat-hello-test" || rec["generation"].(float64) != 0 || rec["expiresAt"] != nil {
		t.Fatalf("draft: %s", data)
	}

	// Second feature: next port block; duplicate refused.
	e.must(root, "feature", "new", "second-one")
	if envfile.Read(filepath.Join(root, "features", "second-one", ".ipalpha", "settings"))["mongo_port"] != "27217" {
		t.Fatal("second feature port block")
	}
	if _, err := e.run(root, "feature", "new", "second-one"); err == nil {
		t.Fatal("duplicate feature accepted")
	}

	// A failed new rolls back.
	git(t, filepath.Join(root, "core", "oikos-webapp"), "checkout", "-q", "-b", "feat/rollback-me")
	if _, err := e.run(root, "feature", "new", "rollback-me"); err == nil {
		t.Fatal("new succeeded with the branch checked out elsewhere")
	}
	if _, err := os.Stat(filepath.Join(root, "features", "rollback-me")); err == nil {
		t.Fatal("rollback left the folder")
	}
	if gitOK(filepath.Join(root, "core", "auth-api"), "rev-parse", "-q", "--verify", "refs/heads/feat/rollback-me") {
		t.Fatal("rollback left a created branch")
	}
	git(t, filepath.Join(root, "core", "oikos-webapp"), "checkout", "-q", "master")

	// publish --feature (inside the feature folder, no AI, no terminal).
	appendFile(t, filepath.Join(froot, "apps", "forms", "forms-webapp", "README.md"), "change\n")
	out = e.must(froot, "publish", "--dry-run")
	if st := git(t, filepath.Join(froot, "apps", "forms", "forms-webapp"), "status", "--porcelain"); st == "" {
		t.Fatal("dry-run committed")
	}
	if _, err := e.run(froot, "publish", "--feature", "hello-test", "--folder", "forms-webapp"); err == nil {
		t.Fatal("--feature accepted release flags")
	}
	out = e.must(froot, "publish", "--no-wait")
	if !strings.Contains(out, "Changed services: forms-webapp") {
		t.Fatalf("publish output:\n%s", out)
	}
	if !gitOK(filepath.Join(e.origins, "forms-webapp.git"), "rev-parse", "-q", "--verify", "refs/heads/feat/hello-test") {
		t.Fatal("feat/hello-test not pushed")
	}
	if gitOK(filepath.Join(e.origins, "auth-api.git"), "rev-parse", "-q", "--verify", "refs/heads/feat/hello-test") {
		t.Fatal("unchanged auth-api pushed")
	}
	raw := git(t, filepath.Join(e.origins, "deployment.git"), "show", "master:previews/hello-test/release.json")
	json.Unmarshal([]byte(raw), &rec)
	images := rec["images"].(map[string]any)
	if rec["generation"].(float64) != 1 || rec["action"] != "publish" || images["forms-webapp"] == nil {
		t.Fatalf("record: %s", raw)
	}

	// shared-js closure: every core API, not forms-api (an app outside core).
	appendFile(t, filepath.Join(froot, "core", "shared-js", "README.md"), "x\n")
	e.must(froot, "publish", "--no-wait")
	raw = git(t, filepath.Join(e.origins, "deployment.git"), "show", "master:previews/hello-test/release.json")
	json.Unmarshal([]byte(raw), &rec)
	images = rec["images"].(map[string]any)
	for _, s := range []string{"auth-api", "persons-api", "places-api", "dispatch-api", "ai-api", "developers-api"} {
		if images[s] == nil {
			t.Errorf("closure misses %s", s)
		}
	}
	if images["forms-api"] != nil || images["auth-webapp"] != nil {
		t.Errorf("closure too wide: %v", images)
	}

	// Teammate pushes are never overwritten.
	f2 := filepath.Join(root, "features", "second-one")
	appendFile(t, filepath.Join(f2, "apps", "forms", "forms-webapp", "README.md"), "a\n")
	e.must(f2, "publish", "--no-wait")
	mate := filepath.Join(e.tmp, "mate")
	git(t, e.tmp, "clone", "-q", "-b", "feat/second-one", filepath.Join(e.origins, "forms-webapp.git"), mate)
	appendFile(t, filepath.Join(mate, "README.md"), "mate\n")
	git(t, mate, "commit", "-qam", "teammate work")
	git(t, mate, "push", "-q", "origin", "feat/second-one")
	git(t, filepath.Join(f2, "apps", "forms", "forms-webapp"), "fetch", "-q", "origin")
	appendFile(t, filepath.Join(f2, "apps", "forms", "forms-webapp", "README.md"), "b\n")
	if _, err := e.run(f2, "publish", "--no-wait"); err == nil {
		t.Fatal("publish overwrote a teammate push")
	}
	if git(t, filepath.Join(e.origins, "forms-webapp.git"), "log", "-1", "--format=%s", "feat/second-one") != "teammate work" {
		t.Fatal("teammate commit lost")
	}

	// list + extend keep CI fields.
	if out := e.must(root, "feature", "list"); !strings.Contains(out, "hello-test") {
		t.Fatalf("list:\n%s", out)
	}
	e.must(root, "feature", "extend", "hello-test", "--no-wait")

	// destroy: refuses local changes and needs confirmation; keeps branches.
	appendFile(t, filepath.Join(froot, "apps", "forms", "forms-webapp", "README.md"), "dirty\n")
	if _, err := e.run(root, "feature", "destroy", "hello-test", "--yes", "--no-wait"); err == nil {
		t.Fatal("destroy ignored local changes")
	}
	git(t, filepath.Join(froot, "apps", "forms", "forms-webapp"), "checkout", "-q", "--", "README.md")
	if _, err := e.run(root, "feature", "destroy", "hello-test", "--no-wait"); err == nil {
		t.Fatal("destroy without confirmation")
	}
	c := exec.Command(bin, "feature", "destroy", "hello-test", "--no-wait")
	c.Dir, c.Env, c.Stdin = root, e.vars, strings.NewReader("hello-test\n")
	if out, err := c.CombinedOutput(); err != nil {
		t.Fatalf("destroy: %v\n%s", err, out)
	}
	if _, err := os.Stat(froot); err == nil {
		t.Fatal("feature folder still there")
	}
	if !gitOK(filepath.Join(root, "apps", "forms", "forms-webapp"), "rev-parse", "-q", "--verify", "refs/heads/feat/hello-test") {
		t.Fatal("destroy deleted the branch")
	}

	// new reuses the kept branch with its commits.
	e.must(root, "feature", "new", "hello-test")
	if git(t, filepath.Join(froot, "apps", "forms", "forms-webapp"), "log", "-1", "--format=%s") == "init" {
		t.Fatal("reused branch lost its commits")
	}
}
