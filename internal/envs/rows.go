// Package envs is the ./envs picker: one row per IPAlpha environment on this computer (the main
// workspace and every features/<slug>), with ./run state, tmux session, agents working there, git
// changes and open warden requests, plus the tmux commands that hop between them.
package envs

import (
	"bufio"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/ipalpha-dev/tooling/internal/catalog"
	"github.com/ipalpha-dev/tooling/internal/ports"
	"github.com/ipalpha-dev/tooling/internal/run"
	"github.com/ipalpha-dev/tooling/internal/sys"
	"github.com/ipalpha-dev/tooling/internal/workspace"
)

// MainName is the name (and tmux suffix) of the main workspace.
const MainName = "main"

// RunInfo is the ./run state of one workspace.
type RunInfo struct {
	Owner int    // pid of the live ./run, 0 when none
	Up    int    // services listening
	Total int    // services ./run starts here
	URL   string // Oikos URL of this workspace (actual port when ./run assigned one)
}

// Proc is a running process with its working folder (agents are found among these).
type Proc struct {
	Name string
	Cwd  string
}

// Row is one environment.
type Row struct {
	Name     string // "main" or the feature slug
	Root     string
	Main     bool
	Current  bool // the workspace ./envs was called from
	Branches string
	Run      RunInfo
	Session  bool // tmux session ipalpha-<name> exists
	Agents   map[string]int
	Dirty    int // uncommitted files over every repository
	Ahead    int // commits ahead of the default branch over every repository
	Requests int // open warden requests filed from this feature
}

// Deps are the outside world; tests replace them.
type Deps struct {
	Git      func(dir string, args ...string) (string, error)
	Procs    func() []Proc
	Sessions func() map[string]bool
	RunInfo  func(root string) RunInfo
	// QADir is ~/src/ipalpha-qa (requests/<area>/open/*.md); empty or missing skips requests.
	QADir string
}

// SessionName is the tmux session of an environment.
func SessionName(name string) string { return "ipalpha-" + name }

// DefaultDeps wires the real git, ps/lsof, tmux and ./run state.
func DefaultDeps(t *Tmux) Deps {
	qa := os.Getenv("IPALPHA_QA_DIR")
	if qa == "" {
		if h, err := os.UserHomeDir(); err == nil {
			qa = filepath.Join(h, "src", "ipalpha-qa")
		}
	}
	return Deps{Git: sys.Git, Procs: listProcs, Sessions: t.Sessions, RunInfo: RunInfoOf, QADir: qa}
}

// Discover lists the environments around a main workspace: main first, then features/<slug> by name.
func Discover(main string) []Row {
	rows := []Row{{Name: MainName, Root: main, Main: true}}
	entries, _ := os.ReadDir(filepath.Join(main, "features"))
	for _, e := range entries {
		root := filepath.Join(main, "features", e.Name())
		if st, err := os.Stat(filepath.Join(root, workspace.DirName, "settings")); err == nil && !st.IsDir() {
			rows = append(rows, Row{Name: e.Name(), Root: root})
		}
	}
	sort.SliceStable(rows[1:], func(i, j int) bool { return rows[1+i].Name < rows[1+j].Name })
	return rows
}

// Gather fills every row. current is the root ./envs was started from.
func Gather(main, current string, d Deps) []Row {
	rows := Discover(main)
	sessions := d.Sessions()
	procs := d.Procs()
	reqs := requestsByFrom(d.QADir)
	var wg sync.WaitGroup
	sem := make(chan struct{}, 6)
	for i := range rows {
		r := &rows[i]
		r.Current = samePath(r.Root, current)
		r.Session = sessions[SessionName(r.Name)]
		r.Requests = reqs[r.Name]
		r.Agents = agentsIn(rows, r, procs)
		if d.RunInfo != nil {
			r.Run = d.RunInfo(r.Root)
		}
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			r.Branches, r.Dirty, r.Ahead = gitSummary(r.Root, d.Git)
		}()
	}
	wg.Wait()
	return rows
}

func samePath(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	ra, err1 := filepath.EvalSymlinks(a)
	rb, err2 := filepath.EvalSymlinks(b)
	if err1 != nil || err2 != nil {
		return filepath.Clean(a) == filepath.Clean(b)
	}
	return ra == rb
}

// agentsIn counts the agent processes whose folder is inside row r (and not inside a deeper row,
// because features/<slug> sits inside the main workspace folder).
func agentsIn(rows []Row, r *Row, procs []Proc) map[string]int {
	out := map[string]int{}
	for _, p := range procs {
		best := -1
		for i := range rows {
			if inside(rows[i].Root, p.Cwd) && (best < 0 || len(rows[i].Root) > len(rows[best].Root)) {
				best = i
			}
		}
		if best >= 0 && rows[best].Root == r.Root {
			out[p.Name]++
		}
	}
	return out
}

func inside(root, dir string) bool {
	if dir == "" {
		return false
	}
	rel, err := filepath.Rel(root, dir)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

var defaultBranches = []string{"master", "main"}

// gitSummary summarises every repository folder of a workspace: branch names, uncommitted files and
// commits ahead of the default branch.
func gitSummary(root string, git func(string, ...string) (string, error)) (branches string, dirty, ahead int) {
	counts := map[string]int{}
	for _, repo := range catalog.Repos {
		dir := repo.Path(root)
		if st, err := os.Stat(filepath.Join(dir, ".git")); err != nil || st == nil {
			continue
		}
		br, err := git(dir, "branch", "--show-current")
		if err != nil || br == "" {
			br = "(detached)"
		}
		counts[br]++
		if out, err := git(dir, "status", "--porcelain"); err == nil && out != "" {
			dirty += len(strings.Split(strings.TrimSpace(out), "\n"))
		}
		for _, def := range defaultBranches {
			if def == br {
				break
			}
			if out, err := git(dir, "rev-list", "--count", def+"..HEAD"); err == nil {
				n, _ := strconv.Atoi(strings.TrimSpace(out))
				ahead += n
				break
			}
		}
	}
	return summarizeBranches(counts), dirty, ahead
}

// summarizeBranches shows the non-default branches (with how many repos are on each), or the
// default branch when nothing else is checked out.
func summarizeBranches(counts map[string]int) string {
	type bc struct {
		b string
		n int
	}
	var other []bc
	var def []bc
	for b, n := range counts {
		if b == "master" || b == "main" {
			def = append(def, bc{b, n})
		} else {
			other = append(other, bc{b, n})
		}
	}
	list := other
	if len(list) == 0 {
		list = def
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].n != list[j].n {
			return list[i].n > list[j].n
		}
		return list[i].b < list[j].b
	})
	var parts []string
	for _, x := range list {
		if len(list) > 1 || len(other) > 0 && x.n > 1 {
			parts = append(parts, x.b+" ×"+strconv.Itoa(x.n))
		} else {
			parts = append(parts, x.b)
		}
	}
	return strings.Join(parts, ", ")
}

// RunInfoOf reads the ./run state of a workspace folder: live owner, listening services and Oikos URL.
func RunInfoOf(root string) RunInfo {
	w, err := workspace.Open(root)
	if err != nil {
		return RunInfo{}
	}
	plan, ok := ports.Load(w.StateDir())
	port := w.Settings.Port("oikos-webapp")
	if ok {
		port = plan.Port("oikos-webapp")
	}
	info := RunInfo{Owner: run.RunningOwner(w), URL: "http://localhost:" + strconv.Itoa(port)}
	for _, repo := range w.Present(catalog.KindAPI, catalog.KindWeb) {
		info.Total++
		p := w.Settings.Port(repo.Name)
		if ok {
			p = plan.Port(repo.Name)
		}
		if info.Owner != 0 && sys.PortBusy(p) {
			info.Up++
		}
	}
	return info
}

// requestsByFrom counts open warden requests per `from:` first word.
func requestsByFrom(qa string) map[string]int {
	out := map[string]int{}
	if qa == "" {
		return out
	}
	files, _ := filepath.Glob(filepath.Join(qa, "requests", "*", "open", "*.md"))
	for _, f := range files {
		if from := fromOf(f); from != "" {
			out[from]++
		}
	}
	return out
}

func fromOf(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for i := 0; i < 20 && sc.Scan(); i++ {
		if v, ok := strings.CutPrefix(sc.Text(), "from:"); ok {
			fields := strings.Fields(v)
			if len(fields) > 0 {
				return strings.Trim(fields[0], "(),;:")
			}
		}
	}
	return ""
}

// Find returns the row named name ("main" or a slug).
func Find(rows []Row, name string) (Row, bool) {
	for _, r := range rows {
		if r.Name == name {
			return r, true
		}
	}
	return Row{}, false
}

// AgentsText is "claude, pi ×2".
func (r Row) AgentsText() string {
	names := make([]string, 0, len(r.Agents))
	for n := range r.Agents {
		names = append(names, n)
	}
	sort.Strings(names)
	for i, n := range names {
		if c := r.Agents[n]; c > 1 {
			names[i] = n + " ×" + strconv.Itoa(c)
		}
	}
	return strings.Join(names, ", ")
}
