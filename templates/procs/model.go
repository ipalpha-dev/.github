package main

import (
	"encoding/base64"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
)

type teaMsg interface{}

type tickMsg time.Time
type stopStepMsg struct {
	index int
}

type quitNowMsg struct{}

type model struct {
	ipalphaDir   string
	root         string
	procs        []*proc
	selected     int
	width        int
	height       int
	ready        bool
	quitting     bool
	shutdownDone bool
	vp           viewport.Model
	follow       bool
	selectMode   bool
	focusLog     bool
	events       chan teaMsg
	status       string
	lastLogKey   string
}

func newModel(ipalphaDir, root string, procs []*proc) model {
	return model{
		ipalphaDir: ipalphaDir,
		root:       root,
		procs:      procs,
		selected:   0,
		follow:     true,
		focusLog:   false,
		events:     make(chan teaMsg, 256),
	}
}

func (m model) Init() tea.Cmd {
	return tea.Batch(
		m.startAutostart(),
		listenEvents(m.events),
		tick(),
	)
}

func (m model) startAutostart() tea.Cmd {
	return func() tea.Msg {
		ports := map[string]string{}
		for _, p := range m.procs {
			if p.port != "" {
				ports[p.id] = p.port
			}
		}
		for _, p := range m.procs {
			if !p.autostart {
				continue
			}
			if p.kind == "infrastructure" {
				_ = p.start(m.events)
				continue
			}
			pp := p
			go pp.startAfterDeps(m.events, ports)
			time.Sleep(120 * time.Millisecond)
		}
		return nil
	}
}

func listenEvents(ch <-chan teaMsg) tea.Cmd {
	return func() tea.Msg {
		msg, ok := <-ch
		if !ok {
			return nil
		}
		return msg
	}
}

func tick() tea.Cmd {
	return tea.Tick(200*time.Millisecond, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func (m model) layout() (listW, logW, listH, vpW, vpH int) {
	listW = listWidth(m.width)
	logW = m.width - listW - 1
	if logW < 24 {
		logW = 24
	}
	bodyH := m.height - 4
	if bodyH < 8 {
		bodyH = 8
	}
	listH = bodyH
	vpW = logW - 4
	if vpW < 10 {
		vpW = 10
	}
	vpH = bodyH - 1
	if vpH < 3 {
		vpH = 3
	}
	return listW, logW, listH, vpW, vpH
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		_, _, _, vpW, vpH := m.layout()
		if !m.ready {
			m.vp = viewport.New(vpW, vpH)
			m.vp.MouseWheelEnabled = true
			m.ready = true
		} else {
			m.vp.Width = vpW
			m.vp.Height = vpH
		}
		m.refreshLog(true)
		return m, nil

	case logMsg:
		if m.selectedProc() != nil && m.selectedProc().id == msg.id {
			m.refreshLog(false)
		}
		return m, listenEvents(m.events)

	case exitMsg:
		if m.selectedProc() != nil && m.selectedProc().id == msg.id {
			m.refreshLog(true)
		}
		return m, listenEvents(m.events)

	case tickMsg:
		m.refreshLog(false)
		return m, tick()

	case tea.MouseMsg:
		if !m.ready {
			return m, nil
		}
		switch msg.Button {
		case tea.MouseButtonWheelUp, tea.MouseButtonWheelDown:
			m.follow = false
			m.focusLog = true
			var cmd tea.Cmd
			m.vp, cmd = m.vp.Update(msg)
			return m, cmd
		}

	case stopStepMsg:
		if !m.quitting {
			return m, nil
		}
		i := len(m.procs) - 1 - msg.index
		if i >= 0 && i < len(m.procs) {
			p := m.procs[i]
			m.status = tr("stopping") + " " + p.name + "…"
			p.stop()
			return m, tea.Tick(80*time.Millisecond, func(t time.Time) tea.Msg {
				return stopStepMsg{index: msg.index + 1}
			})
		}
		m.shutdownDone = true
		m.status = tr("all_done")
		return m, tea.Tick(350*time.Millisecond, func(t time.Time) tea.Msg {
			return quitNowMsg{}
		})

	case quitNowMsg:
		return m, tea.Quit

	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			if m.quitting {
				m.stopAll()
				m.shutdownDone = true
				return m, tea.Quit
			}
			m.quitting = true
			m.status = tr("stopping_all")
			return m, func() tea.Msg { return stopStepMsg{index: 0} }

		case "tab", "right", "l":
			m.focusLog = true
			m.status = tr("log_focus")
			return m, nil
		case "shift+tab", "left", "h":
			m.focusLog = false
			m.status = tr("proc_focus")
			return m, nil

		case "up", "k":
			if m.focusLog {
				m.follow = false
				m.vp.LineUp(1)
				return m, nil
			}
			if m.selected > 0 {
				m.selected--
				m.follow = true
				m.lastLogKey = ""
				m.refreshLog(true)
			}
			return m, nil
		case "down", "j":
			if m.focusLog {
				m.follow = false
				m.vp.LineDown(1)
				if m.vp.AtBottom() {
					m.follow = true
				}
				return m, nil
			}
			if m.selected < len(m.procs)-1 {
				m.selected++
				m.follow = true
				m.lastLogKey = ""
				m.refreshLog(true)
			}
			return m, nil

		case "r":
			if p := m.selectedProc(); p != nil {
				m.status = tr("restart") + " " + p.name
				ports := m.depPorts()
				go p.restartWithDeps(m.events, ports)
			}
			return m, nil
		case "s":
			if p := m.selectedProc(); p != nil {
				st, _, _ := p.snapshot()
				if st == stateRunning || st == stateStarting || st == stateWaiting {
					m.status = tr("stop") + " " + p.name
					go p.stop()
				} else {
					m.status = tr("start") + " " + p.name
					ports := m.depPorts()
					go p.startAfterDeps(m.events, ports)
				}
			}
			return m, nil
		case "x":
			if p := m.selectedProc(); p != nil {
				m.status = tr("stop") + " " + p.name
				go p.stop()
			}
			return m, nil

		case "pgup", "ctrl+u", "u":
			m.follow = false
			m.focusLog = true
			m.vp.HalfViewUp()
			return m, nil
		case "pgdown", "ctrl+d", "d":
			m.focusLog = true
			m.vp.HalfViewDown()
			if m.vp.AtBottom() {
				m.follow = true
			} else {
				m.follow = false
			}
			return m, nil
		case "home", "g":
			m.follow = false
			m.focusLog = true
			m.vp.GotoTop()
			return m, nil
		case "end", "G":
			m.follow = true
			m.focusLog = true
			m.vp.GotoBottom()
			return m, nil
		case "o":
			if p := m.selectedProc(); p != nil {
				if p.frontend == "" {
					m.status = tr("no_frontend")
				} else {
					m.status = tr("open") + " " + p.frontend
					go openURL(p.frontend)
				}
			}
			return m, nil
		case "c":
			if p := m.selectedProc(); p != nil {
				p.clearLines()
				m.status = tr("cleared")
				m.lastLogKey = ""
				m.refreshLog(true)
			}
			return m, nil
		case "m":
			m.selectMode = !m.selectMode
			if m.selectMode {
				m.status = tr("select_on")
				return m, tea.DisableMouse
			}
			m.status = tr("select_off")
			return m, tea.EnableMouseCellMotion
		case "y":
			if p := m.selectedProc(); p != nil {
				_, _, lines := p.snapshot()
				if err := copyToClipboard(stripANSI(strings.Join(lines, "\n"))); err != nil {
					m.status = tr("copy_fail")
				} else {
					m.status = fmt.Sprintf("%s: %d %s", tr("copied"), len(lines), tr("lines"))
				}
			}
			return m, nil
		case "f":
			m.follow = !m.follow
			if m.follow {
				m.vp.GotoBottom()
			}
			return m, nil
		}
		return m, nil
	}

	return m, nil
}

func (m *model) selectedProc() *proc {
	if m.selected < 0 || m.selected >= len(m.procs) {
		return nil
	}
	return m.procs[m.selected]
}

func (m model) depPorts() map[string]string {
	ports := map[string]string{}
	for _, p := range m.procs {
		if p.port != "" {
			ports[p.id] = p.port
		}
	}
	return ports
}
func (m *model) refreshLog(force bool) {
	p := m.selectedProc()
	if p == nil || !m.ready {
		return
	}
	_, _, lines := p.snapshot()
	key := p.id + "\x00" + itoa(len(lines))
	if len(lines) > 0 {
		key += "\x00" + lines[len(lines)-1]
	}
	if !force && key == m.lastLogKey {
		if m.follow {
			m.vp.GotoBottom()
		}
		return
	}
	m.lastLogKey = key

	content := strings.Join(lines, "\n")
	if content == "" {
		content = tr("no_output")
	}
	y := m.vp.YOffset
	m.vp.SetContent(content)
	if m.follow {
		m.vp.GotoBottom()
	} else {
		m.vp.SetYOffset(y)
	}
}

func (m *model) stopAll() {
	for i := len(m.procs) - 1; i >= 0; i-- {
		m.procs[i].stop()
	}
}

func listWidth(total int) int {
	w := 34
	if total < 100 {
		w = 30
	}
	if total < 80 {
		w = 26
	}
	if total < 60 {
		w = 22
	}
	return w
}

func openURL(u string) {
	name := "xdg-open"
	if runtime.GOOS == "darwin" {
		name = "open"
	}
	_ = exec.Command(name, u).Start()
}

var reANSI = regexp.MustCompile(`\x1b\[[0-9;?]*[ -/]*[@-~]`)

func stripANSI(s string) string {
	return reANSI.ReplaceAllString(s, "")
}

func copyToClipboard(text string) error {
	var c *exec.Cmd
	switch {
	case runtime.GOOS == "darwin":
		c = exec.Command("pbcopy")
	case commandExists("wl-copy"):
		c = exec.Command("wl-copy")
	case commandExists("xclip"):
		c = exec.Command("xclip", "-selection", "clipboard")
	default:
		fmt.Fprintf(os.Stderr, "\x1b]52;c;%s\a", base64.StdEncoding.EncodeToString([]byte(text)))
		return nil
	}
	c.Stdin = strings.NewReader(text)
	return c.Run()
}

func commandExists(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}
