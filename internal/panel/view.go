package panel

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/ipalpha-dev/tooling/internal/catalog"
	"github.com/ipalpha-dev/tooling/internal/i18n"
	"github.com/ipalpha-dev/tooling/internal/run"
	"github.com/ipalpha-dev/tooling/internal/ui"
)

var (
	listBox  = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("240")).Padding(0, 1)
	logBox   = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(ui.Blue).Padding(0, 1)
	logFocus = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(ui.Accent).Padding(0, 1)
	cardBox  = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(ui.Red).Padding(0, 1)
	selStyle = lipgloss.NewStyle().Foreground(ui.Accent).Bold(true)
	helpBox  = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(ui.Blue).Padding(1, 2)
)

// dot returns the status glyph of a row.
func dot(p *run.Proc) string {
	st, code, known, ready, crash := p.Snapshot()
	if p.Spec.Kind == run.KindInfra {
		switch p.Health() {
		case "healthy", "running":
			return ui.OK.Render("●")
		case "starting", "created", "restarting":
			return ui.Warn.Render("◐")
		case "unhealthy", "exited", "dead", "stopped":
			return ui.Bad.Render("✖")
		}
		return ui.Muted.Render("○")
	}
	switch st {
	case run.Running:
		if known && ready {
			return ui.OK.Render("●")
		}
		return ui.Warn.Render("◐")
	case run.Starting:
		return ui.Warn.Render("◐")
	case run.Exited:
		if code != 0 || crash != nil {
			return ui.Bad.Render("✖")
		}
	}
	if p.Spec.Missing != "" {
		return ui.Muted.Render("·")
	}
	return ui.Muted.Render("○")
}

func statusText(p *run.Proc) string {
	st, code, known, ready, _ := p.Snapshot()
	if p.Spec.Kind == run.KindInfra {
		if h := p.Health(); h != "" {
			return i18n.T("health_" + h)
		}
		return i18n.T("state_unknown")
	}
	if p.Spec.Missing != "" {
		return p.Spec.Missing
	}
	switch st {
	case run.Running:
		if !known {
			return i18n.T("state_starting")
		}
		if !ready {
			return i18n.T("state_not_ready")
		}
		return i18n.T("state_ready")
	case run.Starting:
		return i18n.T("state_starting")
	case run.Exited:
		if code != 0 {
			return i18n.T("state_exit", code)
		}
		return i18n.T("state_exited")
	}
	return i18n.T("state_stopped")
}

// View renders the panel.
func (m *Model) View() string {
	if m.done {
		return ui.OK.Render(i18n.T("panel_all_stopped")) + "\n"
	}
	if !m.ready {
		return i18n.T("panel_loading") + "\n"
	}
	if m.plainSel {
		return ui.Muted.Render(i18n.T("panel_select_on")) + "\n" + m.vp.View() + "\n"
	}
	if m.help {
		return m.helpView()
	}
	listW, logW, bodyH, vpW, _ := m.layout()

	var lines []string
	prev := ""
	for i, p := range m.procs {
		sec := section(p)
		if sec != prev {
			if prev != "" {
				lines = append(lines, "")
			}
			switch {
			case sec == "infra":
				lines = append(lines, ui.Muted.Render(i18n.T("panel_section_infra")))
			case sec == "api":
				lines = append(lines, ui.Muted.Render(i18n.T("panel_section_core")))
			case sec == "web":
				lines = append(lines, ui.Muted.Render(i18n.T("panel_section_web")))
			default:
				if !strings.HasPrefix(prev, "app:") {
					lines = append(lines, ui.Muted.Render(i18n.T("panel_section_apps")))
				}
				lines = append(lines, ui.Muted.Render("  "+catalog.AppGroupTitle(p.Spec.Group)))
			}
			prev = sec
		}
		label := p.Spec.Name
		indent := ""
		if p.Spec.Group != "" {
			indent = "  "
		}
		max := listW - 7 - len(indent)
		if max < 6 {
			max = 6
		}
		label = ansi.Truncate(label, max, "…")
		st, _, _, _, crash := p.Snapshot()
		failed := st == run.Exited && crash != nil
		switch {
		case i == m.sel && !m.focusLog:
			line := selStyle.Render("› ") + indent + dot(p) + " "
			if failed {
				line += ui.Bad.Render(label)
			} else {
				line += selStyle.Render(label)
			}
			lines = append(lines, line)
		case i == m.sel:
			lines = append(lines, ui.Muted.Render("› ")+indent+dot(p)+" "+label)
		case failed:
			lines = append(lines, "  "+indent+dot(p)+" "+ui.Bad.Render(label))
		default:
			lines = append(lines, "  "+indent+dot(p)+" "+label)
		}
	}

	inner := bodyH - 2
	selRow := 0
	for i, r := range m.listRows() {
		if r == m.sel {
			selRow = i
		}
	}
	if selRow < m.listOffset {
		m.listOffset = selRow
	}
	if selRow >= m.listOffset+inner {
		m.listOffset = selRow - inner + 1
	}
	if m.listOffset > len(lines)-inner {
		m.listOffset = max(0, len(lines)-inner)
	}
	visible := lines[m.listOffset:]
	if len(visible) > inner {
		visible = visible[:inner]
	}
	listText := strings.Join(visible, "\n")

	p := m.selected()
	var right strings.Builder
	if p != nil {
		head := ui.Bold.Render(p.Spec.Name) + "  " + statusText(p)
		if p.Spec.Port > 0 {
			head += ui.Muted.Render("  ·  :" + strconv.Itoa(p.Spec.Port))
		}
		if up := Uptime(p.Uptime()); up != "" && p.Spec.Kind != run.KindInfra {
			head += ui.Muted.Render("  ·  " + up)
		}
		right.WriteString(ansi.Truncate(head, vpW, "…") + "\n")
		sub := ""
		if p.Spec.URL != "" {
			sub = ui.Link.Render(p.Spec.URL)
		} else if p.Spec.ReadyURL != "" {
			sub = ui.Muted.Render(p.Spec.ReadyURL)
		}
		if m.follow {
			sub += ui.Muted.Render("  · " + i18n.T("panel_following"))
		}
		right.WriteString(ansi.Truncate(sub, vpW, "…") + "\n")
		if _, _, _, _, crash := p.Snapshot(); crash != nil {
			right.WriteString(m.card(p, crash, vpW) + "\n")
		}
	}
	right.WriteString(m.vp.View())

	lb := logBox
	if m.focusLog {
		lb = logFocus
	}
	left := listBox.Width(listW - 2).Height(inner).MaxHeight(bodyH).Render(listText)
	rightR := lb.Width(logW - 2).Height(inner).MaxHeight(bodyH).Render(right.String())
	body := lipgloss.JoinHorizontal(lipgloss.Top, left, rightR)

	title := ui.Title.Render("IPAlpha · ./run")
	failed, ready, total := 0, 0, 0
	for _, pr := range m.procs {
		if pr.Spec.Kind == run.KindInfra {
			continue
		}
		st, _, _, _, crash := pr.Snapshot()
		if st == run.Running || st == run.Starting {
			total++
			if pr.Ready() {
				ready++
			}
		}
		if st == run.Exited && crash != nil {
			failed++
		}
	}
	summary := ui.Muted.Render(fmt.Sprintf("  %s %d/%d", i18n.T("panel_ready_count"), ready, total))
	if failed > 0 {
		summary += "  " + ui.Bad.Render(i18n.T("panel_failed_count", failed))
	}
	if m.quitting {
		summary += "  " + ui.Warn.Render(i18n.T("panel_shutting_down"))
	}
	if m.status != "" && time.Since(m.statusAt) < 8*time.Second {
		summary += "  " + ui.Muted.Render(ansi.Truncate(m.status, m.width/2, "…"))
	}
	top := title + summary
	if len(m.sess.Notes) > 0 {
		top += "\n" + ui.Warn.Render(ansi.Truncate("! "+m.notesLine(), m.width-1, "…"))
	}
	help := ui.Muted.Render(ansi.Truncate(i18n.T("panel_keys"), m.width-1, "…"))
	return top + "\n" + body + "\n" + help
}

// notesLine summarizes the session notes in one line (details are in the ? help screen).
func (m *Model) notesLine() string {
	moved := len(m.sess.Plan.Moves)
	var other []string
	for _, n := range m.sess.Notes {
		if !strings.Contains(n, "→") && !isMoveNote(m, n) {
			other = append(other, n)
		}
	}
	var parts []string
	if moved == 1 {
		parts = append(parts, m.sess.Notes[0])
	} else if moved > 1 {
		parts = append(parts, i18n.T("panel_ports_moved", moved))
	}
	parts = append(parts, other...)
	return strings.Join(parts, "  ·  ")
}

func isMoveNote(m *Model, n string) bool {
	for _, mv := range m.sess.Plan.Moves {
		if strings.HasPrefix(n, mv.Key+":") {
			return true
		}
	}
	return false
}

func (m *Model) card(p *run.Proc, c *run.Crash, w int) string {
	var b strings.Builder
	b.WriteString(ui.Bad.Render("✖ "+c.Summary) + "\n")
	for _, l := range c.Lines {
		b.WriteString(ui.Muted.Render("│ ") + ansi.Truncate(l, w-6, "…") + "\n")
	}
	if c.Fix != "" {
		b.WriteString("→ " + ansi.Wordwrap(c.Fix, w-6, " "))
	}
	return cardBox.Width(w - 2).Render(strings.TrimRight(b.String(), "\n"))
}

func (m *Model) helpView() string {
	rows := [][2]string{
		{"↑/↓  j/k", i18n.T("help_select")},
		{"Tab  ←/→", i18n.T("help_focus")},
		{"s", i18n.T("help_toggle")},
		{"r", i18n.T("help_restart")},
		{"x", i18n.T("help_stop")},
		{"o", i18n.T("help_open")},
		{"p", i18n.T("help_port")},
		{"K", i18n.T("help_kill")},
		{"e", i18n.T("help_copy_error")},
		{"y", i18n.T("help_copy_log")},
		{"L", i18n.T("help_log_file")},
		{"c", i18n.T("help_clear")},
		{"f  G", i18n.T("help_follow")},
		{"PgUp/PgDn", i18n.T("help_scroll")},
		{"m", i18n.T("help_mouse")},
		{"q", i18n.T("help_quit")},
	}
	var b strings.Builder
	b.WriteString(ui.Bold.Foreground(ui.Accent).Render(i18n.T("help_title")) + "\n\n")
	for _, r := range rows {
		b.WriteString(fmt.Sprintf("%-12s %s\n", ui.Bold.Render(r[0]), r[1]))
	}
	b.WriteString("\n" + ui.Muted.Render(i18n.T("help_legend")) + "\n")
	if len(m.sess.Notes) > 0 {
		b.WriteString("\n" + ui.Warn.Render(i18n.T("help_notes")) + "\n")
		for _, n := range m.sess.Notes {
			b.WriteString(ui.Muted.Render("  "+n) + "\n")
		}
	}
	b.WriteString(ui.Muted.Render(i18n.T("help_logs_dir", m.ctl.LogDir)) + "\n\n")
	b.WriteString(ui.Muted.Render(i18n.T("help_close")))
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, helpBox.Render(b.String()))
}
