package panel

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/ipalpha-dev/tooling/internal/catalog"
	"github.com/ipalpha-dev/tooling/internal/i18n"
	"github.com/ipalpha-dev/tooling/internal/ports"
	"github.com/ipalpha-dev/tooling/internal/run"
	"github.com/ipalpha-dev/tooling/internal/workspace"
)

func fakeModel(t *testing.T, notes int) *Model {
	t.Helper()
	var specs []*run.Spec
	for _, s := range []string{"mongo", "redis", "rabbitmq", "mailpit"} {
		specs = append(specs, &run.Spec{ID: s, Name: s, Kind: run.KindInfra, Port: 1})
	}
	for _, r := range catalog.Repos {
		if !r.Runs() {
			continue
		}
		k := run.KindAPI
		if r.Kind == catalog.KindWeb {
			k = run.KindWeb
		}
		specs = append(specs, &run.Spec{ID: r.Name, Name: r.Label(), Kind: k, Group: r.App, Port: r.Port, URL: "http://localhost:1/"})
	}
	sess := &run.Session{W: &workspace.Workspace{Root: t.TempDir(), Settings: workspace.NewSettings()}, Specs: specs,
		Plan: ports.Plan{Actual: map[string]int{}}}
	for i := 0; i < notes; i++ {
		sess.Notes = append(sess.Notes, "x: port 1 is busy, using 2 this run")
		sess.Plan.Moves = append(sess.Plan.Moves, ports.Move{Key: "x"})
	}
	ctl := run.NewController(sess)
	m := &Model{sess: sess, ctl: ctl, procs: ctl.Procs, follow: true, events: ctl.Events}
	long := strings.Repeat("a very long log line that needs wrapping ", 8)
	for _, p := range ctl.Procs {
		for i := 0; i < 300; i++ {
			p.Note(long)
		}
	}
	return m
}

// The view must never be taller or wider than the terminal (it would scroll and garble the panel).
func TestViewFitsTerminal(t *testing.T) {
	for _, notes := range []int{0, 3} {
		for _, size := range [][2]int{{80, 24}, {100, 30}, {140, 40}, {200, 60}, {60, 20}} {
			m := fakeModel(t, notes)
			m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
			for _, sel := range []int{0, 5, len(m.procs) - 1} {
				m.sel = sel
				m.refresh(true)
				v := m.View()
				if h := lipgloss.Height(v); h > size[1] {
					t.Errorf("%dx%d notes=%d sel=%d: view is %d lines tall", size[0], size[1], notes, sel, h)
				}
				for i, line := range strings.Split(v, "\n") {
					if w := lipgloss.Width(line); w > size[0] {
						t.Errorf("%dx%d: line %d is %d wide", size[0], size[1], i, w)
						break
					}
				}
			}
		}
	}
}

func TestPanelSectionsUseCoreFrontendsGroupedAppsThenInfrastructure(t *testing.T) {
	m := fakeModel(t, 0)
	defer m.ctl.StopAll()
	m.Update(tea.WindowSizeMsg{Width: 160, Height: 120})
	view := m.View()
	last := -1
	for _, key := range []string{"panel_section_core", "panel_section_web", "panel_section_apps", "panel_section_infra"} {
		position := strings.Index(view, i18n.T(key))
		if position < 0 || position <= last {
			t.Fatalf("section %s is missing or out of order", key)
		}
		last = position
	}
	groups := map[string]int{}
	previous := ""
	for _, proc := range m.procs {
		current := section(proc)
		if strings.HasPrefix(current, "app:") && current != previous {
			groups[current]++
		}
		previous = current
	}
	for group, count := range groups {
		if count != 1 {
			t.Fatalf("app %s appears in %d separate groups", group, count)
		}
	}
}

// Clicking a row selects the process printed on it.
func TestRowAt(t *testing.T) {
	m := fakeModel(t, 0)
	m.Update(tea.WindowSizeMsg{Width: 140, Height: 60})
	v := m.View()
	lines := strings.Split(v, "\n")
	for y, line := range lines {
		i := m.rowAt(y)
		if i < 0 {
			continue
		}
		if !strings.Contains(line, m.procs[i].Spec.Name) {
			t.Errorf("row %d maps to %s but shows %q", y, m.procs[i].Spec.Name, line)
		}
	}
}
