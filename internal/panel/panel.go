// Package panel is the ./run process panel: one row per infra service, API and web app, live logs,
// readiness, a crash card that explains failures, and keys to start/stop/restart/open.
package panel

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/ipalpha-dev/tooling/internal/i18n"
	"github.com/ipalpha-dev/tooling/internal/run"
	"github.com/ipalpha-dev/tooling/internal/sys"
	"github.com/ipalpha-dev/tooling/internal/ui"
)

type tickMsg time.Time
type eventMsg run.Event
type stopStep int
type quitNow struct{}
type statusMsg string

// Model is the bubbletea model of the panel.
type Model struct {
	sess       *run.Session
	ctl        *run.Controller
	procs      []*run.Proc
	sel        int
	width      int
	height     int
	ready      bool
	vp         viewport.Model
	follow     bool
	focusLog   bool
	help       bool
	status     string
	statusAt   time.Time
	quitting   bool
	done       bool
	lastKey    string
	events     chan run.Event
	wrapped    []string
	owner      []int
	selA       int
	selB       int
	hasSel     bool
	dragging   bool
	plainSel   bool
	listOffset int
}

// Run opens the panel and blocks until the developer quits.
func Run(sess *run.Session, ctl *run.Controller) error {
	stopOnSignal(ctl)
	m := &Model{sess: sess, ctl: ctl, procs: ctl.Procs, follow: true, events: ctl.Events}
	// Start on the first service: infra logs are noise unless something is wrong there.
	for i, p := range ctl.Procs {
		if p.Spec.Kind != run.KindInfra {
			m.sel = i
			break
		}
	}
	p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseCellMotion())
	_, err := p.Run()
	ctl.StopAll()
	return err
}

// Init starts autostart processes and the ticker.
func (m *Model) Init() tea.Cmd {
	return tea.Batch(func() tea.Msg { m.ctl.StartAuto(); return nil }, m.listen(), tick())
}

func (m *Model) listen() tea.Cmd {
	return func() tea.Msg {
		e, ok := <-m.events
		if !ok {
			return nil
		}
		return eventMsg(e)
	}
}

func tick() tea.Cmd {
	return tea.Tick(250*time.Millisecond, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func (m *Model) selected() *run.Proc {
	if m.sel < 0 || m.sel >= len(m.procs) {
		return nil
	}
	return m.procs[m.sel]
}

func (m *Model) say(s string) {
	m.status = s
	m.statusAt = time.Now()
}

func listWidth(total int) int {
	switch {
	case total >= 120:
		return 38
	case total >= 100:
		return 34
	case total >= 80:
		return 30
	default:
		return 26
	}
}

func (m *Model) layout() (listW, logW, bodyH, vpW, vpH int) {
	listW = listWidth(m.width)
	logW = m.width - listW - 1
	if logW < 30 {
		logW = 30
	}
	bodyH = m.height - 3
	if len(m.sess.Notes) > 0 {
		bodyH--
	}
	if bodyH < 8 {
		bodyH = 8
	}
	vpW = logW - 4
	if vpW < 10 {
		vpW = 10
	}
	vpH = bodyH - 4 - m.cardHeight(vpW)
	if vpH < 3 {
		vpH = 3
	}
	return
}

// Update handles keys, mouse, ticks and process events.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		_, _, _, vpW, vpH := m.layout()
		if !m.ready {
			m.vp = viewport.New(vpW, vpH)
			m.vp.MouseWheelEnabled = true
			m.ready = true
		} else {
			m.vp.Width, m.vp.Height = vpW, vpH
		}
		m.refresh(true)
		return m, nil
	case eventMsg:
		if p := m.selected(); p != nil && p.Spec.ID == msg.ID {
			m.refresh(msg.Exit)
		}
		if msg.Exit {
			m.ctl.OnExit(msg.ID)
		}
		return m, m.listen()
	case tickMsg:
		if m.ready {
			_, _, _, vpW, vpH := m.layout()
			m.vp.Width, m.vp.Height = vpW, vpH
		}
		m.refresh(false)
		return m, tick()
	case statusMsg:
		m.say(string(msg))
		return m, nil
	case stopStep:
		return m.stopStep(int(msg))
	case quitNow:
		return m, tea.Quit
	case tea.MouseMsg:
		return m.mouse(msg)
	case tea.KeyMsg:
		return m.key(msg)
	}
	return m, nil
}

func (m *Model) stopStep(i int) (tea.Model, tea.Cmd) {
	idx := len(m.procs) - 1 - i
	if idx >= 0 {
		p := m.procs[idx]
		if st, _, _, _, _ := p.Snapshot(); st == run.Running || st == run.Starting {
			m.say(i18n.T("panel_stopping", p.Spec.Name))
		}
		return m, func() tea.Msg { p.Stop(); return stopStep(i + 1) }
	}
	m.done = true
	return m, tea.Tick(200*time.Millisecond, func(time.Time) tea.Msg { return quitNow{} })
}

func (m *Model) do(fn func() string) tea.Cmd {
	return func() tea.Msg { return statusMsg(fn()) }
}

func (m *Model) key(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.help {
		m.help = false
		return m, nil
	}
	p := m.selected()
	switch k.String() {
	case "q", "ctrl+c":
		if m.quitting {
			m.ctl.StopAll()
			return m, tea.Quit
		}
		m.quitting = true
		m.say(i18n.T("panel_stopping_all"))
		return m, func() tea.Msg { return stopStep(0) }
	case "?":
		m.help = true
	case "tab", "right", "l":
		m.focusLog = true
	case "shift+tab", "left", "h":
		m.focusLog = false
	case "up", "k":
		if m.focusLog {
			m.follow = false
			m.vp.LineUp(1)
		} else if m.sel > 0 {
			m.sel--
			m.selChanged()
		}
	case "down", "j":
		if m.focusLog {
			m.vp.LineDown(1)
			m.follow = m.vp.AtBottom()
		} else if m.sel < len(m.procs)-1 {
			m.sel++
			m.selChanged()
		}
	case "pgup", "ctrl+u", "u":
		m.follow, m.focusLog = false, true
		m.vp.HalfViewUp()
	case "pgdown", "ctrl+d", "d":
		m.focusLog = true
		m.vp.HalfViewDown()
		m.follow = m.vp.AtBottom()
	case "home", "g":
		m.follow, m.focusLog = false, true
		m.vp.GotoTop()
	case "end", "G", "f":
		m.follow = true
		m.vp.GotoBottom()
	case "s":
		if p != nil {
			return m, m.do(func() string { return m.ctl.Toggle(p) })
		}
	case "r":
		if p != nil {
			return m, m.do(func() string { return m.ctl.Restart(p) })
		}
	case "x":
		if p != nil {
			return m, m.do(func() string { return m.ctl.StopRemember(p) })
		}
	case "p":
		if p != nil {
			return m, m.do(func() string { return m.ctl.FixPort(p) })
		}
	case "K":
		if p != nil {
			return m, m.do(func() string { return m.ctl.KillPortHolder(p) })
		}
	case "o":
		if p != nil {
			return m, m.do(func() string { return m.ctl.Open(p) })
		}
	case "c":
		if p != nil {
			p.Clear()
			m.lastKey = ""
			m.refresh(true)
			m.say(i18n.T("panel_cleared"))
		}
	case "y":
		if p != nil {
			lines := p.Lines()
			if err := sys.Clipboard(ansi.Strip(strings.Join(lines, "\n"))); err != nil {
				m.say(i18n.T("panel_copy_failed"))
			} else {
				m.say(i18n.T("panel_copied", len(lines)))
			}
		}
	case "e":
		if p != nil {
			return m, m.do(func() string { return m.ctl.CopyError(p) })
		}
	case "L":
		if p != nil {
			path := filepath.Join(m.ctl.LogDir, p.Spec.ID+".log")
			m.say(i18n.T("panel_log_at", path))
		}
	case "m":
		m.plainSel = !m.plainSel
		if m.plainSel {
			m.say(i18n.T("panel_select_on"))
			return m, tea.DisableMouse
		}
		m.say(i18n.T("panel_select_off"))
		return m, tea.EnableMouseCellMotion
	case "esc":
		m.hasSel = false
		m.refresh(true)
	}
	return m, nil
}

func (m *Model) selChanged() {
	m.follow = true
	m.hasSel = false
	m.lastKey = ""
	m.refresh(true)
}

func (m *Model) logTop() int {
	_, _, _, vpW, _ := m.layout()
	top := 4 + m.cardHeight(vpW)
	if len(m.sess.Notes) > 0 {
		top++
	}
	return top
}

func (m *Model) mouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if !m.ready {
		return m, nil
	}
	listW, _, _, _, _ := m.layout()
	if msg.Button == tea.MouseButtonWheelUp || msg.Button == tea.MouseButtonWheelDown {
		if msg.X < listW {
			if msg.Button == tea.MouseButtonWheelUp && m.sel > 0 {
				m.sel--
			} else if msg.Button == tea.MouseButtonWheelDown && m.sel < len(m.procs)-1 {
				m.sel++
			}
			m.selChanged()
			return m, nil
		}
		m.follow, m.focusLog = false, true
		var cmd tea.Cmd
		m.vp, cmd = m.vp.Update(msg)
		if m.vp.AtBottom() {
			m.follow = true
		}
		return m, cmd
	}
	if msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonLeft && msg.X < listW {
		if i := m.rowAt(msg.Y); i >= 0 {
			m.sel = i
			m.focusLog = false
			m.selChanged()
		}
		return m, nil
	}
	row := msg.Y - m.logTop()
	inLog := msg.X >= listW+2 && row >= 0 && row < m.vp.Height
	line := m.vp.YOffset + row
	switch {
	case msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonLeft:
		if !inLog || line >= len(m.wrapped) {
			m.hasSel = false
			m.refresh(true)
			return m, nil
		}
		m.dragging, m.hasSel = true, true
		m.follow, m.focusLog = false, true
		m.selA, m.selB = line, line
		m.refresh(true)
	case msg.Action == tea.MouseActionMotion && m.dragging:
		if row < 0 {
			m.vp.LineUp(1)
		} else if row >= m.vp.Height {
			m.vp.LineDown(1)
		}
		m.selB = clamp(m.vp.YOffset+row, 0, len(m.wrapped)-1)
		m.refresh(true)
	case msg.Action == tea.MouseActionRelease && m.dragging:
		m.dragging = false
		m.copySelection()
		m.refresh(true)
	}
	return m, nil
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func (m *Model) copySelection() {
	p := m.selected()
	if p == nil || !m.hasSel {
		return
	}
	lines := p.Lines()
	lo, hi := m.selA, m.selB
	if lo > hi {
		lo, hi = hi, lo
	}
	var picked []string
	last := -1
	for i := lo; i <= hi && i < len(m.owner); i++ {
		if o := m.owner[i]; o != last && o < len(lines) {
			picked = append(picked, ansi.Strip(lines[o]))
			last = o
		}
	}
	if err := sys.Clipboard(strings.Join(picked, "\n")); err != nil {
		m.say(i18n.T("panel_copy_failed"))
		return
	}
	m.say(i18n.T("panel_copied", len(picked)))
}

func (m *Model) refresh(force bool) {
	p := m.selected()
	if p == nil || !m.ready {
		return
	}
	key := p.Spec.ID + "\x00" + p.LineKey()
	if !force && key == m.lastKey {
		if m.follow {
			m.vp.GotoBottom()
		}
		return
	}
	m.lastKey = key
	lines := p.Lines()
	if len(lines) == 0 {
		lines = []string{ui.Muted.Render(i18n.T("panel_no_output"))}
	}
	m.wrapped, m.owner = wrapLines(lines, m.vp.Width)
	shown := m.wrapped
	if m.hasSel {
		lo, hi := m.selA, m.selB
		if lo > hi {
			lo, hi = hi, lo
		}
		shown = append([]string(nil), m.wrapped...)
		for i := lo; i <= hi && i < len(shown); i++ {
			shown[i] = lipgloss.NewStyle().Reverse(true).Render(ansi.Strip(shown[i]))
		}
	}
	y := m.vp.YOffset
	m.vp.SetContent(strings.Join(shown, "\n"))
	if m.follow {
		m.vp.GotoBottom()
	} else {
		m.vp.SetYOffset(y)
	}
}

func wrapLines(lines []string, width int) ([]string, []int) {
	if width < 1 {
		width = 1
	}
	var out []string
	var owner []int
	for i, l := range lines {
		for _, part := range strings.Split(ansi.Hardwrap(l, width, true), "\n") {
			out = append(out, part)
			owner = append(owner, i)
		}
	}
	return out, owner
}

// rows maps list rows (y) to process indexes, mirroring View's layout.
func (m *Model) rowAt(y int) int {
	top := 2
	if len(m.sess.Notes) > 0 {
		top++
	}
	rows := m.listRows()
	for i := m.listOffset; i < len(rows); i++ {
		if top+i-m.listOffset == y {
			return rows[i]
		}
	}
	return -1
}

// listRows returns, for each printed list line, the process index (-1 for headings/blank lines).
func (m *Model) listRows() []int {
	var rows []int
	prev := ""
	for i, p := range m.procs {
		sec := section(p)
		if sec != prev {
			if prev != "" {
				rows = append(rows, -1)
			}
			rows = append(rows, -1)
			if strings.HasPrefix(sec, "app:") && !strings.HasPrefix(prev, "app:") {
				rows = append(rows, -1) // "Apps" title + group title
			}
			prev = sec
		}
		rows = append(rows, i)
	}
	return rows
}

func section(p *run.Proc) string {
	switch {
	case p.Spec.Group != "":
		return "app:" + p.Spec.Group
	case p.Spec.Kind == run.KindInfra:
		return "infra"
	case p.Spec.Kind == run.KindAPI:
		return "api"
	default:
		return "web"
	}
}

func (m *Model) cardHeight(w int) int {
	p := m.selected()
	if p == nil {
		return 0
	}
	_, _, _, _, crash := p.Snapshot()
	if crash == nil {
		return 0
	}
	return lipgloss.Height(m.card(p, crash, w))
}

// Uptime formats durations compactly.
func Uptime(d time.Duration) string {
	switch {
	case d <= 0:
		return ""
	case d < time.Minute:
		return strconv.Itoa(int(d.Seconds())) + "s"
	case d < time.Hour:
		return strconv.Itoa(int(d.Minutes())) + "m"
	default:
		return fmt.Sprintf("%dh%02dm", int(d.Hours()), int(d.Minutes())%60)
	}
}

func init() {
	// Bubbletea reads TERM; Windows Terminal and modern consoles support the alt screen.
	if os.Getenv("TERM") == "" && sys.Windows {
		os.Setenv("TERM", "xterm-256color")
	}
}
