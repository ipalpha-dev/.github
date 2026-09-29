package main

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

var (
	titleStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("15")).Background(lipgloss.Color("33")).Padding(0, 1)
	listBorder = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("240")).Padding(0, 1)
	logBorder  = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("33")).Padding(0, 1)
	logFocus   = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("39")).Padding(0, 1)
	selStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("39")).Bold(true)
	runStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	waitStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
	exitStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Bold(true)
	stopStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	muted      = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	helpStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	failBg     = lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Bold(true)
)

func (m model) View() string {
	if m.shutdownDone {
		return exitStyle.Render(tr("all_stopped")) + "\n"
	}
	if !m.ready {
		return tr("loading") + "\n"
	}

	if m.selectMode {
		return muted.Render(tr("select_on")) + "\n" + m.vp.View() + "\n"
	}

	listW, logW, listH, _, _ := m.layout()

	var list strings.Builder
	listTitle := tr("processes")
	if !m.focusLog {
		listTitle = tr("processes") + "  ·  " + tr("focus")
	}
	list.WriteString(muted.Render(listTitle) + "\n")
	prevKind := ""
	for i, p := range m.procs {
		section := p.kind
		if section == "attached" {
			section = "service"
		}
		if section != prevKind {
			switch section {
			case "service":
				if prevKind != "" {
					list.WriteString("\n")
				}
				list.WriteString(muted.Render(tr("section_api")) + "\n")
			case "app":
				list.WriteString("\n")
				list.WriteString(muted.Render(tr("section_web")) + "\n")
			}
			prevKind = section
		}

		st, code, _ := p.snapshot()
		var dotS string
		switch st {
		case stateRunning:
			if known, ready := p.isReady(); known && !ready {
				dotS = waitStyle.Render("◐")
			} else {
				dotS = runStyle.Render("●")
			}
		case stateStarting, stateWaiting:
			dotS = waitStyle.Render("◐")
		case stateExited:
			if code != 0 {
				dotS = exitStyle.Render("✖")
			} else {
				dotS = stopStyle.Render("○")
			}
		default:
			dotS = stopStyle.Render("○")
		}

		label := p.name
		indent := ""
		if p.kind == "attached" {
			indent = "  "
		}
		if st == stateExited && code != 0 {
			label = fmt.Sprintf("%s  %s %d", label, tr("exit"), code)
		} else if st == stateWaiting && len(p.deps) > 0 {
			label = fmt.Sprintf("%s  …", label)
		}

		maxName := listW - 8
		if maxName < 8 {
			maxName = 8
		}
		if len([]rune(label)) > maxName {
			r := []rune(label)
			label = string(r[:maxName-1]) + "…"
		}

		var line string
		if i == m.selected {
			if st == stateExited && code != 0 {
				line = exitStyle.Render("› ") + indent + dotS + " " + exitStyle.Render(label)
			} else {
				line = selStyle.Render("› ") + indent + dotS + " " + selStyle.Render(label)
			}
		} else if st == stateExited && code != 0 {
			line = "  " + indent + dotS + " " + failBg.Render(label)
		} else {
			line = "  " + indent + dotS + " " + label
		}
		list.WriteString(line + "\n")
	}
	innerListH := listH - 1
	if innerListH < 1 {
		innerListH = 1
	}
	lines := strings.Count(list.String(), "\n")
	for lines < innerListH {
		list.WriteString("\n")
		lines++
	}

	p := m.selectedProc()
	header := "LOGS"
	if p != nil {
		header = fmt.Sprintf("%s  ·  %s", p.name, p.statusText())
		if p.port != "" {
			header += "  ·  :" + p.port
		}
		if all := append(append([]string{}, p.deps...), p.softDeps...); len(all) > 0 {
			header += "  ·  " + tr("uses") + ": " + strings.Join(all, ", ")
		}
		if m.follow {
			header += "  ·  " + tr("follow")
		}
		if m.focusLog {
			header += "  ·  " + tr("focus")
		}
	}

	_, _, _, vpW, _ := m.layout()
	if r := []rune(header); len(r) > vpW {
		header = string(r[:vpW-1]) + "…"
	}
	left := listBorder.Width(listW).Height(listH).Render(list.String())
	rightBody := muted.Render(header) + "\n" + m.vp.View()
	rb := logBorder
	if m.focusLog {
		rb = logFocus
	}
	right := rb.Width(logW).Height(listH).Render(rightBody)

	body := lipgloss.JoinHorizontal(lipgloss.Top, left, right)
	help := helpStyle.Render(tr("help"))
	status := ""
	if m.status != "" {
		status = "  " + muted.Render(m.status)
	}
	failCount := 0
	for _, pr := range m.procs {
		if pr.failed() {
			failCount++
		}
	}
	if m.quitting {
		status = "  " + exitStyle.Render(tr("shutting_down")) + status
	} else if failCount > 0 {
		status = "  " + exitStyle.Render(fmt.Sprintf("%d %s", failCount, tr("failed"))) + status
	}

	return titleStyle.Render(tr("title")) + status + "\n" + body + "\n" + help + "\n"
}
