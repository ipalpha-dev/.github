package envs

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/ipalpha-dev/tooling/internal/i18n"
)

func mkWorkspace(t *testing.T, root string, repos ...string) {
	t.Helper()
	must := func(err error) {
		if err != nil {
			t.Fatal(err)
		}
	}
	must(os.MkdirAll(filepath.Join(root, ".ipalpha"), 0o755))
	must(os.WriteFile(filepath.Join(root, ".ipalpha", "settings"), []byte("lang=en-US\n"), 0o644))
	for _, r := range repos {
		must(os.MkdirAll(filepath.Join(root, r, ".git"), 0o755))
	}
}

func fakeGit(branches map[string]string, dirty map[string]string, ahead map[string]string) func(string, ...string) (string, error) {
	return func(dir string, args ...string) (string, error) {
		switch args[0] {
		case "branch":
			return branches[dir], nil
		case "status":
			return dirty[dir], nil
		case "rev-list":
			if a, ok := ahead[dir]; ok && strings.HasPrefix(args[2], "master") {
				return a, nil
			}
			return "", fmt.Errorf("no such branch")
		}
		return "", fmt.Errorf("unexpected %v", args)
	}
}

func TestGatherRows(t *testing.T) {
	i18n.Set("en-US")
	defer i18n.Set("pt-BR")
	main := t.TempDir()
	mkWorkspace(t, main, "core/auth-api", "core/oikos-webapp")
	feat := filepath.Join(main, "features", "forms")
	mkWorkspace(t, feat, "core/auth-api", "core/oikos-webapp")
	mkWorkspace(t, filepath.Join(main, "features", "zeta"), "core/auth-api")
	os.MkdirAll(filepath.Join(main, "features", "stray"), 0o755) // not a workspace

	qa := t.TempDir()
	os.MkdirAll(filepath.Join(qa, "requests", "github", "open"), 0o755)
	os.WriteFile(filepath.Join(qa, "requests", "github", "open", "a.md"), []byte("from: forms\nneed: x\n"), 0o644)
	os.WriteFile(filepath.Join(qa, "requests", "github", "open", "b.md"), []byte("from: forms (agent)\n"), 0o644)
	os.WriteFile(filepath.Join(qa, "requests", "github", "open", "c.md"), []byte("from: warden (Kevyn)\n"), 0o644)

	fa, fo := filepath.Join(feat, "core/auth-api"), filepath.Join(feat, "core/oikos-webapp")
	d := Deps{
		Git: fakeGit(
			map[string]string{filepath.Join(main, "core/auth-api"): "master", filepath.Join(main, "core/oikos-webapp"): "master",
				fa: "feat/forms", fo: "feat/forms", filepath.Join(main, "features/zeta/core/auth-api"): "master"},
			map[string]string{fa: " M a.ts\n?? b.ts", fo: ""},
			map[string]string{fa: "2", fo: "1"}),
		Procs: func() []Proc {
			return []Proc{{"claude", feat + "/core/auth-api"}, {"pi", feat}, {"claude", main + "/core"}, {"codex", "/elsewhere"}}
		},
		Sessions: func() map[string]bool { return map[string]bool{"ipalpha-forms": true} },
		RunInfo: func(root string) RunInfo {
			if root == feat {
				return RunInfo{Owner: 42, Up: 3, Total: 5, URL: "http://localhost:5210"}
			}
			return RunInfo{Total: 2, URL: "http://localhost:5110"}
		},
		QADir: qa,
	}
	rows := Gather(main, feat, d)
	var names []string
	for _, r := range rows {
		names = append(names, r.Name)
	}
	if !reflect.DeepEqual(names, []string{"main", "forms", "zeta"}) {
		t.Fatalf("rows %v", names)
	}
	m, f := rows[0], rows[1]
	if !m.Main || m.Current || m.Session || m.Branches != "master" || m.Dirty != 0 || m.Requests != 0 {
		t.Errorf("main row: %+v", m)
	}
	if m.AgentsText() != "claude" || f.AgentsText() != "claude, pi" {
		t.Errorf("agents main=%q forms=%q (feature folders must not count for main)", m.AgentsText(), f.AgentsText())
	}
	if !f.Current || !f.Session || f.Branches != "feat/forms ×2" || f.Dirty != 2 || f.Ahead != 3 || f.Requests != 2 {
		t.Errorf("forms row: %+v", f)
	}
	if f.Run.Up != 3 || f.Run.URL != "http://localhost:5210" {
		t.Errorf("run: %+v", f.Run)
	}
	table := PlainTable(rows)
	for _, want := range []string{"forms *", "3/5 up", "stopped", "feat/forms ×2", "±2 ↑3", "[claude, pi]", "2 request(s)"} {
		if !strings.Contains(table, want) {
			t.Errorf("table missing %q:\n%s", want, table)
		}
	}
}

func TestMissingQADirIsSkipped(t *testing.T) {
	if n := requestsByFrom(filepath.Join(t.TempDir(), "nope")); len(n) != 0 {
		t.Fatal(n)
	}
	if n := requestsByFrom(""); len(n) != 0 {
		t.Fatal(n)
	}
}

func TestParseAgents(t *testing.T) {
	ps := `  101 /Users/k/.local/bin/claude --dangerously-skip-permissions
  102 pi
  103 /usr/bin/node /opt/homebrew/bin/codex --x
  104 /bin/zsh -c claude
  105 grok
  106 /usr/bin/vim pi.txt
`
	got := ParseAgents(ps)
	want := map[string]string{"101": "claude", "102": "pi", "103": "codex", "105": "grok"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("%v", got)
	}
	procs := ParseCwds("p101\nfcwd\nn/a/b\np102\nn/c\np999\nn/zzz\n", got)
	if len(procs) != 2 || procs[0] != (Proc{"claude", "/a/b"}) || procs[1] != (Proc{"pi", "/c"}) {
		t.Fatalf("%v", procs)
	}
}

func TestFilter(t *testing.T) {
	rows := []Row{{Name: "main", Branches: "master"}, {Name: "forms", Branches: "feat/forms", Agents: map[string]int{"claude": 1}}}
	if got := Filter(rows, "feat"); len(got) != 1 || got[0].Name != "forms" {
		t.Fatal(got)
	}
	if got := Filter(rows, "CLAUDE forms"); len(got) != 1 {
		t.Fatal(got)
	}
	if got := Filter(rows, "zzz"); len(got) != 0 {
		t.Fatal(got)
	}
	if got := Filter(rows, " "); len(got) != 2 {
		t.Fatal(got)
	}
}

func TestSummarizeBranches(t *testing.T) {
	cases := []struct {
		in   map[string]int
		want string
	}{
		{map[string]int{"master": 20}, "master"},
		{map[string]int{"master": 18, "feat/x": 2}, "feat/x ×2"},
		{map[string]int{"feat/x": 1}, "feat/x"},
		{map[string]int{"feat/a": 2, "feat/b": 2, "master": 5}, "feat/a ×2, feat/b ×2"},
	}
	for _, c := range cases {
		if got := summarizeBranches(c.in); got != c.want {
			t.Errorf("%v → %q want %q", c.in, got, c.want)
		}
	}
}

// ── tmux command construction, with a fake runner ──

type fake struct {
	calls    []string
	sessions map[string]bool
	windows  map[string]bool
}

func (f *fake) Output(args ...string) (string, error) {
	f.calls = append(f.calls, strings.Join(args, " "))
	switch args[0] {
	case "has-session":
		if f.sessions[strings.TrimPrefix(args[2], "=")] {
			return "", nil
		}
		return "", fmt.Errorf("no session")
	case "list-windows":
		var out []string
		for w := range f.windows {
			out = append(out, w)
		}
		return strings.Join(out, "\n"), nil
	case "list-sessions":
		var out []string
		for s := range f.sessions {
			out = append(out, s)
		}
		return strings.Join(out, "\n"), nil
	}
	return "", nil
}

func (f *fake) Interactive(args ...string) error {
	f.calls = append(f.calls, "INTERACTIVE "+strings.Join(args, " "))
	return nil
}

func TestHopCreatesSessionThenAttachesOutsideTmux(t *testing.T) {
	f := &fake{sessions: map[string]bool{}}
	tm := &Tmux{R: f, Installed: true}
	if err := tm.Do(ActHop, "forms", "/w/features/forms"); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"has-session -t =ipalpha-forms",
		"new-session -d -s ipalpha-forms -c /w/features/forms",
		"INTERACTIVE attach-session -t =ipalpha-forms",
	}
	if !reflect.DeepEqual(f.calls, want) {
		t.Fatalf("%q", f.calls)
	}
}

func TestHopSwitchesClientInsideTmux(t *testing.T) {
	f := &fake{sessions: map[string]bool{"ipalpha-main": true}}
	tm := &Tmux{R: f, Installed: true, Inside: true}
	if err := tm.Do(ActHop, "main", "/w"); err != nil {
		t.Fatal(err)
	}
	want := []string{"has-session -t =ipalpha-main", "switch-client -t =ipalpha-main"}
	if !reflect.DeepEqual(f.calls, want) {
		t.Fatalf("%q", f.calls)
	}
}

func TestShellLogsRunWindows(t *testing.T) {
	f := &fake{sessions: map[string]bool{"ipalpha-forms": true}, windows: map[string]bool{}}
	tm := &Tmux{R: f, Installed: true, Inside: true}
	_ = tm.Do(ActShell, "forms", "/w/f")
	_ = tm.Do(ActLogs, "forms", "/w/f")
	_ = tm.Do(ActRun, "forms", "/w/f")
	joined := strings.Join(f.calls, "\n")
	for _, want := range []string{
		"new-window -t =ipalpha-forms: -c /w/f\n",
		"new-window -t =ipalpha-forms: -c /w/f -n logs '/w/f/logs' ||",
		"new-window -t =ipalpha-forms: -c /w/f -n run '/w/f/run' ||",
	} {
		if !strings.Contains(joined+"\n", want) {
			t.Errorf("missing %q in\n%s", want, joined)
		}
	}
	// An existing run window is selected, not duplicated.
	f.calls, f.windows["run"] = nil, true
	_ = tm.Do(ActRun, "forms", "/w/f")
	if strings.Contains(strings.Join(f.calls, "\n"), "new-window") || !strings.Contains(strings.Join(f.calls, "\n"), "select-window -t =ipalpha-forms:=run") {
		t.Errorf("%q", f.calls)
	}
}

func TestStartRun(t *testing.T) {
	f := &fake{sessions: map[string]bool{}}
	tm := &Tmux{R: f, Installed: true}
	handled, err := tm.StartRun("main", "/w", []string{"--plain=false", "--only", "it's"})
	if err != nil || !handled {
		t.Fatal(handled, err)
	}
	joined := strings.Join(f.calls, "\n")
	if !strings.Contains(joined, `-n run '/w/run' '--plain=false' '--only' 'it'\''s' ||`) || !strings.Contains(joined, "INTERACTIVE attach-session -t =ipalpha-main") {
		t.Fatalf("%s", joined)
	}
	// Inside tmux or without tmux, ./run just runs where it is.
	for _, tm := range []*Tmux{{R: f, Installed: true, Inside: true}, {R: f}} {
		f.calls = nil
		if handled, err := tm.StartRun("main", "/w", nil); handled || err != nil || len(f.calls) != 0 {
			t.Fatal(handled, err, f.calls)
		}
	}
}

func TestMissingTmux(t *testing.T) {
	f := &fake{}
	tm := &Tmux{R: f}
	if err := tm.Do(ActHop, "main", "/w"); err == nil || !strings.Contains(err.Error(), "tmux") {
		t.Fatal(err)
	}
	if len(f.calls) != 0 || len(tm.Sessions()) != 0 {
		t.Fatal("must not call tmux")
	}
}
