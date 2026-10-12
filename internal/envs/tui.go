package envs

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/ipalpha-dev/tooling/internal/i18n"
	"github.com/ipalpha-dev/tooling/internal/ui"
)

// Choice is what the developer picked; the caller runs it after the picker has closed the screen.
type Choice struct {
	Kind Kind
	Row  Row
}

type refreshMsg []Row
type tickMsg time.Time

type model struct {
	rows    []Row
	gather  func() []Row
	filter  string
	sel     int
	width   int
	height  int
	choice  *Choice
	loading bool
}

var (
	selStyle  = lipgloss.NewStyle().Foreground(ui.Accent).Bold(true)
	headStyle = lipgloss.NewStyle().Foreground(ui.Grey)
)

// Pick opens the picker. gather is called again every few seconds and on F5.
func Pick(rows []Row, gather func() []Row) (*Choice, error) {
	m := &model{rows: rows, gather: gather}
	for i, r := range rows {
		if r.Current {
			m.sel = i
		}
	}
	final, err := tea.NewProgram(m, tea.WithAltScreen()).Run()
	if err != nil {
		return nil, err
	}
	return final.(*model).choice, nil
}

func (m *model) Init() tea.Cmd { return tick() }

func tick() tea.Cmd { return tea.Tick(6*time.Second, func(t time.Time) tea.Msg { return tickMsg(t) }) }

func (m *model) refresh() tea.Cmd {
	if m.loading {
		return nil
	}
	m.loading = true
	return func() tea.Msg { return refreshMsg(m.gather()) }
}

// visible rows after the filter.
func (m *model) visible() []Row { return Filter(m.rows, m.filter) }

// Filter keeps the rows whose name, branches or agents contain every word of q.
func Filter(rows []Row, q string) []Row {
	words := strings.Fields(strings.ToLower(q))
	if len(words) == 0 {
		return rows
	}
	var out []Row
	for _, r := range rows {
		hay := strings.ToLower(r.Name + " " + r.Branches + " " + r.AgentsText())
		ok := true
		for _, w := range words {
			if !strings.Contains(hay, w) {
				ok = false
				break
			}
		}
		if ok {
			out = append(out, r)
		}
	}
	return out
}

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
	case tickMsg:
		return m, tea.Batch(m.refresh(), tick())
	case refreshMsg:
		m.loading = false
		name := ""
		if v := m.visible(); m.sel < len(v) {
			name = v[m.sel].Name
		}
		m.rows = []Row(msg)
		m.sel = 0
		for i, r := range m.visible() {
			if r.Name == name {
				m.sel = i
			}
		}
	case tea.KeyMsg:
		return m.key(msg)
	}
	return m, nil
}

func (m *model) key(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	v := m.visible()
	pick := func(kind Kind) (tea.Model, tea.Cmd) {
		if m.sel < len(v) {
			m.choice = &Choice{Kind: kind, Row: v[m.sel]}
			return m, tea.Quit
		}
		return m, nil
	}
	switch k.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc":
		if m.filter != "" {
			m.filter, m.sel = "", 0
			return m, nil
		}
		return m, tea.Quit
	case "up", "ctrl+p":
		if m.sel > 0 {
			m.sel--
		}
	case "down", "ctrl+n":
		if m.sel < len(v)-1 {
			m.sel++
		}
	case "enter":
		return pick(ActHop)
	case "ctrl+t":
		return pick(ActShell)
	case "ctrl+l":
		return pick(ActLogs)
	case "ctrl+r":
		return pick(ActRun)
	case "f5", "ctrl+g":
		return m, m.refresh()
	case "backspace":
		if r := []rune(m.filter); len(r) > 0 {
			m.filter, m.sel = string(r[:len(r)-1]), 0
		}
	default:
		if k.Type == tea.KeyRunes || k.Type == tea.KeySpace {
			m.filter += string(k.Runes)
			if k.Type == tea.KeySpace {
				m.filter += " "
			}
			m.sel = 0
		}
	}
	return m, nil
}

func (m *model) View() string {
	w := m.width
	if w <= 0 {
		w = ui.Width()
	}
	var b strings.Builder
	b.WriteString(ui.Title.Render("IPAlpha · "+i18n.T("envs_title")) + "\n\n")
	b.WriteString(selStyle.Render("› ") + m.filter + headStyle.Render("▏") + "\n")
	if m.filter == "" {
		b.WriteString(headStyle.Render("  "+i18n.T("envs_filter_hint")) + "\n")
	} else {
		b.WriteString("\n")
	}
	b.WriteString(headStyle.Render(ansi.Truncate("  "+HeaderLine(), w, "…")) + "\n")
	v := m.visible()
	if len(v) == 0 {
		b.WriteString("  " + ui.Muted.Render(i18n.T("envs_no_match")) + "\n")
	}
	for i, r := range v {
		line := RowLine(r)
		if i == m.sel {
			b.WriteString(selStyle.Render("▸ ") + ansi.Truncate(line, w-2, "…") + "\n")
		} else {
			b.WriteString("  " + ansi.Truncate(line, w-2, "…") + "\n")
		}
	}
	b.WriteString("\n" + headStyle.Render(ansi.Truncate(i18n.T("envs_keys"), w, "…")))
	return b.String()
}

// column widths shared by the picker and --list.
const (
	wName = 18
	wRun  = 34
	wTmux = 5
)

// HeaderLine names the columns.
func HeaderLine() string {
	return fmt.Sprintf("  %-*s %-*s %-*s %s", wName, i18n.T("envs_h_env"), wRun, i18n.T("envs_h_run"), wTmux, "tmux",
		i18n.T("envs_h_rest"))
}

// RowLine is one environment in plain text (the picker colors the dot only).
func RowLine(r Row) string {
	dot := ui.Muted.Render("○")
	switch {
	case r.Run.Owner != 0 && r.Run.Up > 0:
		dot = ui.OK.Render("●")
	case r.Run.Owner != 0:
		dot = ui.Warn.Render("◐")
	}
	name := r.Name
	if r.Current {
		name += " *"
	}
	return dot + " " + padTo(name, wName) + " " + padTo(RunText(r), wRun) + " " + padTo(SessionMark(r), wTmux) + " " + RestText(r)
}

func padTo(s string, n int) string {
	if w := ansi.StringWidth(s); w < n {
		return s + strings.Repeat(" ", n-w)
	}
	return ansi.Truncate(s, n, "…")
}

// RunText is "12/20 http://localhost:5110" or "stopped :5110".
func RunText(r Row) string {
	if r.Run.Owner != 0 {
		return fmt.Sprintf("%s %s", i18n.T("envs_run_up", r.Run.Up, r.Run.Total), r.Run.URL)
	}
	return i18n.T("envs_run_stopped")
}

// SessionMark shows whether the tmux session exists.
func SessionMark(r Row) string {
	if r.Session {
		return "yes"
	}
	return "-"
}

// RestText: branches, changes, agents, requests.
func RestText(r Row) string {
	parts := []string{r.Branches}
	if r.Dirty > 0 || r.Ahead > 0 {
		parts = append(parts, fmt.Sprintf("±%d ↑%d", r.Dirty, r.Ahead))
	}
	if a := r.AgentsText(); a != "" {
		parts = append(parts, "["+a+"]")
	}
	if r.Requests > 0 {
		parts = append(parts, i18n.T("envs_requests", r.Requests))
	}
	return strings.Join(parts, "  ")
}

// PlainTable is `./envs --list`: one line per environment, no colors, tab-friendly columns.
func PlainTable(rows []Row) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%-*s %-*s %-*s %s\n", wName, i18n.T("envs_h_env"), wRun, i18n.T("envs_h_run"), wTmux, "tmux", i18n.T("envs_h_rest"))
	for _, r := range rows {
		name := r.Name
		if r.Current {
			name += " *"
		}
		fmt.Fprintf(&b, "%-*s %-*s %-*s %s\n", wName, name, wRun, RunText(r), wTmux, SessionMark(r), RestText(r))
	}
	return b.String()
}
