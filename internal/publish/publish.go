// Package publish is ./publish: pick the changed repositories, let the configured AI propose the
// semver bump and commit message (or type them when the AI is unavailable), then commit, tag
// libraries, push, publish libraries to npm, build images and bump deployment.
package publish

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/ipalpha-dev/tooling/internal/ai"
	"github.com/ipalpha-dev/tooling/internal/catalog"
	"github.com/ipalpha-dev/tooling/internal/envfile"
	"github.com/ipalpha-dev/tooling/internal/i18n"
	"github.com/ipalpha-dev/tooling/internal/repos"
	"github.com/ipalpha-dev/tooling/internal/sys"
	"github.com/ipalpha-dev/tooling/internal/ui"
	"github.com/ipalpha-dev/tooling/internal/workspace"
)

// Options of a release run.
type Options struct {
	DryRun  bool
	Folder  string
	CI      bool // TeamCity builds the images: skip local docker build/push
	Yes     bool
	AI      ai.Config
	Message string
}

// Plan for one repository.
type Plan struct {
	Repo    catalog.Repo
	Dir     string
	Current string
	Next    string
	Bump    string
	Message string
	Reason  string
	Source  string // ai | cache | manual
	Changed int
}

// CurrentVersion reads package.json version (else the last v* tag).
func CurrentVersion(dir string) string {
	if data, err := os.ReadFile(filepath.Join(dir, "package.json")); err == nil {
		var pkg struct {
			Version string `json:"version"`
		}
		if json.Unmarshal(data, &pkg) == nil && pkg.Version != "" {
			return pkg.Version
		}
		return "0.0.0"
	}
	if tag, err := sys.Git(dir, "describe", "--tags", "--abbrev=0"); err == nil {
		return strings.TrimPrefix(tag, "v")
	}
	return "0.0.0"
}

var semverRe = regexp.MustCompile(`^(\d+)\.(\d+)\.(\d+)`)

// Bump a version.
func Bump(v, kind string) string {
	m := semverRe.FindStringSubmatch(v)
	if m == nil {
		m = []string{"", "0", "0", "0"}
	}
	a, _ := strconv.Atoi(m[1])
	b, _ := strconv.Atoi(m[2])
	c, _ := strconv.Atoi(m[3])
	switch kind {
	case "major":
		return fmt.Sprintf("%d.0.0", a+1)
	case "minor":
		return fmt.Sprintf("%d.%d.0", a, b+1)
	case "patch":
		return fmt.Sprintf("%d.%d.%d", a, b, c+1)
	}
	return v
}

func lastTag(dir string) string {
	t, err := sys.Git(dir, "describe", "--tags", "--abbrev=0", "--match", "v*")
	if err != nil {
		return ""
	}
	return t
}

// Dirty: uncommitted changes, or commits since the last v* tag whose version was not bumped yet
// (a bumped-but-not-deployed repo is already marked; nothing more to publish).
func Dirty(dir string) bool {
	if !repos.IsGit(dir) {
		return false
	}
	if st, _ := sys.Git(dir, "status", "--porcelain"); st != "" {
		return true
	}
	tag := lastTag(dir)
	if tag == "" {
		return true
	}
	if log, _ := sys.Git(dir, "log", "--oneline", tag+"..HEAD"); log == "" {
		return false
	}
	return CurrentVersion(dir) == strings.TrimPrefix(tag, "v")
}

// DirtyRepos lists publishable repositories with changes.
func DirtyRepos(w *workspace.Workspace) []catalog.Repo {
	var out []catalog.Repo
	for _, r := range catalog.Repos {
		if r.Publishable() && Dirty(r.Path(w.Root)) {
			out = append(out, r)
		}
	}
	return out
}

func changedFiles(dir string) []string {
	var files []string
	st, _ := sys.Git(dir, "status", "--porcelain")
	for _, l := range strings.Split(st, "\n") {
		if len(l) > 3 {
			f := l[3:]
			if i := strings.Index(f, " -> "); i >= 0 {
				f = f[i+4:]
			}
			files = append(files, strings.Trim(f, `"`))
		}
	}
	if len(files) == 0 {
		if tag := lastTag(dir); tag != "" {
			out, _ := sys.Git(dir, "diff", "--name-only", tag+"..HEAD")
			for _, f := range strings.Split(out, "\n") {
				if f != "" {
					files = append(files, f)
				}
			}
		}
	}
	return files
}

// Clamp: docs-only changes never bump the version.
func Clamp(files []string, bump string) string {
	if len(files) == 0 {
		return bump
	}
	for _, f := range files {
		low := strings.ToLower(f)
		if strings.HasSuffix(low, ".md") || strings.HasPrefix(f, "docs/") || strings.HasSuffix(low, ".txt") || strings.HasPrefix(f, "LICENSE") {
			continue
		}
		return bump
	}
	return "none"
}

func context(dir string, r catalog.Repo) string {
	st, _ := sys.Git(dir, "status", "--porcelain")
	stat, _ := sys.Git(dir, "diff", "--stat", "HEAD")
	var since string
	if tag := lastTag(dir); tag != "" {
		since, _ = sys.Git(dir, "diff", "--stat", tag+"..HEAD")
	}
	log, _ := sys.Git(dir, "log", "--oneline", "-8")
	diff, _ := sys.Git(dir, "diff", "HEAD", "--unified=1", "--", ".", ":(exclude)package-lock.json")
	if len(diff) > 12000 {
		diff = diff[:12000] + "\n… (diff truncated)"
	}
	return fmt.Sprintf("Repository: %s (current version %s)\n\ngit status:\n%s\n\ngit diff --stat (working tree):\n%s\n\nchanges since last release tag:\n%s\n\nrecent commits:\n%s\n\ndiff excerpt:\n%s",
		r.Name, CurrentVersion(dir), st, tailLines(stat, 20), tailLines(since, 20), log, diff)
}

func tailLines(s string, n int) string { return strings.Join(ui.Tail(s, n), "\n") }

const systemPrompt = `You are a release assistant for a TypeScript/NestJS + React monorepo. Given a repository's git status, diff stat, diff excerpt and recent commits, pick the semantic version bump (none for docs-only, patch for fixes, minor for new backwards-compatible features, major for breaking API changes) and write a one-line commit message (English, conventional-commit style, imperative mood, at most 72 characters).
Respond with JSON only: {"reason": "<one sentence>", "bump": "none|patch|minor|major", "message": "<commit message>"}`

func cacheDir(w *workspace.Workspace) string { return filepath.Join(w.StateDir(), "publish-cache") }

// Decide asks the AI (cached by the exact context) and returns the decision.
func Decide(w *workspace.Workspace, r catalog.Repo, dir string, cfg ai.Config) (ai.Decision, string, error) {
	ctx := context(dir, r)
	sum := sha256.Sum256([]byte(cfg.Label() + "\x00" + ctx))
	key := hex.EncodeToString(sum[:])[:24]
	path := filepath.Join(cacheDir(w), r.Name+"-"+key+".json")
	if data, err := os.ReadFile(path); err == nil {
		var d ai.Decision
		if json.Unmarshal(data, &d) == nil && d.Bump != "" {
			return d, "cache", nil
		}
	}
	reply, err := ai.Ask(cfg, systemPrompt+"\n\n"+ctx)
	if err != nil {
		return ai.Decision{}, "", err
	}
	d, err := ai.ParseDecision(reply)
	if err != nil {
		return ai.Decision{}, "", &ai.Error{Engine: cfg.CLI, Detail: err.Error() + ": " + firstN(reply, 200), Hint: i18n.T("ai_hint_model")}
	}
	data, _ := json.Marshal(d)
	_ = os.MkdirAll(cacheDir(w), 0o755)
	_ = os.WriteFile(path, data, 0o644)
	return d, "ai", nil
}

func firstN(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > n {
		return s[:n] + "…"
	}
	return s
}

// ClearCache removes cached decisions.
func ClearCache(w *workspace.Workspace) error { return os.RemoveAll(cacheDir(w)) }

// Manual asks the developer for the bump and message (AI unavailable).
func Manual(r catalog.Repo, current string, files []string) (ai.Decision, error) {
	def := "patch"
	if Clamp(files, "patch") == "none" {
		def = "none"
	}
	bump, err := ui.Select(i18n.T("publish_manual_bump", r.Name), i18n.T("publish_manual_bump_desc", current), []ui.Option{
		{Value: "patch", Label: "patch", Hint: Bump(current, "patch") + " · " + i18n.T("publish_bump_patch")},
		{Value: "minor", Label: "minor", Hint: Bump(current, "minor") + " · " + i18n.T("publish_bump_minor")},
		{Value: "major", Label: "major", Hint: Bump(current, "major") + " · " + i18n.T("publish_bump_major")},
		{Value: "none", Label: "none", Hint: i18n.T("publish_bump_none")},
	}, def)
	if err != nil {
		return ai.Decision{}, err
	}
	msg, err := ui.Input(i18n.T("publish_manual_message", r.Name), i18n.T("publish_manual_message_desc"), "", func(s string) error {
		if strings.TrimSpace(s) == "" {
			return errors.New(i18n.T("publish_message_required"))
		}
		return nil
	})
	if err != nil {
		return ai.Decision{}, err
	}
	return ai.Decision{Reason: i18n.T("publish_manual_reason"), Bump: bump, Message: msg}, nil
}

// Preflight fetches every repo the run touches and fast-forwards when applying. Diverged history fails.
func Preflight(dirs []string, apply bool) error {
	var problems []string
	for _, dir := range dirs {
		if !repos.IsGit(dir) {
			continue
		}
		up, err := sys.Git(dir, "rev-parse", "--abbrev-ref", "--symbolic-full-name", "@{u}")
		if err != nil || up == "" {
			continue
		}
		ui.Info(i18n.T("publish_fetching", filepath.Base(dir)))
		if res := (sys.Cmd{Name: "git", Args: []string{"-C", dir, "fetch", "--quiet"}, Env: []string{"GIT_TERMINAL_PROMPT=0"}}).Run(); res.Err != nil {
			problems = append(problems, filepath.Base(dir)+": "+i18n.T("repos_fetch_failed")+" ("+strings.Join(ui.Tail(res.Output, 1), "")+")")
			continue
		}
		counts, _ := sys.Git(dir, "rev-list", "--left-right", "--count", "HEAD..."+up)
		f := strings.Fields(counts)
		if len(f) != 2 {
			continue
		}
		ahead, _ := strconv.Atoi(f[0])
		behind, _ := strconv.Atoi(f[1])
		if behind == 0 {
			continue
		}
		if ahead > 0 {
			problems = append(problems, i18n.T("publish_diverged", filepath.Base(dir), ahead, behind))
			continue
		}
		if !apply {
			continue
		}
		if st, _ := sys.Git(dir, "status", "--porcelain", "--untracked-files=no"); st != "" {
			if res := (sys.Cmd{Name: "git", Args: []string{"-C", dir, "pull", "--ff-only", "--autostash", "--quiet"}}).Run(); res.Err != nil {
				problems = append(problems, filepath.Base(dir)+": "+strings.Join(ui.Tail(res.Output, 2), " "))
			}
			continue
		}
		if res := (sys.Cmd{Name: "git", Args: []string{"-C", dir, "merge", "--ff-only", "--quiet", up}}).Run(); res.Err != nil {
			problems = append(problems, filepath.Base(dir)+": "+strings.Join(ui.Tail(res.Output, 2), " "))
		}
	}
	if len(problems) > 0 {
		return &ui.Problem{Step: i18n.T("publish_preflight_step"), Cause: i18n.T("publish_preflight_failed"), Tail: problems,
			Fix: []string{i18n.T("publish_fix_diverged")}}
	}
	return nil
}

// SetVersion writes package.json and package-lock.json versions, preserving formatting.
func SetVersion(dir, version string) error {
	re := regexp.MustCompile(`("version"\s*:\s*")[^"]*(")`)
	pj := filepath.Join(dir, "package.json")
	data, err := os.ReadFile(pj)
	if err != nil {
		return nil
	}
	loc := re.FindIndex(data)
	if loc == nil {
		return fmt.Errorf("package.json has no version field")
	}
	out := append([]byte{}, data[:loc[0]]...)
	out = append(out, re.ReplaceAll(data[loc[0]:loc[1]], []byte("${1}"+version+"${2}"))...)
	out = append(out, data[loc[1]:]...)
	if err := os.WriteFile(pj, out, 0o644); err != nil {
		return err
	}
	lock := filepath.Join(dir, "package-lock.json")
	ldata, err := os.ReadFile(lock)
	if err != nil {
		return nil
	}
	return os.WriteFile(lock, SetLockVersion(ldata, version), 0o644)
}

// SetLockVersion changes only the root "version" and packages[""].version of a package-lock.json,
// keeping its exact formatting (dependency versions are never touched).
func SetLockVersion(data []byte, version string) []byte {
	re := regexp.MustCompile(`("version"\s*:\s*")[^"]*(")`)
	text := string(data)
	pkgs := strings.Index(text, `"packages"`)
	if pkgs < 0 {
		pkgs = len(text)
	}
	if loc := re.FindStringIndex(text[:pkgs]); loc != nil {
		text = text[:loc[0]] + re.ReplaceAllString(text[loc[0]:loc[1]], "${1}"+version+"${2}") + text[loc[1]:]
	}
	pkgs = strings.Index(text, `"packages"`)
	if pkgs < 0 {
		return []byte(text)
	}
	root := regexp.MustCompile(`""\s*:\s*\{`).FindStringIndex(text[pkgs:])
	if root == nil {
		return []byte(text)
	}
	start := pkgs + root[1]
	end := strings.Index(text[start:], "}")
	if end < 0 {
		return []byte(text)
	}
	block := text[start : start+end]
	if loc := re.FindStringIndex(block); loc != nil {
		block = block[:loc[0]] + re.ReplaceAllString(block[loc[0]:loc[1]], "${1}"+version+"${2}") + block[loc[1]:]
	}
	return []byte(text[:start] + block + text[start+end:])
}

func npmLoggedIn(dir string) bool {
	_, _, err := sys.RunTimeout(30*time.Second, dir, "", sys.Npm(), "whoami")
	return err == nil
}

func git(dir string, args ...string) error {
	res := sys.Cmd{Name: "git", Args: append([]string{"-C", dir}, args...), Log: ui.LogWriter()}.Run()
	if res.Err != nil {
		return &ui.Problem{Step: "git " + strings.Join(args, " ") + " (" + filepath.Base(dir) + ")", Cause: strings.Join(ui.Tail(res.Output, 1), ""), Tail: ui.Tail(res.Output, 8)}
	}
	return nil
}

// Apply commits, tags, pushes and ships one repository.
func Apply(w *workspace.Workspace, p Plan, ciMode bool) error {
	dir := p.Dir
	if p.Repo.Kind == catalog.KindLibrary && p.Bump != "none" && !npmLoggedIn(dir) {
		return ui.NewProblem(i18n.T("publish_npm_step", p.Repo.Name), i18n.T("publish_npm_login"), "npm login")
	}
	if p.Bump != "none" {
		if err := SetVersion(dir, p.Next); err != nil {
			return ui.Wrap(i18n.T("publish_version_step", p.Repo.Name), err)
		}
	}
	if err := git(dir, "add", "-A"); err != nil {
		return err
	}
	if staged, _ := sys.Git(dir, "diff", "--cached", "--name-only"); staged != "" {
		for _, f := range strings.Split(staged, "\n") {
			if filepath.Base(f) == ".env" {
				_ = git(dir, "reset", "-q", "--", f)
				return ui.NewProblem(i18n.T("publish_commit_step", p.Repo.Name), i18n.T("publish_env_staged", f), i18n.T("publish_fix_gitignore"))
			}
		}
		if err := git(dir, "commit", "-q", "-m", p.Message); err != nil {
			return err
		}
	}
	if p.Bump != "none" && p.Repo.TagsLocally() {
		if err := git(dir, "tag", "v"+p.Next); err != nil {
			return err
		}
	}
	if err := git(dir, "push", "--set-upstream", "origin", "HEAD"); err != nil {
		return err
	}
	if p.Bump != "none" && p.Repo.TagsLocally() {
		if err := git(dir, "push", "origin", "v"+p.Next); err != nil {
			return err
		}
	}
	if p.Bump == "none" {
		return nil
	}
	switch {
	case p.Repo.Kind == catalog.KindLibrary:
		return NpmPublish(p.Repo.Name, dir)
	}
	if !ciMode {
		if err := BuildImage(w, p.Repo, p.Next, dir); err != nil {
			return err
		}
	}
	return BumpDeployment(w, p.Repo.Name, p.Next)
}

// NpmPublish publishes a library (dist rebuilt by prepublishOnly; deleted modules never survive).
func NpmPublish(name, dir string) error {
	if !npmLoggedIn(dir) {
		return ui.NewProblem(i18n.T("publish_npm_step", name), i18n.T("publish_npm_login"), "npm login")
	}
	os.RemoveAll(filepath.Join(dir, "dist"))
	res := sys.Cmd{Dir: dir, Env: sys.NodeEnv(), Name: sys.Npm(), Args: []string{"publish"}, Log: ui.LogWriter()}.Run()
	if res.Err != nil {
		return &ui.Problem{Step: i18n.T("publish_npm_step", name), Cause: strings.Join(ui.Tail(res.Output, 1), ""), Tail: ui.Tail(res.Output, 12)}
	}
	return nil
}

// BuildImage builds and pushes <registry>/<repo>:<version> with Docker or Apple container.
func BuildImage(w *workspace.Workspace, r catalog.Repo, version, dir string) error {
	if !r.HasImage() {
		return nil
	}
	if _, err := os.Stat(filepath.Join(dir, "Dockerfile")); err != nil {
		ui.Info(r.Name + ": " + i18n.T("publish_no_dockerfile"))
		return nil
	}
	image := w.Settings.Registry + "/" + r.Name + ":" + version
	registryHost := strings.SplitN(w.Settings.Registry, "/", 2)[0]
	var build, push []string
	switch {
	case sys.Has("docker"):
		build = []string{"docker", "build", "-t", image, dir}
		push = []string{"docker", "push", image}
	case sys.Has("container"):
		build = []string{"container", "build", "--tag", image, dir}
		push = []string{"container", "image", "push", image}
	default:
		return ui.NewProblem(i18n.T("publish_image_step", image), i18n.T("infra_no_runtime"), i18n.T("infra_fix_install_docker"))
	}
	ui.Info(i18n.T("publish_building", image))
	if res := (sys.Cmd{Name: build[0], Args: build[1:], Log: ui.LogWriter()}).Run(); res.Err != nil {
		return &ui.Problem{Step: i18n.T("publish_image_step", image), Cause: strings.Join(ui.Tail(res.Output, 1), ""), Tail: ui.Tail(res.Output, 15)}
	}
	if res := (sys.Cmd{Name: push[0], Args: push[1:], Log: ui.LogWriter()}).Run(); res.Err != nil {
		return &ui.Problem{Step: i18n.T("publish_push_step", image), Cause: strings.Join(ui.Tail(res.Output, 1), ""), Tail: ui.Tail(res.Output, 8),
			Fix: []string{push[0] + " login " + registryHost}}
	}
	return nil
}

// BumpDeployment rewrites <registry>/<repo>:<tag> in deployment YAML, commits and pushes.
func BumpDeployment(w *workspace.Workspace, repo, version string) error {
	dep := w.Repo("deployment")
	if !repos.IsGit(dep) {
		ui.Warning(i18n.T("publish_deployment_missing"))
		return nil
	}
	prefix := w.Settings.Registry + "/" + repo + ":"
	re := regexp.MustCompile(regexp.QuoteMeta(prefix) + `[^\s"']*`)
	var changed []string
	_ = filepath.WalkDir(dep, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if d.Name() == ".git" || d.Name() == "node_modules" {
				return filepath.SkipDir
			}
			return nil
		}
		if ext := filepath.Ext(p); ext != ".yaml" && ext != ".yml" {
			return nil
		}
		data, err := os.ReadFile(p)
		if err != nil || !strings.Contains(string(data), prefix) {
			return nil
		}
		out := re.ReplaceAll(data, []byte(prefix+version))
		if string(out) != string(data) {
			if os.WriteFile(p, out, 0o644) == nil {
				rel, _ := filepath.Rel(dep, p)
				changed = append(changed, filepath.ToSlash(rel))
			}
		}
		return nil
	})
	if len(changed) == 0 {
		return nil
	}
	if err := git(dep, append([]string{"add", "--"}, changed...)...); err != nil {
		return err
	}
	if sys.GitOK(dep, "diff", "--cached", "--quiet") {
		return nil
	}
	if err := git(dep, "commit", "-q", "-m", "bump "+repo+" to "+version); err != nil {
		return err
	}
	return git(dep, "push")
}

// LoadAI reads the AI configuration of a workspace (flags override).
func LoadAI(w *workspace.Workspace, engine, model string) ai.Config {
	cfg := ai.Config{CLI: w.Settings.AICLI, Model: w.Settings.AIModel, Command: w.Settings.AICommand}
	if engine != "" {
		cfg.CLI = engine
		if engine != w.Settings.AICLI {
			cfg.Model = ""
		}
	}
	if model != "" {
		cfg.Model = model
	}
	return cfg
}

// ReadFeatureSlug returns the slug of a feature workspace (empty for the main one).
func ReadFeatureSlug(w *workspace.Workspace) string {
	return envfile.Read(filepath.Join(w.Dir(), "feature.env"))["slug"]
}
