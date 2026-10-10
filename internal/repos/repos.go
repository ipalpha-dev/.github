// Package repos clones and updates the workspace repositories in parallel. Cloning tries SSH first
// and falls back to the GitHub CLI (browser sign-in, no SSH key needed).
package repos

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/ipalpha-dev/tooling/internal/catalog"
	"github.com/ipalpha-dev/tooling/internal/i18n"
	"github.com/ipalpha-dev/tooling/internal/sys"
	"github.com/ipalpha-dev/tooling/internal/ui"
)

// Access describes how this machine reaches GitHub.
type Access struct {
	SSH bool
	GH  bool // gh is installed and signed in
}

// Probe checks SSH and gh access (≈ 3s at most).
func Probe() Access {
	var a Access
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		out, errOut, _ := sys.RunTimeout(12*time.Second, "", "", "ssh", "-T", "-o", "StrictHostKeyChecking=accept-new", "-o", "BatchMode=yes", "-o", "ConnectTimeout=8", "git@github.com")
		a.SSH = strings.Contains(out+errOut, "successfully authenticated")
	}()
	go func() {
		defer wg.Done()
		if sys.Has("gh") {
			_, _, err := sys.RunTimeout(10*time.Second, "", "", "gh", "auth", "status", "--hostname", "github.com")
			a.GH = err == nil
		}
	}()
	wg.Wait()
	return a
}

// CloneCommand lets tests replace git (IPALPHA_CLONE_COMMAND <url> <dest>).
func cloneOverride() string { return os.Getenv("IPALPHA_CLONE_COMMAND") }

// URL of a repository over SSH.
func URL(org, repo string) string { return fmt.Sprintf("git@github.com:%s/%s.git", org, repo) }

// Clone clones one repository into dest (kept when it already has content).
func Clone(org, repo, dest string, a Access) error {
	if entries, err := os.ReadDir(dest); err == nil && len(entries) > 0 {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	// Tests (all OSes): clone from local bare repositories <dir>/<repo>.git.
	if from := os.Getenv("IPALPHA_CLONE_FROM"); from != "" {
		src := filepath.Join(from, repo+".git")
		if _, err := os.Stat(src); err != nil {
			return fmt.Errorf("repository not found: %s", repo)
		}
		res := sys.Cmd{Name: "git", Args: []string{"clone", "--quiet", src, dest}}.Run()
		if res.Err != nil {
			return fmt.Errorf("%s", lastLine(res.Output))
		}
		return nil
	}
	if c := cloneOverride(); c != "" {
		out, err := exec.Command(c, URL(org, repo), dest).CombinedOutput()
		if err != nil {
			return fmt.Errorf("%s", strings.TrimSpace(string(out)))
		}
		return nil
	}
	var errs []string
	if a.SSH || !a.GH {
		res := sys.Cmd{Name: "git", Args: []string{"clone", "--quiet", URL(org, repo), dest}, Env: []string{"GIT_TERMINAL_PROMPT=0"}}.Run()
		if res.Err == nil {
			return nil
		}
		errs = append(errs, lastLine(res.Output))
		os.RemoveAll(dest)
	}
	if a.GH || sys.Has("gh") {
		res := sys.Cmd{Name: "gh", Args: []string{"repo", "clone", org + "/" + repo, dest, "--", "--quiet"}}.Run()
		if res.Err == nil {
			return nil
		}
		errs = append(errs, lastLine(res.Output))
		os.RemoveAll(dest)
	}
	return fmt.Errorf("%s", strings.Join(errs, " / "))
}

// lastLine is the most useful line of git's output: an "error:"/"fatal:"/rejected line when there is one.
func lastLine(s string) string {
	lines := ui.Tail(s, 12)
	for i := len(lines) - 1; i >= 0; i-- {
		l := strings.TrimSpace(lines[i])
		if strings.HasPrefix(l, "fatal:") || strings.HasPrefix(l, "error:") || strings.Contains(l, "[rejected]") || strings.HasPrefix(l, "ERROR") {
			return l
		}
	}
	if len(lines) == 0 {
		return "git exited with an error"
	}
	return strings.TrimSpace(lines[len(lines)-1])
}

// Outcome of one repository.
type Outcome struct {
	Repo   catalog.Repo
	Status string // cloned | kept | pulled | conflict | fetched | skipped | failed
	Detail string
}

// CloneAll clones every catalog repository into root, 6 at a time. Optional app repos that fail
// (no access) are skipped quietly.
func CloneAll(org, root string, a Access, progress func(Outcome)) []Outcome {
	return parallel(catalog.Repos, func(r catalog.Repo) Outcome {
		dest := r.Path(root)
		if entries, err := os.ReadDir(dest); err == nil && len(entries) > 0 {
			return Outcome{Repo: r, Status: "kept"}
		}
		if err := Clone(org, r.Name, dest, a); err != nil {
			if r.Kind == catalog.KindOptional {
				return Outcome{Repo: r, Status: "skipped", Detail: i18n.T("repos_optional_skipped")}
			}
			return Outcome{Repo: r, Status: "failed", Detail: err.Error()}
		}
		return Outcome{Repo: r, Status: "cloned"}
	}, progress)
}

// IsGit reports a repository or worktree (worktrees have a .git file).
func IsGit(dir string) bool {
	if _, err := os.Stat(filepath.Join(dir, ".git")); err != nil {
		return false
	}
	return sys.GitOK(dir, "rev-parse", "--git-dir")
}

// PullAll fast-forwards every repository (clones missing ones). Feature workspaces only fetch.
func PullAll(org, root string, fetchOnly bool, a Access, progress func(Outcome)) []Outcome {
	return parallel(catalog.Repos, func(r catalog.Repo) Outcome {
		dir := r.Path(root)
		if _, err := os.Stat(dir); err != nil {
			if fetchOnly {
				return Outcome{Repo: r, Status: "skipped"}
			}
			if err := Clone(org, r.Name, dir, a); err != nil {
				if r.Kind == catalog.KindOptional {
					return Outcome{Repo: r, Status: "skipped", Detail: i18n.T("repos_optional_skipped")}
				}
				return Outcome{Repo: r, Status: "failed", Detail: err.Error()}
			}
			return Outcome{Repo: r, Status: "cloned"}
		}
		if !IsGit(dir) {
			return Outcome{Repo: r, Status: "skipped", Detail: i18n.T("repos_not_git")}
		}
		if res := (sys.Cmd{Name: "git", Args: []string{"-C", dir, "fetch", "--prune"}, Env: []string{"GIT_TERMINAL_PROMPT=0"}}).Run(); res.Err != nil {
			return Outcome{Repo: r, Status: "failed", Detail: i18n.T("repos_fetch_failed") + ": " + lastLine(res.Output)}
		}
		// Tags separately and forced: origin owns release tags, and a tag moved there (re-tagged
		// release) must not block the pull with "would clobber existing tag".
		_ = (sys.Cmd{Name: "git", Args: []string{"-C", dir, "fetch", "--tags", "--force"}, Env: []string{"GIT_TERMINAL_PROMPT=0"}}).Run()
		if fetchOnly {
			return Outcome{Repo: r, Status: "fetched"}
		}
		if _, err := sys.Git(dir, "rev-parse", "--abbrev-ref", "@{u}"); err != nil {
			return Outcome{Repo: r, Status: "fetched", Detail: i18n.T("repos_no_upstream")}
		}
		before, _ := sys.Git(dir, "rev-parse", "HEAD")
		if res := (sys.Cmd{Name: "git", Args: []string{"-C", dir, "merge", "--ff-only", "--quiet", "@{u}"}}).Run(); res.Err != nil {
			return Outcome{Repo: r, Status: "conflict", Detail: i18n.T("repos_ff_failed")}
		}
		after, _ := sys.Git(dir, "rev-parse", "HEAD")
		if before == after {
			return Outcome{Repo: r, Status: "current"}
		}
		return Outcome{Repo: r, Status: "pulled"}
	}, progress)
}

func parallel(list []catalog.Repo, fn func(catalog.Repo) Outcome, progress func(Outcome)) []Outcome {
	out := make([]Outcome, len(list))
	sem := make(chan struct{}, 6)
	var wg sync.WaitGroup
	var mu sync.Mutex
	for i, r := range list {
		wg.Add(1)
		sem <- struct{}{}
		go func(i int, r catalog.Repo) {
			defer wg.Done()
			defer func() { <-sem }()
			o := fn(r)
			out[i] = o
			if progress != nil {
				mu.Lock()
				progress(o)
				mu.Unlock()
			}
		}(i, r)
	}
	wg.Wait()
	return out
}
