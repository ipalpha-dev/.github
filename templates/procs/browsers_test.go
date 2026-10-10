package main

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

func TestBrowserSelectionPersistsIncludingEmpty(t *testing.T) {
	dir := t.TempDir()
	want := map[string]bool{"auth-webapp": true, "mordomia-webapp": true, "mailpit": true}
	if got := browserSelection(dir); !reflect.DeepEqual(got, want) {
		t.Fatalf("defaults = %v", got)
	}
	for _, value := range []string{"", "forms-webapp mailpit"} {
		if err := os.WriteFile(filepath.Join(dir, "settings"), []byte("browser_apps="+value+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		got := browserSelection(dir)
		if value == "" && len(got) != 0 {
			t.Fatalf("empty selection = %v", got)
		}
		if value != "" && (!got["mailpit"] || !got["forms-webapp"] || got["auth-webapp"]) {
			t.Fatalf("saved selection = %v", got)
		}
	}
}

func TestMailpitBrowserRowCannotLaunchAnotherProcess(t *testing.T) {
	dir := t.TempDir()
	procs := buildProcs(dir, dir, projectJSON{Projects: []projectSpec{
		{Name: "auth-webapp", Kind: "app", Frontend: "http://localhost:5100/"},
		{Name: "mailpit", Kind: "browser", Frontend: "http://localhost:8025/"},
	}})
	m := newModel(dir, dir, procs)
	m.selected = len(procs) - 1
	if !browserSelection(dir)["mailpit"] {
		t.Fatal("Mailpit should be selected by default")
	}
	for _, key := range []rune{'s', 'r', 'b'} {
		_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{key}})
		if cmd != nil {
			t.Fatalf("browser-only row acts on %c", key)
		}
	}
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	if cmd == nil {
		t.Fatal("x must forget the Mailpit page")
	}
	updated, _ := m.Update(browserActionMsg{id: "mailpit", action: "disable", runAction: "stop"})
	next := updated.(model)
	if st, _, _ := next.selectedProc().snapshot(); st == stateRunning || st == stateStarting {
		t.Fatal("browser-only row must never start a process")
	}
}

func TestBrowserHelpHasFiveLanguages(t *testing.T) {
	for _, code := range []string{"pt-BR", "en-US", "es", "fr", "de"} {
		for _, key := range []string{"browser_on", "browser_off", "browser_failed", "help", "freeing_port", "freed_port", "port_free", "free_port_fail"} {
			if messages[code][key] == "" {
				t.Fatalf("missing %s/%s", code, key)
			}
		}
	}
}

func TestRememberedAppsControlNextStartup(t *testing.T) {
	dir := t.TempDir()
	cfg := projectJSON{Projects: []projectSpec{
		{Name: "auth-api", Kind: "service", Autostart: true},
		{Name: "auth-webapp", Kind: "app", Frontend: "http://localhost:5100/", Autostart: true},
		{Name: "forms-webapp", Kind: "app", Frontend: "http://localhost:5106/", Autostart: true},
	}}
	check := func(wantAuth, wantForms bool) {
		for _, p := range buildProcs(dir, dir, cfg) {
			if p.id == "auth-api" && !p.autostart {
				t.Fatal("app choices must not disable APIs")
			}
			if p.id == "auth-webapp" && p.autostart != wantAuth {
				t.Fatalf("auth app autostart = %v", p.autostart)
			}
			if p.id == "forms-webapp" && p.autostart != wantForms {
				t.Fatalf("forms app autostart = %v", p.autostart)
			}
		}
	}
	check(true, false)
	if err := os.WriteFile(filepath.Join(dir, "settings"), []byte("browser_apps=forms-webapp\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	check(false, true)
	if err := os.WriteFile(filepath.Join(dir, "settings"), []byte("browser_apps=\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	check(false, false)
}

func TestPanelStartStopPersistsAppChoicesAcrossRuns(t *testing.T) {
	dir := t.TempDir()
	data, err := os.ReadFile(filepath.Join("..", "browser-dev.mjs"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "bin"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "bin", "browser-dev.mjs"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "projects.json"), []byte(`{"projects":[{"name":"forms-webapp","kind":"app","frontend":"http://localhost:5106/"}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	p := &proc{id: "forms-webapp", name: "Forms", kind: "app", frontend: "http://localhost:5106/", cwd: dir,
		args: []string{"node", "-e", "setInterval(() => {}, 1000)"}}
	t.Cleanup(p.stop)
	m := newModel(dir, dir, []*proc{p})
	wait := func(state procState) {
		deadline := time.Now().Add(3 * time.Second)
		for time.Now().Before(deadline) {
			p.mu.Lock()
			ok := p.state == state && (state != stateStopped || p.cmdP == nil)
			p.mu.Unlock()
			if ok {
				return
			}
			time.Sleep(5 * time.Millisecond)
		}
		t.Fatal("app did not reach the expected state")
	}
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
	msg := cmd().(browserActionMsg)
	if msg.err != nil || msg.runAction != "start" {
		t.Fatalf("start = %+v", msg)
	}
	updated, _ := m.Update(msg)
	m = updated.(model)
	wait(stateRunning)
	if !browserSelection(dir)[p.id] || !p.autostart {
		t.Fatal("starting an app must remember it")
	}
	// Normal whole-run cleanup must leave it selected for next time.
	m.stopAll()
	if !browserSelection(dir)[p.id] {
		t.Fatal("shutdown discarded a saved app choice")
	}
	_, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
	updated, _ = m.Update(cmd())
	m = updated.(model)
	wait(stateRunning)
	_, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
	msg = cmd().(browserActionMsg)
	if msg.err != nil || msg.runAction != "stop" {
		t.Fatalf("stop = %+v", msg)
	}
	updated, _ = m.Update(msg)
	m = updated.(model)
	wait(stateStopped)
	if browserSelection(dir)[p.id] || p.autostart {
		t.Fatal("explicit app stop must remove next-run selection")
	}
	_, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	msg = cmd().(browserActionMsg)
	if msg.err != nil || msg.runAction != "restart" {
		t.Fatalf("restart = %+v", msg)
	}
	updated, _ = m.Update(msg)
	m = updated.(model)
	wait(stateRunning)
	if !browserSelection(dir)[p.id] {
		t.Fatal("restarting an app must remember it")
	}
	_, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	updated, _ = m.Update(cmd())
	m = updated.(model)
	wait(stateStopped)
	if browserSelection(dir)[p.id] {
		t.Fatal("x must forget the explicitly stopped app")
	}
}

func TestAppsListGroupsNonCoreProcessesAfterFrontends(t *testing.T) {
	dir := t.TempDir()
	setLang("en-US")
	procs := buildProcs(dir, dir, projectJSON{Projects: []projectSpec{
		{Name: "projects-api", Kind: "service", Display: "Projects"},
		{Name: "forms-api", Kind: "service", Group: "forms", Display: "Forms API"},
		{Name: "auth-api", Kind: "service", Display: "Auth"},
		{Name: "auth-webapp", Kind: "app", Display: "Auth Web", Frontend: "http://localhost:5100/"},
		{Name: "forms-webapp", Kind: "app", Group: "forms", Display: "Forms Web", Frontend: "http://localhost:5106/"},
		{Name: "mailpit", Kind: "browser", Display: "Mailpit", Frontend: "http://127.0.0.1:8025/"},
	}})
	var order []string
	for _, p := range procs {
		order = append(order, p.id)
	}
	want := []string{"infrastructure", "projects-api", "auth-api", "auth-webapp", "mailpit", "forms-api", "forms-webapp"}
	if !reflect.DeepEqual(order, want) {
		t.Fatalf("order = %v, want %v", order, want)
	}
	m := newModel(dir, dir, procs)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	view := stripANSI(updated.(model).View())
	core := strings.Index(view, tr("section_api"))
	web := strings.Index(view, tr("section_web"))
	apps := strings.Index(view, tr("section_apps"))
	forms := strings.Index(view, "Forms API")
	if core < 0 || web < core || apps < web || forms < apps || !strings.Contains(view, "  Forms\n") && !strings.Contains(view, "  Forms ") {
		t.Fatalf("sections out of order:\n%s", view)
	}
}
