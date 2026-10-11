// Package feature is ./feature: one isolated workspace per feature (git worktrees on feat/<slug>
// pinned to the last green Core Deploy) and its preview namespace. The cluster only reacts to
// previews/<slug>/release.json pushed to deployment master; this package never writes to
// Kubernetes and never holds a CI token. Design: deployment/docs/feature-environments.md.
package feature

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ipalpha-dev/tooling/internal/catalog"
	"github.com/ipalpha-dev/tooling/internal/env"
	"github.com/ipalpha-dev/tooling/internal/envfile"
	"github.com/ipalpha-dev/tooling/internal/i18n"
	"github.com/ipalpha-dev/tooling/internal/repos"
	"github.com/ipalpha-dev/tooling/internal/sys"
	"github.com/ipalpha-dev/tooling/internal/ui"
	"github.com/ipalpha-dev/tooling/internal/workspace"
)

// Domain of preview hosts.
func Domain() string {
	if d := os.Getenv("IPALPHA_PREVIEW_DOMAIN"); d != "" {
		return d
	}
	return "kevyn.com.br"
}

// WaitMinutes for the Preview pipeline.
func WaitMinutes() int {
	if v, err := strconv.Atoi(os.Getenv("IPALPHA_FEATURE_WAIT_MINUTES")); err == nil && v > 0 {
		return v
	}
	return 60
}

// Hosts of a preview (spec: Oikos, Forms, auth).
func Hosts(slug string) []string {
	d := Domain()
	return []string{"ipalpha-" + slug + "." + d, "forms-ipalpha-" + slug + "." + d, "auth-ipalpha-" + slug + "." + d}
}

// DevelopersHost is served when the baseline includes developers-webapp.
func DevelopersHost(slug string) string { return "developers-ipalpha-" + slug + "." + Domain() }

// AppHosts are served only when the app changed.
func AppHosts(slug string) []string { return []string{"acampa-ipalpha-" + slug + "." + Domain()} }

var slugRe = regexp.MustCompile(`^[a-z0-9-]{3,30}$`)

// ValidateSlug enforces the DNS rules of preview hosts.
func ValidateSlug(slug string) error {
	if !slugRe.MatchString(slug) || strings.HasPrefix(slug, "-") || strings.HasSuffix(slug, "-") {
		return ui.NewProblem(i18n.T("feature_step_slug"), i18n.T("feature_bad_slug", slug))
	}
	for _, h := range append(append(Hosts(slug), DevelopersHost(slug)), AppHosts(slug)...) {
		if len(h) > 63 {
			return ui.NewProblem(i18n.T("feature_step_slug"), i18n.T("feature_host_too_long", h))
		}
	}
	return nil
}

// Dir of a feature workspace.
func Dir(main, slug string) string { return filepath.Join(main, "features", slug) }

// Baseline is releases/core-latest.json.
type Baseline struct {
	Release    string                     `json:"release"`
	Services   map[string]json.RawMessage `json:"services"`
	Libraries  map[string]json.RawMessage `json:"libraries"`
	Deployment struct {
		Commit string `json:"commit"`
	} `json:"deployment"`
	raw []byte
}

type pin struct {
	SourceCommit string `json:"sourceCommit"`
	Commit       string `json:"commit"`
}

func (b *Baseline) commitOf(repo string) string {
	if repo == "deployment" {
		return b.Deployment.Commit
	}
	for _, m := range []map[string]json.RawMessage{b.Services, b.Libraries} {
		if raw, ok := m[repo]; ok {
			var p pin
			_ = json.Unmarshal(raw, &p)
			if p.SourceCommit != "" {
				return p.SourceCommit
			}
			return p.Commit
		}
	}
	return ""
}

func (b *Baseline) has(repo string) bool {
	if repo == "deployment" || repo == "shared-js" {
		return true
	}
	_, s := b.Services[repo]
	_, l := b.Libraries[repo]
	return s || l
}

// ReadBaseline fetches deployment master and reads the baseline at ref (default origin/master).
func ReadBaseline(main, ref string) (*Baseline, string, error) {
	dep := filepath.Join(main, "deployment")
	step := i18n.T("feature_step_baseline")
	if res := (sys.Cmd{Name: "git", Args: []string{"-C", dep, "fetch", "-q", "origin", "master"}, Env: []string{"GIT_TERMINAL_PROMPT=0"}}).Run(); res.Err != nil {
		return nil, "", &ui.Problem{Step: step, Cause: i18n.T("feature_fetch_deployment"), Tail: ui.Tail(res.Output, 4)}
	}
	if ref == "" {
		ref = "origin/master"
	}
	commit, err := sys.Git(dep, "rev-parse", "--verify", "-q", ref+"^{commit}")
	if err != nil || commit == "" {
		return nil, "", ui.NewProblem(step, i18n.T("feature_unknown_revision", ref))
	}
	if !sys.GitOK(dep, "merge-base", "--is-ancestor", commit, "origin/master") {
		return nil, "", ui.NewProblem(step, i18n.T("feature_baseline_not_master"))
	}
	raw, err := sys.Git(dep, "show", commit+":releases/core-latest.json")
	if err != nil {
		return nil, "", ui.NewProblem(step, i18n.T("feature_no_baseline"))
	}
	b := &Baseline{raw: []byte(raw)}
	if err := json.Unmarshal([]byte(raw), b); err != nil {
		return nil, "", ui.Wrap(step, err)
	}
	return b, commit, nil
}

// BaseCommit resolves the commit a repository's feature branch starts from.
func BaseCommit(main string, b *Baseline, r catalog.Repo) (string, string, error) {
	dir := r.Path(main)
	if r.Kind == catalog.KindOptional {
		// Apps with their own registry: the tag v<version> of the image their production manifest
		// names in the baseline's deployment commit; origin/master with a warning when missing.
		re := regexp.MustCompile(`ip-alpha/apps/` + regexp.QuoteMeta(r.App) + `/` + regexp.QuoteMeta(r.Image) + `:([0-9]+\.[0-9]+\.[0-9]+)`)
		out, _ := sys.Git(filepath.Join(main, "deployment"), "grep", "-h", "-o", "-E", re.String(), b.Deployment.Commit, "--", "base/apps")
		version := ""
		if m := re.FindStringSubmatch(out); m != nil {
			version = m[1]
		}
		if version != "" {
			if c, err := sys.Git(dir, "rev-parse", "--verify", "-q", "v"+version+"^{commit}"); err == nil && c != "" {
				return c, "", nil
			}
		}
		if version == "" {
			version = "?"
		}
		c, err := sys.Git(dir, "rev-parse", "--verify", "-q", "origin/master^{commit}")
		if err != nil || c == "" {
			return "", "", fmt.Errorf("%s: neither v%s nor origin/master found", r.Name, version)
		}
		return c, i18n.T("feature_app_no_tag", r.Name, version), nil
	}
	if r.Name == "shared-js" {
		version := ""
		for _, api := range catalog.APIs() {
			c := b.commitOf(api.Name)
			if c == "" {
				continue
			}
			lock, err := sys.Git(api.Path(main), "show", c+":package-lock.json")
			if err != nil {
				continue
			}
			var doc struct {
				Packages map[string]struct {
					Version string `json:"version"`
				} `json:"packages"`
			}
			if json.Unmarshal([]byte(lock), &doc) == nil {
				if v := doc.Packages["node_modules/@ipalpha/shared-js"].Version; v != "" {
					version = v
					break
				}
			}
		}
		if version == "" {
			return "", "", errors.New("cannot resolve the baseline shared-js version")
		}
		_, _ = sys.Git(dir, "fetch", "-q", "--tags", "origin")
		c, err := sys.Git(dir, "rev-parse", "--verify", "-q", "v"+version+"^{commit}")
		if err != nil || c == "" {
			return "", "", fmt.Errorf("shared-js tag v%s not found", version)
		}
		return c, "", nil
	}
	c := b.commitOf(r.Name)
	if c == "" {
		return "", "", fmt.Errorf("baseline has no commit for %s", r.Name)
	}
	return c, "", nil
}

// Record is previews/<slug>/release.json (schema approved in the spec, §1 decision 11). Unknown
// fields written by CI are kept.
type Record map[string]any

// LoadRecord reads a record file.
func LoadRecord(path string) (Record, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var r Record
	if err := json.Unmarshal(data, &r); err != nil {
		return nil, err
	}
	return r, nil
}

// Save writes a record with 2-space indentation and a trailing newline.
func (r Record) Save(path string) error {
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	return envfile.WriteAtomic(path, append(data, '\n'), 0o644)
}

// Repositories of the record.
func (r Record) Repositories() map[string]map[string]any {
	out := map[string]map[string]any{}
	if m, ok := r["repositories"].(map[string]any); ok {
		for k, v := range m {
			if e, ok := v.(map[string]any); ok {
				out[k] = e
			}
		}
	}
	return out
}

func str(v any) string { s, _ := v.(string); return s }

// Changed services: featureCommit ≠ baseCommit plus the dependency closure (shared-js → every core
// API). Apps outside core join only when one of their repos changed.
func (r Record) Changed() []string {
	repos := r.Repositories()
	changed := map[string]bool{}
	for k, e := range repos {
		if str(e["featureCommit"]) != str(e["baseCommit"]) {
			changed[k] = true
		}
	}
	for k := range repos {
		cr, ok := catalog.Get(k)
		core := ok && cr.App == ""
		if !core {
			continue
		}
		if changed["shared-js"] && strings.HasSuffix(k, "-api") {
			changed[k] = true
		}
	}
	var out []string
	for k := range changed {
		if k != "shared-js" && k != "deployment" {
			out = append(out, k)
		}
	}
	sort.Strings(out)
	return out
}

func now() string { return time.Now().UTC().Format("2006-01-02T15:04:05Z") }

// Open returns the main workspace for any workspace (feature or main).
func mainOf(w *workspace.Workspace) string { return w.Main() }

// New creates features/<slug>.
func New(w *workspace.Workspace, slug, baselineRef string, self string, materialize func(*workspace.Workspace) error) (string, error) {
	main := mainOf(w)
	if err := ValidateSlug(slug); err != nil {
		return "", err
	}
	froot := Dir(main, slug)
	if _, err := os.Stat(froot); err == nil {
		return "", ui.NewProblem(i18n.T("feature_step_new", slug), i18n.T("feature_exists", froot))
	}
	mw, err := workspace.Open(main)
	if err != nil {
		return "", err
	}
	var created [][2]string
	rollback := func() {
		for _, r := range catalog.Repos {
			dir := r.Path(froot)
			if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
				_, _ = sys.Git(r.Path(main), "worktree", "remove", "--force", dir)
			}
		}
		for _, c := range created {
			_, _ = sys.Git(c[0], "branch", "-q", "-D", c[1])
		}
		os.RemoveAll(froot)
	}
	b, bcommit, err := ReadBaseline(main, baselineRef)
	if err != nil {
		return "", err
	}
	ui.Section(i18n.T("feature_creating", slug, b.Release))
	fetchAll(main)
	branch := "feat/" + slug
	type repoLine struct{ name, base, head string }
	var lines []repoLine
	pushed := map[string]string{}
	for _, r := range catalog.Repos {
		src := r.Path(main)
		if r.Kind == catalog.KindOptional && !repos.IsGit(src) {
			ui.Info(i18n.T("feature_optional_skipped", r.Name))
			continue
		}
		if r.Kind != catalog.KindOptional && !b.has(r.Name) {
			ui.Info(i18n.T("feature_not_in_baseline", r.Name))
			continue
		}
		if !repos.IsGit(src) {
			rollback()
			return "", ui.NewProblem(i18n.T("feature_step_new", slug), i18n.T("feature_missing_repo", r.Rel()), "./pull")
		}
		base, warn, err := BaseCommit(main, b, r)
		if err != nil {
			rollback()
			return "", ui.Wrap(i18n.T("feature_step_new", slug), err)
		}
		if warn != "" {
			ui.Warning(warn)
		}
		dest := r.Path(froot)
		had := sys.GitOK(src, "show-ref", "--verify", "--quiet", "refs/heads/"+branch)
		base, err = addWorktree(src, dest, branch, base)
		if err != nil {
			rollback()
			return "", ui.Wrap(i18n.T("feature_step_new", slug), err)
		}
		if !had {
			created = append(created, [2]string{src, branch})
		}
		excludeEnv(dest)
		if sys.GitOK(dest, "show-ref", "--verify", "--quiet", "refs/remotes/origin/"+branch) {
			sha, _ := sys.Git(dest, "rev-parse", "refs/remotes/origin/"+branch)
			pushed[r.Name] = sha
		}
		head, _ := sys.Git(dest, "rev-parse", "HEAD")
		ui.Item(r.Name, branch+" @ "+short(base))
		lines = append(lines, repoLine{r.Name, base, head})
	}

	// Settings: own infra name and a +100·k port block no other workspace recorded.
	fs := *mw.Settings
	fs.Ports = map[string]int{}
	fs.InfraName = "ipalpha-" + slug
	offset, err := portOffset(main)
	if err != nil {
		rollback()
		return "", err
	}
	for _, k := range catalog.PortKeys() {
		fs.Ports[k] = mw.Settings.Port(k) + offset
	}
	fw := &workspace.Workspace{Root: froot, Settings: &fs, FeatureSlug: slug, MainRoot: main}
	if err := os.MkdirAll(fw.Dir(), 0o755); err != nil {
		rollback()
		return "", err
	}
	if err := envfile.WriteAtomic(filepath.Join(fw.Dir(), "feature.env"), []byte("slug="+slug+"\nmain_root="+main+"\n"), 0o644); err != nil {
		rollback()
		return "", err
	}
	if data, err := os.ReadFile(filepath.Join(mw.Dir(), ".env")); err == nil {
		_ = envfile.WriteAtomic(filepath.Join(fw.Dir(), ".env"), data, 0o600)
	}
	// Local environment: the main workspace's .env files (ports stay defaults: ./run maps them).
	for _, r := range catalog.APIs() {
		dest := r.Path(froot)
		if _, err := os.Stat(dest); err != nil {
			continue
		}
		if data, err := os.ReadFile(filepath.Join(r.Path(main), ".env")); err == nil {
			_ = envfile.WriteAtomic(filepath.Join(dest, ".env"), data, 0o600)
		}
	}
	if _, err := env.Prepare(fw); err != nil {
		rollback()
		return "", err
	}
	if err := materialize(fw); err != nil {
		rollback()
		return "", err
	}
	if len(pushed) > 0 {
		var b strings.Builder
		for k, v := range pushed {
			fmt.Fprintf(&b, "%s %s\n", k, v)
		}
		_ = os.WriteFile(filepath.Join(fw.Dir(), "pushed"), []byte(b.String()), 0o644)
	}
	// Draft release record.
	owner, _ := sys.Output("", "git", "config", "user.email")
	if owner == "" {
		owner = "unknown"
	}
	reposMap := map[string]any{}
	for _, l := range lines {
		reposMap[l.name] = map[string]any{"url": repos.URL(mw.Settings.Org, l.name), "baseRef": "master", "baseCommit": l.base, "featureCommit": l.head}
	}
	var release any
	if b.Release != "" {
		release = b.Release
	}
	rec := Record{"schemaVersion": 1, "feature": slug, "namespace": "ipalpha-feat-" + slug, "owner": owner,
		"baselineRelease": map[string]any{"id": release, "deploymentCommit": bcommit},
		"repositories":    reposMap, "images": map[string]any{}, "seed": map[string]any{"version": 1, "digest": nil},
		"hosts": Hosts(slug), "generation": 0, "createdAt": now(), "expiresAt": nil, "teamcityBuild": nil}
	if err := rec.Save(filepath.Join(fw.Dir(), "release.json")); err != nil {
		rollback()
		return "", err
	}
	_ = envfile.WriteAtomic(filepath.Join(fw.Dir(), "baseline.json"), b.raw, 0o644)
	return froot, nil
}

func short(s string) string {
	if len(s) > 12 {
		return s[:12]
	}
	return s
}

func fetchAll(main string) {
	done := make(chan struct{})
	n := 0
	for _, r := range catalog.Repos {
		dir := r.Path(main)
		if !repos.IsGit(dir) {
			continue
		}
		n++
		go func(dir string) {
			_ = (sys.Cmd{Name: "git", Args: []string{"-C", dir, "fetch", "-q", "--tags", "origin"}, Env: []string{"GIT_TERMINAL_PROMPT=0"}}).Run()
			done <- struct{}{}
		}(dir)
	}
	for i := 0; i < n; i++ {
		<-done
	}
}

// addWorktree creates dest on branch. A reused feat/<slug> keeps its commits (base = merge-base).
func addWorktree(src, dest, branch, base string) (string, error) {
	if !sys.GitOK(src, "cat-file", "-e", base+"^{commit}") {
		return "", fmt.Errorf("commit %s not found in %s", short(base), src)
	}
	_ = os.MkdirAll(filepath.Dir(dest), 0o755)
	run := func(args ...string) error {
		res := sys.Cmd{Name: "git", Args: append([]string{"-C", src}, args...), Log: ui.LogWriter()}.Run()
		if res.Err != nil {
			return fmt.Errorf("git %s: %s", strings.Join(args, " "), strings.Join(ui.Tail(res.Output, 2), " "))
		}
		return nil
	}
	switch {
	case sys.GitOK(src, "show-ref", "--verify", "--quiet", "refs/heads/"+branch):
		if err := run("worktree", "add", "-q", dest, branch); err != nil {
			return "", err
		}
	case sys.GitOK(src, "show-ref", "--verify", "--quiet", "refs/remotes/origin/"+branch):
		if err := run("worktree", "add", "-q", "--track", "-b", branch, dest, "origin/"+branch); err != nil {
			return "", err
		}
	default:
		if err := run("worktree", "add", "-q", "--no-track", "-b", branch, dest, base); err != nil {
			return "", err
		}
		return base, nil
	}
	if sys.GitOK(dest, "merge-base", "--is-ancestor", "HEAD", base) {
		if sys.GitOK(dest, "merge", "-q", "--ff-only", base) {
			return base, nil
		}
	}
	ui.Info(i18n.T("feature_branch_has_commits", filepath.Base(src), branch))
	mb, err := sys.Git(dest, "merge-base", "HEAD", base)
	return mb, err
}

// excludeEnv keeps copied secrets out of commits through the shared info/exclude.
func excludeEnv(dir string) {
	common, err := sys.Git(dir, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil {
		return
	}
	path := filepath.Join(common, "info", "exclude")
	data, _ := os.ReadFile(path)
	for _, l := range strings.Split(string(data), "\n") {
		if strings.TrimSpace(l) == ".env" {
			return
		}
	}
	_ = os.MkdirAll(filepath.Dir(path), 0o755)
	text := string(data)
	if text != "" && !strings.HasSuffix(text, "\n") {
		text += "\n"
	}
	_ = os.WriteFile(path, []byte(text+".env\n"), 0o644)
}

// portOffset is the first +100·k block whose ports no other workspace recorded.
func portOffset(main string) (int, error) {
	used := workspace.UsedPorts(main)
	for k := 1; k <= 60; k++ {
		off := k * 100
		ok := true
		for _, key := range catalog.PortKeys() {
			if used[catalog.DefaultPorts()[key]+off] {
				ok = false
				break
			}
		}
		if ok {
			return off, nil
		}
	}
	return 0, ui.NewProblem(i18n.T("feature_step_ports"), i18n.T("feature_no_port_block"))
}

func pushedGet(froot, repo string) string {
	data, _ := os.ReadFile(filepath.Join(froot, workspace.DirName, "pushed"))
	last := ""
	for _, l := range strings.Split(string(data), "\n") {
		if k, v, ok := strings.Cut(l, " "); ok && k == repo {
			last = v
		}
	}
	return last
}

func pushedSet(froot, repo, sha string) {
	path := filepath.Join(froot, workspace.DirName, "pushed")
	data, _ := os.ReadFile(path)
	var keep []string
	for _, l := range strings.Split(string(data), "\n") {
		if l != "" && !strings.HasPrefix(l, repo+" ") {
			keep = append(keep, l)
		}
	}
	keep = append(keep, repo+" "+sha)
	_ = os.WriteFile(path, []byte(strings.Join(keep, "\n")+"\n"), 0o644)
}

// Committer writes a commit message for a repository (AI or manual).
type Committer func(repo catalog.Repo, dir string) (string, error)

// Publish commits (no version bump) and pushes feat/<slug>, then pushes the release record.
func Publish(w *workspace.Workspace, slug string, wait, dryRun bool, commit Committer) error {
	main := mainOf(w)
	froot := Dir(main, slug)
	recPath := filepath.Join(froot, workspace.DirName, "release.json")
	rec, err := LoadRecord(recPath)
	if err != nil {
		return ui.NewProblem(i18n.T("feature_step_publish", slug), i18n.T("feature_missing", slug), "./feature new "+slug)
	}
	if err := ValidateSlug(slug); err != nil {
		return err
	}
	branch := "feat/" + slug
	reposMap := rec.Repositories()
	for _, r := range catalog.Repos {
		dir := r.Path(froot)
		if !repos.IsGit(dir) {
			continue
		}
		cur, _ := sys.Git(dir, "symbolic-ref", "--short", "-q", "HEAD")
		if cur != branch {
			return ui.NewProblem(i18n.T("feature_step_publish", slug), i18n.T("feature_wrong_branch", r.Name, cur, branch))
		}
		dirty, _ := sys.Git(dir, "status", "--porcelain")
		if dryRun {
			if dirty != "" {
				ui.Item(r.Name, i18n.T("feature_would_commit"))
			}
			continue
		}
		if dirty != "" {
			msg, err := commit(r, dir)
			if err != nil {
				return err
			}
			if err := git(dir, "add", "-A"); err != nil {
				return err
			}
			staged, _ := sys.Git(dir, "diff", "--cached", "--name-only")
			for _, f := range strings.Split(staged, "\n") {
				if filepath.Base(f) == ".env" {
					_ = git(dir, "reset", "-q", "--", f)
					return ui.NewProblem(i18n.T("feature_step_publish", slug), i18n.T("publish_env_staged", r.Name+"/"+f))
				}
			}
			if err := git(dir, "commit", "-q", "-m", msg); err != nil {
				return err
			}
			ui.Item(r.Name, msg)
		}
		entry := reposMap[r.Name]
		if entry == nil {
			continue
		}
		head, _ := sys.Git(dir, "rev-parse", "HEAD")
		entry["featureCommit"] = head
		lease := pushedGet(froot, r.Name)
		base := str(entry["baseCommit"])
		if (head != base || lease != "") && head != lease {
			// Rebase rewrites feat/<slug>, hence force — but only over the tip this workspace last
			// pushed (empty lease = the branch must not exist yet), never over a teammate's push.
			res := sys.Cmd{Name: "git", Args: []string{"-C", dir, "push", "-q", "--force-with-lease=" + branch + ":" + lease, "origin", branch}, Log: ui.LogWriter()}.Run()
			if res.Err != nil {
				return &ui.Problem{Step: i18n.T("feature_step_publish", slug), Cause: i18n.T("feature_push_rejected", r.Name, branch), Tail: ui.Tail(res.Output, 4)}
			}
			pushedSet(froot, r.Name, head)
		}
	}
	if dryRun {
		ui.Info(i18n.T("feature_changed") + ": " + strings.Join(rec.Changed(), " "))
		return nil
	}
	if err := rec.Save(recPath); err != nil {
		return err
	}
	changed := rec.Changed()
	ui.Info(i18n.T("feature_changed") + ": " + strings.Join(changed, " "))
	requested := now()
	sha, err := pushRecord(main, slug, recPath, "[preview] "+slug+" publish", func(r Record) {
		gen, _ := r["generation"].(float64)
		r["generation"] = gen + 1
		r["action"] = "publish"
		r["requestedAt"] = requested
		images, _ := r["images"].(map[string]any)
		if images == nil {
			images = map[string]any{}
		}
		for _, s := range changed {
			if _, ok := images[s]; !ok {
				images[s] = map[string]any{}
			}
		}
		r["images"] = images
	})
	if err != nil {
		return err
	}
	ui.Info(i18n.T("feature_record_pushed", short(sha), slug))
	if !wait {
		return nil
	}
	return Wait(main, slug, sha, requested, "publish")
}

func git(dir string, args ...string) error {
	res := sys.Cmd{Name: "git", Args: append([]string{"-C", dir}, args...), Log: ui.LogWriter()}.Run()
	if res.Err != nil {
		return &ui.Problem{Step: "git " + strings.Join(args, " "), Cause: strings.Join(ui.Tail(res.Output, 1), ""), Tail: ui.Tail(res.Output, 6)}
	}
	return nil
}

// pushRecord pushes previews/<slug>/release.json to deployment master from a throwaway worktree,
// rebased on the latest master and retried on races. CI-owned fields on master are kept.
func pushRecord(main, slug, local, message string, patch func(Record)) (string, error) {
	dep := filepath.Join(main, "deployment")
	step := i18n.T("feature_step_record")
	for attempt := 0; attempt < 5; attempt++ {
		if res := (sys.Cmd{Name: "git", Args: []string{"-C", dep, "fetch", "-q", "origin", "master"}}).Run(); res.Err != nil {
			return "", &ui.Problem{Step: step, Cause: i18n.T("feature_fetch_deployment"), Tail: ui.Tail(res.Output, 3)}
		}
		tmp, err := os.MkdirTemp("", "ipalpha-record-")
		if err != nil {
			return "", err
		}
		wt := filepath.Join(tmp, "wt")
		cleanup := func() {
			_, _ = sys.Git(dep, "worktree", "remove", "--force", wt)
			os.RemoveAll(tmp)
		}
		if err := git(dep, "worktree", "add", "-q", "--detach", wt, "origin/master"); err != nil {
			os.RemoveAll(tmp)
			return "", err
		}
		target := filepath.Join(wt, "previews", slug, "release.json")
		_ = os.MkdirAll(filepath.Dir(target), 0o755)
		localRec, err := LoadRecord(local)
		if err != nil {
			cleanup()
			return "", err
		}
		rec, err := LoadRecord(target)
		if err != nil {
			rec = localRec
		} else {
			if v, ok := localRec["repositories"]; ok {
				rec["repositories"] = v
			}
			if v, ok := localRec["baselineRelease"]; ok {
				rec["baselineRelease"] = v
			}
		}
		patch(rec)
		if err := rec.Save(target); err != nil {
			cleanup()
			return "", err
		}
		_ = git(wt, "add", filepath.ToSlash(filepath.Join("previews", slug, "release.json")))
		if !sys.GitOK(wt, "diff", "--cached", "--quiet") {
			if err := git(wt, "commit", "-q", "-m", message); err != nil {
				cleanup()
				return "", err
			}
			if res := (sys.Cmd{Name: "git", Args: []string{"-C", wt, "push", "-q", "origin", "HEAD:master"}}).Run(); res.Err != nil {
				cleanup()
				continue
			}
		}
		sha, _ := sys.Git(wt, "rev-parse", "HEAD")
		data, _ := os.ReadFile(target)
		_ = os.WriteFile(local, data, 0o644)
		cleanup()
		return sha, nil
	}
	return "", ui.NewProblem(step, i18n.T("feature_record_retries"))
}

// Wait polls deployment master for CI's record commit (success = lastResult.requestedAt matches).
func Wait(main, slug, sha, requested, action string) error {
	dep := filepath.Join(main, "deployment")
	ui.Info(i18n.T("feature_waiting", short(sha)))
	deadline := time.Now().Add(time.Duration(WaitMinutes()) * time.Minute)
	for time.Now().Before(deadline) {
		_ = (sys.Cmd{Name: "git", Args: []string{"-C", dep, "fetch", "-q", "origin", "master"}}).Run()
		raw, err := sys.Git(dep, "show", "origin/master:previews/"+slug+"/release.json")
		if action == "destroy" && err != nil {
			ui.Done(i18n.T("feature_destroyed", slug))
			return nil
		}
		if err == nil {
			var r Record
			if json.Unmarshal([]byte(raw), &r) == nil {
				last, _ := r["lastResult"].(map[string]any)
				if str(last["requestedAt"]) == requested {
					if str(last["status"]) == "success" {
						printURLs(r)
						return nil
					}
					if str(last["status"]) == "failed" {
						return ui.NewProblem(i18n.T("feature_failed"), str(last["message"]), i18n.T("feature_status_unavailable"))
					}
				}
			}
		}
		time.Sleep(15 * time.Second)
	}
	return ui.NewProblem(i18n.T("feature_waiting_step"), i18n.T("feature_status_unavailable"))
}

func printURLs(r Record) {
	hosts, _ := r["hosts"].([]any)
	for _, h := range hosts {
		ui.Println("  " + ui.Link.Render("https://"+str(h)))
	}
	if len(hosts) > 0 {
		ui.Println("  " + i18n.T("feature_inbox") + ": " + ui.Link.Render("https://"+str(hosts[0])+"/mailbox"))
	}
	build, _ := r["teamcityBuild"].(map[string]any)
	ui.Info(fmt.Sprintf("%s: %s   build: %s", i18n.T("feature_expires"), firstNonEmpty(str(r["expiresAt"]), "-"), firstNonEmpty(str(build["url"]), "-")))
}

func firstNonEmpty(v ...string) string {
	for _, s := range v {
		if s != "" {
			return s
		}
	}
	return ""
}

// Request pushes an action (extend/reset/destroy) to the record.
func Request(w *workspace.Workspace, slug, action string, wait bool) error {
	main := mainOf(w)
	recPath := filepath.Join(Dir(main, slug), workspace.DirName, "release.json")
	if _, err := os.Stat(recPath); err != nil {
		return ui.NewProblem(i18n.T("feature_step_action", action, slug), i18n.T("feature_missing", slug), "./feature list")
	}
	requested := now()
	sha, err := pushRecord(main, slug, recPath, "[preview] "+slug+" "+action, func(r Record) {
		r["action"] = action
		r["requestedAt"] = requested
		if action == "destroy" {
			r["expiresAt"] = requested
		}
	})
	if err != nil {
		return err
	}
	ui.Info(i18n.T("feature_record_pushed", short(sha), slug) + " (" + action + ")")
	if !wait {
		return nil
	}
	return Wait(main, slug, sha, requested, action)
}

// Destroy deletes the preview and the local worktrees (branches are kept).
func Destroy(w *workspace.Workspace, slug string, yes, wait, force bool, stopInfra func(*workspace.Workspace) error) error {
	main := mainOf(w)
	froot := Dir(main, slug)
	step := i18n.T("feature_step_action", "destroy", slug)
	if same(w.Root, froot) {
		return ui.NewProblem(step, i18n.T("feature_destroy_from_main"), "cd "+main)
	}
	for _, r := range catalog.Repos {
		dir := r.Path(froot)
		if !repos.IsGit(dir) {
			continue
		}
		if st, _ := sys.Git(dir, "status", "--porcelain"); st != "" && !force {
			return ui.NewProblem(step, i18n.T("feature_uncommitted", r.Name), "./publish --feature "+slug, "./feature destroy "+slug+" --force")
		}
	}
	if !yes && !ui.TypeToConfirm(i18n.T("feature_confirm"), slug) {
		return ui.ErrCancelled
	}
	if err := Request(w, slug, "destroy", wait); err != nil {
		return err
	}
	if fw, err := workspace.Open(froot); err == nil && stopInfra != nil {
		_ = stopInfra(fw)
	}
	for _, r := range catalog.Repos {
		dir := r.Path(froot)
		if repos.IsGit(dir) {
			_, _ = sys.Git(r.Path(main), "worktree", "remove", "--force", dir)
		}
	}
	os.RemoveAll(froot)
	ui.Done(i18n.T("feature_destroyed", slug))
	return nil
}

func same(a, b string) bool {
	sa, err1 := os.Stat(a)
	sb, err2 := os.Stat(b)
	return err1 == nil && err2 == nil && os.SameFile(sa, sb)
}

// Rebase moves the feature to the latest green Core Deploy.
func Rebase(w *workspace.Workspace, slug string) error {
	main := mainOf(w)
	froot := Dir(main, slug)
	recPath := filepath.Join(froot, workspace.DirName, "release.json")
	rec, err := LoadRecord(recPath)
	step := i18n.T("feature_step_action", "rebase", slug)
	if err != nil {
		return ui.NewProblem(step, i18n.T("feature_missing", slug))
	}
	b, bcommit, err := ReadBaseline(main, "")
	if err != nil {
		return err
	}
	reposMap := rec.Repositories()
	for _, r := range catalog.Repos {
		dir := r.Path(froot)
		if !repos.IsGit(dir) || reposMap[r.Name] == nil {
			continue
		}
		if st, _ := sys.Git(dir, "status", "--porcelain"); st != "" {
			return ui.NewProblem(step, i18n.T("feature_uncommitted", r.Name))
		}
		old := str(reposMap[r.Name]["baseCommit"])
		nb, warn, err := BaseCommit(main, b, r)
		if err != nil {
			return ui.Wrap(step, err)
		}
		if warn != "" {
			ui.Warning(warn)
		}
		if old == nb {
			continue
		}
		_, _ = sys.Git(dir, "fetch", "-q", "origin")
		if res := (sys.Cmd{Name: "git", Args: []string{"-C", dir, "rebase", "-q", "--onto", nb, old}}).Run(); res.Err != nil {
			_, _ = sys.Git(dir, "rebase", "--abort")
			return &ui.Problem{Step: step, Cause: i18n.T("feature_rebase_conflict", r.Name), Tail: ui.Tail(res.Output, 6),
				Fix: []string{fmt.Sprintf("git -C %s rebase --onto %s %s", dir, nb, old)}}
		}
		head, _ := sys.Git(dir, "rev-parse", "HEAD")
		reposMap[r.Name]["baseCommit"] = nb
		reposMap[r.Name]["featureCommit"] = head
		ui.Item(r.Name, short(old)+" → "+short(nb))
	}
	var release any
	if b.Release != "" {
		release = b.Release
	}
	rec["baselineRelease"] = map[string]any{"id": release, "deploymentCommit": bcommit}
	if err := rec.Save(recPath); err != nil {
		return err
	}
	_ = envfile.WriteAtomic(filepath.Join(froot, workspace.DirName, "baseline.json"), b.raw, 0o644)
	ui.Done(i18n.T("feature_rebased", slug))
	return nil
}

// Entry of `./feature list`.
type Entry struct {
	Slug    string
	Gen     int
	State   string
	Expires string
	Local   bool
	Hosts   []string
}

// List reads every preview recorded on deployment master plus local-only features.
func List(w *workspace.Workspace) []Entry {
	main := mainOf(w)
	dep := filepath.Join(main, "deployment")
	_ = (sys.Cmd{Name: "git", Args: []string{"-C", dep, "fetch", "-q", "origin", "master"}}).Run()
	slugs := map[string]bool{}
	if out, err := sys.Git(dep, "ls-tree", "--name-only", "origin/master:previews"); err == nil {
		for _, s := range strings.Split(out, "\n") {
			if s != "" && !strings.HasPrefix(s, "_") {
				slugs[s] = true
			}
		}
	}
	locals, _ := filepath.Glob(filepath.Join(main, "features", "*", workspace.DirName, "release.json"))
	for _, l := range locals {
		slugs[filepath.Base(filepath.Dir(filepath.Dir(l)))] = true
	}
	var out []Entry
	for _, slug := range catalog.SortedKeys(slugs) {
		var r Record
		if raw, err := sys.Git(dep, "show", "origin/master:previews/"+slug+"/release.json"); err == nil {
			_ = json.Unmarshal([]byte(raw), &r)
		} else if rec, err := LoadRecord(filepath.Join(Dir(main, slug), workspace.DirName, "release.json")); err == nil {
			r = rec
		}
		e := Entry{Slug: slug}
		_, err := os.Stat(Dir(main, slug))
		e.Local = err == nil
		gen, _ := r["generation"].(float64)
		e.Gen = int(gen)
		last, _ := r["lastResult"].(map[string]any)
		pending := str(r["requestedAt"]) != "" && str(last["requestedAt"]) != str(r["requestedAt"])
		exp, _ := time.Parse(time.RFC3339, str(r["expiresAt"]))
		switch {
		case e.Gen == 0 && !pending:
			e.State = "local only"
		case pending:
			e.State = firstNonEmpty(str(r["action"]), "publish") + "…"
		case str(last["status"]) == "failed":
			e.State = "failed"
		case exp.IsZero():
			e.State = "publishing"
		case exp.Before(time.Now()):
			e.State = "expired"
		default:
			e.State = "live"
		}
		e.Expires = firstNonEmpty(str(r["expiresAt"]), "-")
		if !exp.IsZero() && exp.After(time.Now()) {
			e.Expires += fmt.Sprintf(" (%dh)", int(time.Until(exp).Hours()+0.5))
		}
		if hosts, ok := r["hosts"].([]any); ok && (e.State == "live" || e.State == "expired" || (pending && e.Gen > 0)) {
			for _, h := range hosts {
				e.Hosts = append(e.Hosts, str(h))
			}
		}
		out = append(out, e)
	}
	return out
}
