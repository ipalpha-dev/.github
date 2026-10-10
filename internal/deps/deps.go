// Package deps installs npm dependencies in parallel, building shared-js/shared-ui first because the
// services copy them (install-links), and refreshes those copies after a rebuild.
package deps

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/ipalpha-dev/tooling/internal/catalog"
	"github.com/ipalpha-dev/tooling/internal/i18n"
	"github.com/ipalpha-dev/tooling/internal/sys"
	"github.com/ipalpha-dev/tooling/internal/ui"
	"github.com/ipalpha-dev/tooling/internal/workspace"
)

// Task is one npm action.
type Task struct {
	Repo   catalog.Repo
	Dir    string
	Action string // install | build
}

// Plan lists what must run, in phases.
func Plan(w *workspace.Workspace) [][]Task {
	var libsInstall, libsBuild, rest []Task
	for _, r := range w.Present(catalog.KindLibrary) {
		dir := r.Path(w.Root)
		if !exists(filepath.Join(dir, "package.json")) {
			continue
		}
		if NeedsInstall(dir) {
			libsInstall = append(libsInstall, Task{r, dir, "install"})
		}
		if NeedsBuild(dir) {
			libsBuild = append(libsBuild, Task{r, dir, "build"})
		}
	}
	for _, r := range w.Present(catalog.KindAPI, catalog.KindWeb) {
		dir := r.Path(w.Root)
		if exists(filepath.Join(dir, "package.json")) && NeedsInstall(dir) {
			rest = append(rest, Task{r, dir, "install"})
		}
	}
	var phases [][]Task
	for _, p := range [][]Task{libsInstall, libsBuild, rest} {
		if len(p) > 0 {
			phases = append(phases, p)
		}
	}
	return phases
}

func exists(p string) bool { _, err := os.Stat(p); return err == nil }

func mtime(p string) time.Time {
	st, err := os.Stat(p)
	if err != nil {
		return time.Time{}
	}
	return st.ModTime()
}

// NeedsInstall: no node_modules, a newer lockfile, or a copied shared lib without dist.
func NeedsInstall(dir string) bool {
	nm := filepath.Join(dir, "node_modules")
	if !exists(nm) {
		return true
	}
	if lock := filepath.Join(dir, "package-lock.json"); exists(lock) && mtime(lock).After(mtime(nm)) {
		return true
	}
	for _, lib := range []string{"shared-js", "shared-ui"} {
		copyDir := filepath.Join(nm, "@ipalpha", lib)
		if exists(copyDir) && !exists(filepath.Join(copyDir, "dist")) {
			return true
		}
	}
	return false
}

// NeedsBuild: no dist, or a source file newer than dist.
func NeedsBuild(dir string) bool {
	dist := filepath.Join(dir, "dist")
	if !exists(dist) {
		return true
	}
	return newerThan(filepath.Join(dir, "src"), mtime(dist))
}

// NeedsBuildSince reports a file under srcDir newer than target.
func NeedsBuildSince(srcDir, target string) bool { return newerThan(srcDir, mtime(target)) }

func newerThan(root string, t time.Time) bool {
	found := false
	_ = filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil || found {
			return filepath.SkipDir
		}
		if !d.IsDir() {
			if info, err := d.Info(); err == nil && info.ModTime().After(t) {
				found = true
				return filepath.SkipAll
			}
		}
		return nil
	})
	return found
}

// Run executes every phase with up to `jobs` parallel tasks. Each task logs to
// .ipalpha/logs/npm/<repo>-<action>.log; the first failure stops the run with its last lines.
func Run(w *workspace.Workspace, progress func(string)) error {
	jobs := runtime.NumCPU()
	if jobs > 6 {
		jobs = 6
	}
	if jobs < 2 {
		jobs = 2
	}
	logDir := filepath.Join(w.LogsDir(), "npm")
	_ = os.MkdirAll(logDir, 0o755)
	for _, phase := range Plan(w) {
		if err := runPhase(phase, jobs, logDir, progress); err != nil {
			return err
		}
	}
	RefreshSharedCopies(w, progress)
	return nil
}

func runPhase(tasks []Task, jobs int, logDir string, progress func(string)) error {
	sem := make(chan struct{}, jobs)
	var wg sync.WaitGroup
	var mu sync.Mutex
	var first error
	for _, t := range tasks {
		t := t
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			mu.Lock()
			stop := first != nil
			mu.Unlock()
			if stop {
				return
			}
			if progress != nil {
				progress(fmt.Sprintf("npm %s · %s", t.Action, t.Repo.Rel()))
			}
			if err := runTask(t, logDir); err != nil {
				mu.Lock()
				if first == nil {
					first = err
				}
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	return first
}

func runTask(t Task, logDir string) error {
	logPath := filepath.Join(logDir, strings.ReplaceAll(t.Repo.Rel(), "/", "_")+"-"+t.Action+".log")
	f, err := os.Create(logPath)
	if err != nil {
		return err
	}
	defer f.Close()
	var args []string
	if t.Action == "install" {
		for _, lib := range []string{"shared-js", "shared-ui"} {
			os.RemoveAll(filepath.Join(t.Dir, "node_modules", "@ipalpha", lib))
		}
		args = []string{"install", "--no-audit", "--no-fund"}
	} else {
		args = []string{"run", "build"}
	}
	res := sys.Cmd{Dir: t.Dir, Env: sys.NodeEnv(), Name: sys.Npm(), Args: args, Log: io.Writer(f)}.Run()
	if res.Err != nil {
		return &ui.Problem{
			Step:  i18n.T("deps_failed", "npm "+t.Action, t.Repo.Rel()),
			Cause: npmCause(res.Output),
			Tail:  ui.Tail(filterNpm(res.Output), 20),
			Log:   logPath,
			Fix:   npmFix(res.Output, t),
		}
	}
	now := time.Now()
	target := filepath.Join(t.Dir, "node_modules")
	if t.Action == "build" {
		target = filepath.Join(t.Dir, "dist")
	}
	_ = os.Chtimes(target, now, now)
	return nil
}

func filterNpm(out string) string {
	var keep []string
	for _, l := range strings.Split(out, "\n") {
		if strings.TrimSpace(l) == "npm error" {
			continue
		}
		keep = append(keep, l)
	}
	return strings.Join(keep, "\n")
}

func npmCause(out string) string {
	low := strings.ToLower(out)
	switch {
	case strings.Contains(low, "eresolve"):
		return i18n.T("deps_cause_eresolve")
	case strings.Contains(low, "enotfound") || strings.Contains(low, "etimedout") || strings.Contains(low, "econnreset"):
		return i18n.T("deps_cause_network")
	case strings.Contains(low, "e404") && strings.Contains(low, "@ipalpha"):
		return i18n.T("deps_cause_private")
	case strings.Contains(low, "eacces") || strings.Contains(low, "eperm"):
		return i18n.T("deps_cause_permission")
	case strings.Contains(low, "gyp") || strings.Contains(low, "node-pre-gyp"):
		return i18n.T("deps_cause_native")
	case strings.Contains(low, "error ts") || strings.Contains(low, "error  ts"):
		return i18n.T("deps_cause_typescript")
	}
	return i18n.T("deps_cause_generic")
}

func npmFix(out string, t Task) []string {
	low := strings.ToLower(out)
	fix := []string{}
	switch {
	case strings.Contains(low, "enotfound") || strings.Contains(low, "etimedout"):
		fix = append(fix, i18n.T("deps_fix_network"))
	case strings.Contains(low, "eacces") || strings.Contains(low, "eperm"):
		if sys.Windows {
			fix = append(fix, i18n.T("deps_fix_windows_lock"))
		} else {
			fix = append(fix, i18n.T("deps_fix_permission"))
		}
	case strings.Contains(low, "gyp"):
		fix = append(fix, i18n.T("deps_fix_native"))
	}
	fix = append(fix, i18n.T("deps_fix_clean", filepath.Join(t.Repo.Rel(), "node_modules")))
	return fix
}

// RefreshSharedCopies copies a rebuilt shared library's dist into the consumers that copied it.
func RefreshSharedCopies(w *workspace.Workspace, progress func(string)) {
	for _, lib := range []string{"shared-js", "shared-ui"} {
		src := filepath.Join(w.Repo(lib), "dist")
		if !exists(src) {
			continue
		}
		for _, r := range w.Present(catalog.KindAPI, catalog.KindWeb) {
			cp := filepath.Join(r.Path(w.Root), "node_modules", "@ipalpha", lib)
			st, err := os.Lstat(cp)
			if err != nil || st.Mode()&os.ModeSymlink != 0 || !exists(filepath.Join(cp, "dist")) {
				continue
			}
			if !newerThan(src, mtime(filepath.Join(cp, "dist"))) {
				continue
			}
			os.RemoveAll(filepath.Join(cp, "dist"))
			if err := copyTree(src, filepath.Join(cp, "dist")); err == nil {
				_ = copyFile(filepath.Join(w.Repo(lib), "package.json"), filepath.Join(cp, "package.json"))
				if progress != nil {
					progress(fmt.Sprintf("refresh %s → %s", lib, r.Rel()))
				}
			}
		}
	}
}

func copyTree(src, dst string) error {
	return filepath.WalkDir(src, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		return copyFile(p, target)
	})
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}
