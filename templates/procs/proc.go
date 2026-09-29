package main

import (
	"bufio"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"
)

const maxLogLines = 8000

type procState int

const (
	stateStopped procState = iota
	stateWaiting
	stateStarting
	stateRunning
	stateExited
)

func (s procState) String() string {
	switch s {
	case stateStopped:
		return "stopped"
	case stateWaiting:
		return "waiting"
	case stateStarting:
		return "starting"
	case stateRunning:
		return "running"
	case stateExited:
		return "exited"
	default:
		return "?"
	}
}

type proc struct {
	id        string
	name      string
	kind      string
	cwd       string
	port      string
	frontend  string
	parent    string
	deps      []string
	softDeps  []string
	shell     bool
	cmd       string
	stopCmd   string
	args      []string
	autostart bool

	mu     sync.Mutex
	state  procState
	exit   int
	lines  []string
	cmdP   *exec.Cmd
	cancel chan struct{}
	// readiness: services expose GET /ready (200 = may receive traffic). Polled while running.
	readyKnown bool
	ready      bool
	readyAt    time.Time
}

type logMsg struct {
	id   string
	line string
}

type exitMsg struct {
	id   string
	code int
}

func (p *proc) snapshot() (state procState, exit int, lines []string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	cp := make([]string, len(p.lines))
	copy(cp, p.lines)
	return p.state, p.exit, cp
}

func (p *proc) appendLine(line string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.lines = append(p.lines, line)
	if len(p.lines) > maxLogLines {
		p.lines = p.lines[len(p.lines)-maxLogLines:]
	}
}

func (p *proc) start(ch chan<- teaMsg) error {
	p.mu.Lock()
	if p.state == stateRunning || p.state == stateStarting || p.state == stateWaiting {
		p.mu.Unlock()
		return nil
	}
	p.state = stateStarting
	p.exit = 0
	p.cancel = make(chan struct{})
	p.mu.Unlock()

	return p.startNow(ch)
}

func (p *proc) cancelled() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.cancel == nil {
		return p.state == stateStopped
	}
	select {
	case <-p.cancel:
		return true
	default:
		return p.state == stateStopped
	}
}

// startAfterDeps no longer waits: every service boots at once and reports readiness through
// its own GET /ready (see readinessLoop). Dependencies are only echoed for context.
func (p *proc) startAfterDeps(ch chan<- teaMsg, depPorts map[string]string) {
	p.mu.Lock()
	if p.state == stateRunning || p.state == stateStarting || p.state == stateWaiting {
		p.mu.Unlock()
		return
	}
	p.state = stateStarting
	p.exit = 0
	p.cancel = make(chan struct{})
	p.mu.Unlock()

	if len(p.deps) > 0 || len(p.softDeps) > 0 {
		all := append(append([]string{}, p.deps...), p.softDeps...)
		p.appendLine(tr("uses") + ": " + strings.Join(all, ", ") + " — " + tr("start_no_wait"))
	}
	_ = depPorts
	_ = p.startNow(ch)
}

// readinessLoop polls GET /ready while the process runs. Only services with a port take part;
// web apps and infra are "ready" as soon as they run.
func (p *proc) readinessLoop() {
	if p.port == "" || p.kind == "infrastructure" || p.kind == "app" || p.kind == "attached" {
		return
	}
	url := "http://127.0.0.1:" + p.port + "/ready"
	client := &http.Client{Timeout: 1500 * time.Millisecond}
	for {
		st, _, _ := p.snapshot()
		if st != stateRunning {
			p.mu.Lock()
			p.readyKnown = false
			p.ready = false
			p.mu.Unlock()
			return
		}
		ok := false
		if resp, err := client.Get(url); err == nil {
			ok = resp.StatusCode == 200
			resp.Body.Close()
		}
		p.mu.Lock()
		if !p.readyKnown || p.ready != ok {
			p.readyAt = time.Now()
		}
		p.readyKnown = true
		p.ready = ok
		p.mu.Unlock()
		time.Sleep(1500 * time.Millisecond)
	}
}

func (p *proc) isReady() (known bool, ready bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.readyKnown, p.ready
}

func (p *proc) startNow(ch chan<- teaMsg) error {
	var c *exec.Cmd
	if p.shell {
		c = exec.Command("bash", "-c", p.cmd)
	} else {
		c = exec.Command(p.args[0], p.args[1:]...)
	}
	c.Dir = p.cwd
	c.Env = os.Environ()
	c.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	stdout, err := c.StdoutPipe()
	if err != nil {
		p.setState(stateExited, 1)
		p.appendLine(tr("start_error") + ": " + err.Error())
		select {
		case ch <- exitMsg{id: p.id, code: 1}:
		default:
		}
		return err
	}
	stderr, err := c.StderrPipe()
	if err != nil {
		p.setState(stateExited, 1)
		p.appendLine(tr("start_error") + ": " + err.Error())
		select {
		case ch <- exitMsg{id: p.id, code: 1}:
		default:
		}
		return err
	}
	if err := c.Start(); err != nil {
		p.setState(stateExited, 1)
		p.appendLine(tr("start_error") + ": " + err.Error())
		select {
		case ch <- exitMsg{id: p.id, code: 1}:
		default:
		}
		return err
	}

	p.mu.Lock()
	p.cmdP = c
	p.state = stateRunning
	p.readyKnown = false
	p.ready = false
	p.mu.Unlock()

	go p.readinessLoop()
	go p.pump(stdout, ch)
	go p.pump(stderr, ch)
	go func() {
		err := c.Wait()
		code := 0
		if err != nil {
			if ee, ok := err.(*exec.ExitError); ok {
				code = ee.ExitCode()
			} else {
				code = 1
			}
		}
		p.mu.Lock()
		p.cmdP = nil
		if p.state != stateStopped {
			p.state = stateExited
			p.exit = code
		}
		p.mu.Unlock()
		select {
		case ch <- exitMsg{id: p.id, code: code}:
		default:
		}
	}()
	return nil
}

func (p *proc) pump(r io.Reader, ch chan<- teaMsg) {
	sc := bufio.NewScanner(r)
	buf := make([]byte, 0, 64*1024)
	sc.Buffer(buf, 1024*1024)
	for sc.Scan() {
		line := sc.Text()
		p.appendLine(line)
		select {
		case ch <- logMsg{id: p.id, line: line}:
		default:
		}
	}
}

func (p *proc) stop() {
	p.mu.Lock()
	c := p.cmdP
	cancel := p.cancel
	p.state = stateStopped
	p.exit = 0
	if cancel != nil {
		select {
		case <-cancel:
		default:
			close(cancel)
		}
		p.cancel = nil
	}
	p.mu.Unlock()

	if p.stopCmd != "" {
		down := exec.Command("bash", "-c", p.stopCmd)
		down.Dir = p.cwd
		_ = down.Run()
	}

	if c == nil || c.Process == nil {
		return
	}
	pid := c.Process.Pid
	pgid, pgErr := syscall.Getpgid(pid)
	if pgErr == nil {
		_ = syscall.Kill(-pgid, syscall.SIGTERM)
	} else {
		_ = c.Process.Signal(syscall.SIGTERM)
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if err := syscall.Kill(pid, 0); err != nil {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if err := syscall.Kill(pid, 0); err == nil {
		if pgErr == nil {
			_ = syscall.Kill(-pgid, syscall.SIGKILL)
		} else {
			_ = c.Process.Kill()
		}
		time.Sleep(100 * time.Millisecond)
	}
	p.mu.Lock()
	if p.cmdP == c {
		p.cmdP = nil
	}
	p.state = stateStopped
	p.mu.Unlock()
}

func (p *proc) restart(ch chan<- teaMsg) {
	p.stop()
	time.Sleep(150 * time.Millisecond)
	_ = p.start(ch)
}

func (p *proc) restartWithDeps(ch chan<- teaMsg, depPorts map[string]string) {
	p.stop()
	time.Sleep(150 * time.Millisecond)
	if len(p.deps) > 0 {
		go p.startAfterDeps(ch, depPorts)
		return
	}
	_ = p.start(ch)
}

func (p *proc) setState(s procState, exit int) {
	p.mu.Lock()
	p.state = s
	p.exit = exit
	p.mu.Unlock()
}

func (p *proc) statusLabel() string {
	st, code, _ := p.snapshot()
	switch st {
	case stateRunning:
		if known, ready := p.isReady(); known && !ready {
			return "◐"
		}
		return "●"
	case stateStarting, stateWaiting:
		return "◐"
	case stateExited:
		if code == 0 {
			return "○"
		}
		return "✖"
	default:
		return "○"
	}
}

func (p *proc) statusText() string {
	st, code, _ := p.snapshot()
	switch st {
	case stateRunning:
		if known, ready := p.isReady(); known && !ready {
			return tr("not_ready")
		}
		return tr("running")
	case stateStarting:
		return tr("starting")
	case stateWaiting:
		return tr("waiting")
	case stateExited:
		if code != 0 {
			return tr("exit") + " " + itoa(code)
		}
		return tr("exited")
	default:
		return tr("stopped")
	}
}
func (p *proc) failed() bool {
	st, code, _ := p.snapshot()
	return st == stateExited && code != 0
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [12]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}

func (p *proc) clearLines() {
	p.mu.Lock()
	p.lines = nil
	p.mu.Unlock()
}
