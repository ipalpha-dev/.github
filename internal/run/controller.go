package run

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/ipalpha-dev/tooling/internal/catalog"
	"github.com/ipalpha-dev/tooling/internal/i18n"
	"github.com/ipalpha-dev/tooling/internal/ports"
	"github.com/ipalpha-dev/tooling/internal/sys"
	"github.com/ipalpha-dev/tooling/internal/ui"
)

// Controller owns the processes of a session and implements every action of the panel.
type Controller struct {
	S      *Session
	Procs  []*Proc
	Events chan Event
	LogDir string

	mu       sync.Mutex
	opened   map[string]bool
	ctx      context.Context
	cancel   context.CancelFunc
	autoOpen bool
}

// NewController builds the rows (infra, core APIs, web apps, then apps outside core).
func NewController(s *Session) *Controller {
	ctx, cancel := context.WithCancel(context.Background())
	c := &Controller{S: s, Events: make(chan Event, 512), LogDir: s.W.LogsDir() + string(os.PathSeparator) + "run",
		opened: map[string]bool{}, ctx: ctx, cancel: cancel,
		autoOpen: os.Getenv("IPALPHA_OPEN_BROWSERS") != "0" && os.Getenv("CI") == ""}
	var infra, core, web, apps []*Proc
	for _, sp := range s.Specs {
		p := NewProc(sp)
		switch {
		case sp.Group != "":
			apps = append(apps, p)
		case sp.Kind == KindInfra:
			infra = append(infra, p)
		case sp.Kind == KindAPI:
			core = append(core, p)
		default:
			web = append(web, p)
		}
	}
	// Apps outside core: API rows before their web rows, grouped by app in catalog order.
	c.Procs = append(append(append(infra, core...), web...), apps...)
	return c
}

// StartAuto starts every autostart row, watches infra health and opens remembered pages.
func (c *Controller) StartAuto() {
	for _, p := range c.Procs {
		if p.Spec.Autostart {
			p.Start(c.Events, c.LogDir)
			time.Sleep(60 * time.Millisecond)
		}
	}
	c.recordPIDs()
	go c.watchInfra()
	if c.autoOpen {
		go c.openRemembered()
	}
}

func (c *Controller) recordPIDs() {
	pids := map[string]int{}
	for _, p := range c.Procs {
		if g := p.Group(); g > 0 {
			pids[p.Spec.ID] = g // infra log followers too: a crashed panel must not leave them
		}
	}
	RecordPIDs(c.S.W, pids)
}

// OnExit is called by the UI when a process exits.
func (c *Controller) OnExit(id string) { c.recordPIDs() }

func (c *Controller) watchInfra() {
	for {
		health := c.S.Engine.Health()
		for _, p := range c.Procs {
			if p.Spec.Kind == KindInfra {
				p.SetHealth(health[p.Spec.Service])
			}
		}
		select {
		case <-c.ctx.Done():
			return
		case <-time.After(3 * time.Second):
		}
	}
}

// openRemembered opens each remembered page once its server answers (at most two minutes).
func (c *Controller) openRemembered() {
	deadline := time.Now().Add(2 * time.Minute)
	client := &http.Client{Timeout: 800 * time.Millisecond, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	for time.Now().Before(deadline) {
		pending := 0
		for _, p := range c.Procs {
			if p.Spec.URL == "" || catalog.NeverAutoOpen[p.Spec.ID] || !c.S.W.Settings.BrowserSelected(p.Spec.Browser()) {
				continue
			}
			c.mu.Lock()
			done := c.opened[p.Spec.ID]
			c.mu.Unlock()
			if done {
				continue
			}
			pending++
			probe := p.Spec.ReadyURL
			if probe == "" {
				probe = p.Spec.URL
			}
			resp, err := client.Get(probe)
			if err != nil {
				continue
			}
			resp.Body.Close()
			if resp.StatusCode >= 200 && resp.StatusCode < 400 {
				c.mu.Lock()
				c.opened[p.Spec.ID] = true
				c.mu.Unlock()
				if err := sys.OpenURL(p.Spec.URL); err != nil {
					p.Note(i18n.T("panel_open_failed", p.Spec.URL))
				}
			}
		}
		if pending == 0 {
			return
		}
		select {
		case <-c.ctx.Done():
			return
		case <-time.After(700 * time.Millisecond):
		}
	}
}

func (c *Controller) remember(id string, on bool) {
	sel := c.S.W.Settings.Browsers()
	var out []string
	for _, s := range sel {
		if s != id {
			out = append(out, s)
		}
	}
	if on {
		out = append(out, id)
	}
	c.S.W.Settings.SetBrowsers(out)
	if err := c.S.W.Save(); err != nil {
		ui.Raw("save settings: %v", err)
	}
}

// Toggle starts or stops a row (web apps are remembered for the next ./run).
func (c *Controller) Toggle(p *Proc) string {
	if p.Spec.Kind == KindInfra {
		return c.Restart(p)
	}
	st, _, _, _, _ := p.Snapshot()
	if st == Running || st == Starting {
		p.Stop()
		if p.Spec.Kind == KindWeb {
			c.remember(p.Spec.ID, false)
		}
		c.recordPIDs()
		return i18n.T("panel_stopped", p.Spec.Name)
	}
	if p.Spec.Missing != "" {
		return p.Spec.Missing
	}
	if msg := c.ensurePort(p); msg != "" {
		p.Note(msg)
	}
	p.Start(c.Events, c.LogDir)
	if p.Spec.Kind == KindWeb {
		c.remember(p.Spec.ID, true)
	}
	c.recordPIDs()
	return i18n.T("panel_started", p.Spec.Name)
}

// Restart restarts a row (infra: the container).
func (c *Controller) Restart(p *Proc) string {
	if p.Spec.Kind == KindInfra {
		if err := c.S.Engine.Restart(p.Spec.Service); err != nil {
			return i18n.T("panel_restart_failed", p.Spec.Name, firstLine(err.Error()))
		}
		p.Restart(c.Events, c.LogDir)
		return i18n.T("panel_restarted", p.Spec.Name)
	}
	if p.Spec.Missing != "" {
		return p.Spec.Missing
	}
	p.Stop()
	if msg := c.ensurePort(p); msg != "" {
		p.Note(msg)
	}
	p.Start(c.Events, c.LogDir)
	if p.Spec.Kind == KindWeb {
		c.remember(p.Spec.ID, true)
	}
	c.recordPIDs()
	return i18n.T("panel_restarted", p.Spec.Name)
}

// StopRemember stops a row and forgets a web app/page for the next ./run.
func (c *Controller) StopRemember(p *Proc) string {
	if p.Spec.Kind == KindInfra {
		if p.Spec.URL != "" {
			c.remember(p.Spec.Browser(), false)
			return i18n.T("panel_page_forgotten", p.Spec.Name)
		}
		return i18n.T("panel_infra_stays")
	}
	p.Stop()
	if p.Spec.Kind == KindWeb {
		c.remember(p.Spec.ID, false)
	}
	c.recordPIDs()
	return i18n.T("panel_stopped", p.Spec.Name)
}

// Open opens a row's page and remembers it.
func (c *Controller) Open(p *Proc) string {
	if p.Spec.URL == "" {
		return i18n.T("panel_no_page", p.Spec.Name)
	}
	if p.Spec.Kind == KindWeb {
		if st, _, _, _, _ := p.Snapshot(); st != Running && st != Starting {
			c.Toggle(p)
		}
	}
	c.remember(p.Spec.Browser(), true)
	go func() {
		deadline := time.Now().Add(60 * time.Second)
		for time.Now().Before(deadline) {
			if resp, err := http.Get(p.Spec.ReadyURL); err == nil {
				resp.Body.Close()
				break
			}
			if p.Spec.ReadyURL == "" {
				break
			}
			time.Sleep(500 * time.Millisecond)
		}
		if err := sys.OpenURL(p.Spec.URL); err != nil {
			p.Note(i18n.T("panel_open_failed", p.Spec.URL))
		}
	}()
	return i18n.T("panel_opening", p.Spec.URL)
}

// ensurePort moves a stopped process to a free port when its port was taken meanwhile.
func (c *Controller) ensurePort(p *Proc) string {
	if p.Spec.Port == 0 || p.Spec.Kind == KindInfra || os.Getenv("IPALPHA_TEST_NO_PORT_PROBE") == "1" {
		return ""
	}
	if !sys.PortBusy(p.Spec.Port) {
		return ""
	}
	return c.movePort(p)
}

// FixPort moves a row to the next free port right now (key p).
func (c *Controller) FixPort(p *Proc) string {
	if p.Spec.Port == 0 || p.Spec.Kind == KindInfra {
		return i18n.T("panel_no_port", p.Spec.Name)
	}
	p.Stop()
	msg := c.movePort(p)
	p.Start(c.Events, c.LogDir)
	c.recordPIDs()
	return msg
}

// movePort picks a new port for one process and refreshes every spec that points at it.
func (c *Controller) movePort(p *Proc) string {
	old := p.Spec.Port
	taken := map[int]bool{}
	for _, q := range c.Procs {
		taken[q.Spec.Port] = true
	}
	np, err := sys.FreePort(old+1, func(x int) bool { return taken[x] || catalog.IsDefaultPort(x) })
	if err != nil {
		return err.Error()
	}
	plan := c.S.Plan
	plan.Actual[p.Spec.PortKey] = np
	plan.Moves = append(plan.Moves, ports.Move{Key: p.Spec.PortKey, Preferred: old, Actual: np})
	c.S.Plan = plan
	_ = plan.Save(c.S.W.StateDir())
	rw := ports.NewRewriter(plan)
	fresh := buildSpecs(c.S.W, c.S.Engine, plan, rw)
	byID := map[string]*Spec{}
	for _, s := range fresh {
		byID[s.ID] = s
	}
	var affected []string
	for _, q := range c.Procs {
		ns := byID[q.Spec.ID]
		if ns == nil || q.Spec.Kind == KindInfra {
			continue
		}
		refsOld := q.Spec.Refs[old]
		q.Spec.Env, q.Spec.Refs, q.Spec.Port, q.Spec.URL, q.Spec.ReadyURL = ns.Env, ns.Refs, ns.Port, ns.URL, ns.ReadyURL
		if refsOld && q != p {
			if st, _, _, _, _ := q.Snapshot(); st == Running || st == Starting {
				affected = append(affected, q.Spec.Name)
			}
		}
	}
	msg := i18n.T("run_port_moved", p.Spec.ID, old, np)
	c.S.Notes = append(c.S.Notes, msg)
	if len(affected) > 0 {
		msg += " · " + i18n.T("panel_restart_peers", strings.Join(affected, ", "))
	}
	return msg
}

// KillPortHolder stops whoever else listens on the row's port, then restarts the row (key K).
func (c *Controller) KillPortHolder(p *Proc) string {
	if p.Spec.Port == 0 || p.Spec.Kind == KindInfra {
		return i18n.T("panel_no_port", p.Spec.Name)
	}
	own := p.PID()
	p.Stop()
	n, err := sys.KillHolders(p.Spec.Port, own)
	if err != nil {
		p.Note(i18n.T("panel_kill_failed", err.Error()))
	}
	time.Sleep(200 * time.Millisecond)
	p.Start(c.Events, c.LogDir)
	c.recordPIDs()
	return i18n.T("panel_killed", n, p.Spec.Port)
}

// CopyError copies a ready-to-paste error report (for a colleague or an AI assistant).
func (c *Controller) CopyError(p *Proc) string {
	_, code, _, _, crash := p.Snapshot()
	lines := p.Lines()
	if len(lines) > 80 {
		lines = lines[len(lines)-80:]
	}
	var b strings.Builder
	fmt.Fprintf(&b, "IPAlpha ./run — %s (%s) on %s\n", p.Spec.Name, p.Spec.ID, sys.OSName())
	fmt.Fprintf(&b, "status: %s, exit %d, port %d\n", statusTextPlain(p), code, p.Spec.Port)
	if crash != nil {
		fmt.Fprintf(&b, "diagnosis: %s\nsuggestion: %s\n", crash.Summary, crash.Fix)
	}
	b.WriteString("\nlast output:\n")
	for _, l := range lines {
		b.WriteString(ansi.Strip(l) + "\n")
	}
	if err := sys.Clipboard(b.String()); err != nil {
		return i18n.T("panel_copy_failed")
	}
	return i18n.T("panel_error_copied")
}

func statusTextPlain(p *Proc) string {
	st, _, known, ready, _ := p.Snapshot()
	switch st {
	case Running:
		if known && ready {
			return "ready"
		}
		return "running, not ready"
	case Starting:
		return "starting"
	case Exited:
		return "exited"
	}
	return "stopped"
}

// StopAll stops every process (infra containers keep running for a fast next start).
func (c *Controller) StopAll() {
	c.cancel()
	var wg sync.WaitGroup
	for _, p := range c.Procs {
		if p.Spec.Kind == KindInfra {
			p.Stop()
			continue
		}
		wg.Add(1)
		go func(p *Proc) { defer wg.Done(); p.Stop() }(p)
	}
	wg.Wait()
	RecordPIDs(c.S.W, map[string]int{})
}

// pending lists autostart processes that are not ready yet.
func (c *Controller) pending() []*Proc {
	var out []*Proc
	for _, p := range c.Procs {
		if p.Spec.Kind == KindInfra || !p.Spec.Autostart || p.Spec.Missing != "" {
			continue
		}
		if !p.Ready() {
			out = append(out, p)
		}
	}
	return out
}

// Find a row by id.
func (c *Controller) Find(id string) *Proc {
	for _, p := range c.Procs {
		if p.Spec.ID == id {
			return p
		}
	}
	return nil
}

// Port of a row as text.
func (p *Proc) PortText() string {
	if p.Spec.Port == 0 {
		return ""
	}
	return ":" + strconv.Itoa(p.Spec.Port)
}
