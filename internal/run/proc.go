package run

import (
	"bufio"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/ipalpha-dev/tooling/internal/i18n"
	"github.com/ipalpha-dev/tooling/internal/sys"
)

// State of a process.
type State int

const (
	Stopped State = iota
	Starting
	Running
	Exited
)

const maxLines = 8000

// Proc is one running (or stoppable) row: an API, a web app or an infra log follower.
type Proc struct {
	Spec *Spec

	mu       sync.Mutex
	state    State
	exit     int
	lines    []string
	cmd      *exec.Cmd
	ready    bool
	known    bool
	since    time.Time
	gen      int
	crash    *Crash
	logFile  *os.File
	health   string // infra container health
	restarts int
	group    int // process group (Unix) / root pid (Windows) of the last start
}

// Crash explains why a process stopped, with a suggested action.
type Crash struct {
	Summary string
	Fix     string
	Lines   []string
	Port    int // EADDRINUSE port, when that was the cause
}

// Event tells the UI something changed.
type Event struct {
	ID   string
	Line string
	Exit bool
}

// NewProc wraps a spec.
func NewProc(s *Spec) *Proc { return &Proc{Spec: s} }

// Snapshot of the visible state.
func (p *Proc) Snapshot() (State, int, bool, bool, *Crash) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.state, p.exit, p.known, p.ready, p.crash
}

// Lines returns a copy of the log.
func (p *Proc) Lines() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]string(nil), p.lines...)
}

// LineCount and last line (cheap change detection).
func (p *Proc) LineKey() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.lines) == 0 {
		return "0"
	}
	return strconv.Itoa(len(p.lines)) + p.lines[len(p.lines)-1]
}

// Clear the log.
func (p *Proc) Clear() {
	p.mu.Lock()
	p.lines = nil
	p.mu.Unlock()
}

// Health of an infra row.
func (p *Proc) Health() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.health
}

// SetHealth updates an infra row.
func (p *Proc) SetHealth(h string) {
	p.mu.Lock()
	p.health = h
	p.mu.Unlock()
}

// PID of the running process (0 when none).
func (p *Proc) PID() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.cmd != nil && p.cmd.Process != nil {
		return p.cmd.Process.Pid
	}
	return 0
}

// Group returns the process group of the last start while any of it still runs (0 otherwise).
func (p *Proc) Group() int {
	p.mu.Lock()
	g := p.group
	p.mu.Unlock()
	if g > 0 && sys.GroupAlive(g) {
		return g
	}
	return 0
}

// Escape sequences other than colors (SGR) would move the cursor or clear the panel's screen
// (tsc --watch clears the console on every rebuild): keep colors, drop the rest.
var (
	reCSI      = regexp.MustCompile(`\x1b\[[0-9;?]*[ -/]*[@-~]`)
	reOtherEsc = regexp.MustCompile(`\x1b(?:\][^\x07\x1b]*(?:\x07|\x1b\\)|[PX^_][^\x1b]*\x1b\\|[@-Z\\-_a-z])`)
	reControl  = regexp.MustCompile("[\x00-\x08\x0b-\x1a\x1c-\x1f\x7f]")
)

// Sanitize keeps a log line safe to draw inside the panel.
func Sanitize(line string) string {
	if i := strings.LastIndex(line, "\r"); i >= 0 {
		line = line[i+1:] // progress bars redraw with \r: keep the final state
	}
	line = reCSI.ReplaceAllStringFunc(line, func(s string) string {
		if strings.HasSuffix(s, "m") {
			return s
		}
		return ""
	})
	line = reOtherEsc.ReplaceAllString(line, "")
	line = strings.ReplaceAll(line, "\t", "    ")
	return reControl.ReplaceAllString(line, "")
}

func (p *Proc) append(line string) {
	line = Sanitize(line)
	p.mu.Lock()
	p.lines = append(p.lines, line)
	if len(p.lines) > maxLines {
		p.lines = p.lines[len(p.lines)-maxLines:]
	}
	if p.logFile != nil {
		fmt.Fprintln(p.logFile, ansi.Strip(line))
	}
	p.mu.Unlock()
}

// Note adds a tool message to the log (dimmed in the panel).
func (p *Proc) Note(msg string) { p.append("\x1b[2m› " + msg + "\x1b[0m") }

// Start launches the process. logDir receives <id>.log (truncated per start).
func (p *Proc) Start(events chan<- Event, logDir string) {
	p.mu.Lock()
	if p.state == Running || p.state == Starting {
		p.mu.Unlock()
		return
	}
	if p.Spec.Missing != "" {
		p.mu.Unlock()
		p.Note(p.Spec.Missing)
		return
	}
	p.state = Starting
	p.exit = 0
	p.crash = nil
	p.known, p.ready = false, false
	p.gen++
	gen := p.gen
	p.mu.Unlock()

	if logDir != "" {
		_ = os.MkdirAll(logDir, 0o755)
		if f, err := os.Create(filepath.Join(logDir, p.Spec.ID+".log")); err == nil {
			p.mu.Lock()
			if p.logFile != nil {
				p.logFile.Close()
			}
			p.logFile = f
			p.mu.Unlock()
		}
	}
	c := exec.Command(p.Spec.Command, p.Spec.Args...)
	c.Dir = p.Spec.Dir
	c.Env = append(os.Environ(), p.Spec.Env...)
	c.Env = append(c.Env, "FORCE_COLOR=1", "IPALPHA_PANEL=1")
	sys.Detach(c)
	stdout, _ := c.StdoutPipe()
	stderr, _ := c.StderrPipe()
	if err := c.Start(); err != nil {
		p.mu.Lock()
		p.state, p.exit = Exited, 1
		p.crash = &Crash{Summary: i18n.T("crash_start_failed", err.Error()), Fix: startFix(p.Spec.Command)}
		p.mu.Unlock()
		p.append(i18n.T("crash_start_failed", err.Error()))
		send(events, Event{ID: p.Spec.ID, Exit: true})
		return
	}
	sys.Attach(c)
	p.mu.Lock()
	p.cmd = c
	p.group = c.Process.Pid
	p.state = Running
	p.since = time.Now()
	p.mu.Unlock()

	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); p.pump(stdout, events) }()
	go func() { defer wg.Done(); p.pump(stderr, events) }()
	if p.Spec.ReadyURL != "" && p.Spec.Kind != KindInfra {
		go p.readiness(gen)
	}
	go func() {
		wg.Wait()
		err := c.Wait()
		code := 0
		if err != nil {
			if ee, ok := err.(*exec.ExitError); ok {
				code = ee.ExitCode()
			} else {
				code = 1
			}
		}
		sys.KillGroup(c.Process.Pid) // tsc --watch and node children outlive npm otherwise
		p.mu.Lock()
		if p.cmd == c {
			p.cmd = nil
		}
		stopped := p.state == Stopped
		if !stopped && p.gen == gen {
			p.state, p.exit = Exited, code
			if code != 0 || p.Spec.Kind != KindInfra {
				p.crash = Diagnose(p.lines, code, p.Spec)
			}
		}
		p.mu.Unlock()
		send(events, Event{ID: p.Spec.ID, Exit: true})
	}()
}

func startFix(cmd string) string {
	if cmd == "node" || strings.HasPrefix(cmd, "npm") {
		return i18n.T("crash_fix_node_missing")
	}
	return i18n.T("crash_fix_doctor")
}

func send(ch chan<- Event, e Event) {
	if ch == nil {
		return
	}
	select {
	case ch <- e:
	default:
	}
}

func (p *Proc) pump(r io.Reader, events chan<- Event) {
	if r == nil {
		return
	}
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := Sanitize(strings.TrimRight(sc.Text(), "\r"))
		if line == "" && strings.TrimSpace(ansi.Strip(sc.Text())) != "" {
			continue // only a screen clear
		}
		p.append(line)
		send(events, Event{ID: p.Spec.ID, Line: line})
	}
}

func (p *Proc) readiness(gen int) {
	client := &http.Client{Timeout: 1500 * time.Millisecond, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	for {
		p.mu.Lock()
		alive := p.state == Running && p.gen == gen
		p.mu.Unlock()
		if !alive {
			return
		}
		ok := false
		if resp, err := client.Get(p.Spec.ReadyURL); err == nil {
			ok = resp.StatusCode >= 200 && resp.StatusCode < 400
			resp.Body.Close()
		}
		p.mu.Lock()
		if p.gen == gen {
			p.known, p.ready = true, ok
		}
		p.mu.Unlock()
		time.Sleep(1500 * time.Millisecond)
	}
}

// Ready reports a running process that answers its ready URL.
func (p *Proc) Ready() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.state == Running && p.known && p.ready
}

// Stop terminates the whole process tree.
func (p *Proc) Stop() {
	p.mu.Lock()
	c := p.cmd
	group := p.group
	p.state = Stopped
	p.exit = 0
	p.crash = nil
	p.gen++
	p.mu.Unlock()
	if c != nil {
		sys.KillTree(c)
	}
	if group > 0 && sys.GroupAlive(group) {
		sys.KillGroup(group)
	}
	p.mu.Lock()
	if p.logFile != nil {
		p.logFile.Close()
		p.logFile = nil
	}
	p.mu.Unlock()
}

// Restart = Stop + Start.
func (p *Proc) Restart(events chan<- Event, logDir string) {
	p.Stop()
	time.Sleep(150 * time.Millisecond)
	p.Start(events, logDir)
}

// Uptime of a running process.
func (p *Proc) Uptime() time.Duration {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.state != Running {
		return 0
	}
	return time.Since(p.since)
}

var (
	reAddrInUse  = regexp.MustCompile(`EADDRINUSE[^\n]*?:(\d{2,5})`)
	reModule     = regexp.MustCompile(`Cannot find module '([^']+)'`)
	reEnvMissing = regexp.MustCompile(`(?:missing|required)[^\n]*?\b([A-Z][A-Z0-9_]{2,})\b`)
	reMongoAuth  = regexp.MustCompile(`(?i)(MongoServerError: Authentication failed|bad auth)`)
	reConnRef    = regexp.MustCompile(`ECONNREFUSED[^\n]*?:(\d{2,5})`)
	reTS         = regexp.MustCompile(`error TS\d+`)
)

// Diagnose reads the last lines of a stopped process and explains the most likely cause.
func Diagnose(lines []string, code int, spec *Spec) *Crash {
	n := len(lines)
	start := n - 60
	if start < 0 {
		start = 0
	}
	tail := lines[start:]
	text := ansi.Strip(strings.Join(tail, "\n"))
	c := &Crash{Lines: interesting(tail)}
	switch {
	case reAddrInUse.MatchString(text):
		port, _ := strconv.Atoi(reAddrInUse.FindStringSubmatch(text)[1])
		c.Port = port
		c.Summary = i18n.T("crash_port_in_use", port)
		c.Fix = i18n.T("crash_fix_port")
	case reModule.MatchString(text):
		mod := reModule.FindStringSubmatch(text)[1]
		c.Summary = i18n.T("crash_module_missing", mod)
		if strings.HasPrefix(mod, "@ipalpha/") {
			c.Fix = i18n.T("crash_fix_shared")
		} else {
			c.Fix = i18n.T("crash_fix_reinstall")
		}
	case reMongoAuth.MatchString(text):
		c.Summary = i18n.T("crash_mongo_auth")
		c.Fix = i18n.T("crash_fix_mongo_auth")
	case reTS.MatchString(text):
		c.Summary = i18n.T("crash_typescript")
		c.Fix = i18n.T("crash_fix_typescript")
	case reEnvMissing.MatchString(text) && strings.Contains(strings.ToLower(text), "env"):
		key := reEnvMissing.FindStringSubmatch(text)[1]
		c.Summary = i18n.T("crash_env_missing", key)
		c.Fix = i18n.T("crash_fix_env", spec.ID)
	case reConnRef.MatchString(text):
		port, _ := strconv.Atoi(reConnRef.FindStringSubmatch(text)[1])
		c.Summary = i18n.T("crash_conn_refused", port)
		c.Fix = i18n.T("crash_fix_infra")
	case code == 0:
		c.Summary = i18n.T("crash_exited_clean")
		c.Fix = i18n.T("crash_fix_restart")
	default:
		c.Summary = i18n.T("crash_exit_code", code)
		c.Fix = i18n.T("crash_fix_read_log")
	}
	return c
}

var reErrorish = regexp.MustCompile(`(?i)(error|exception|fail|cannot|unable|refused|denied|missing|invalid|EADDRINUSE|throw|at .+:\d+:\d+)`)

func interesting(lines []string) []string {
	var picked []string
	for _, l := range lines {
		s := strings.TrimSpace(ansi.Strip(l))
		if s != "" && reErrorish.MatchString(s) {
			picked = append(picked, s)
		}
	}
	if len(picked) == 0 {
		for _, l := range lines {
			if s := strings.TrimSpace(ansi.Strip(l)); s != "" {
				picked = append(picked, s)
			}
		}
	}
	if len(picked) > 6 {
		picked = picked[len(picked)-6:]
	}
	return picked
}
