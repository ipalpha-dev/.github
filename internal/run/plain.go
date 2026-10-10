package run

import (
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/ipalpha-dev/tooling/internal/i18n"
	"github.com/ipalpha-dev/tooling/internal/ui"
)

// Plain runs every process with prefixed output (no terminal UI). Used without a TTY, in CI and
// with ./run --plain. Ctrl+C stops everything. A crash prints its diagnosis box.
func Plain(c *Controller) error { return PlainUntil(c, 0) }

// PlainUntil with a deadline > 0 returns as soon as every autostart process is ready (nil), or
// when one crashes / the deadline passes (error). Used by CI and `./run --until-ready`.
func PlainUntil(c *Controller, deadline time.Duration) error {
	var until <-chan time.Time
	if deadline > 0 {
		until = time.After(deadline)
	}
	check := time.NewTicker(time.Second)
	defer check.Stop()
	width := 0
	for _, p := range c.Procs {
		if n := len(p.Spec.ID); n > width {
			width = n
		}
	}
	sig := make(chan os.Signal, 2)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(sig)

	go c.StartAuto()
	ui.Println(ui.Muted.Render(i18n.T("plain_started", c.LogDir)))
	for _, n := range c.S.Notes {
		ui.Warning(n)
	}
	for {
		select {
		case <-sig:
			ui.Println("\n" + i18n.T("panel_stopping_all"))
			c.StopAll()
			return nil
		case <-until:
			var waiting []string
			for _, p := range c.pending() {
				waiting = append(waiting, p.Spec.ID)
			}
			c.StopAll()
			return ui.NewProblem(i18n.T("run_title"), i18n.T("plain_waiting", strings.Join(waiting, ", ")), i18n.T("crash_fix_doctor"))
		case <-check.C:
			if deadline > 0 && len(c.pending()) == 0 {
				ui.Done(i18n.T("plain_all_ready"))
				c.StopAll()
				return nil
			}
		case e := <-c.Events:
			p := c.Find(e.ID)
			if p == nil {
				continue
			}
			if e.Line != "" {
				if p.Spec.Kind == KindInfra {
					continue // container logs stay in their file (./logs mongo)
				}
				fmt.Printf("%s │ %s\n", ui.Muted.Render(fmt.Sprintf("%-*s", width, e.ID)), e.Line)
				continue
			}
			if e.Exit {
				c.OnExit(e.ID)
				st, code, _, _, crash := p.Snapshot()
				if st != Exited {
					continue
				}
				if crash != nil && (code != 0 || p.Spec.Kind != KindInfra) {
					prob := &ui.Problem{Step: p.Spec.Name + " " + i18n.T("state_exit", code), Cause: crash.Summary,
						Tail: crash.Lines, Fix: []string{crash.Fix}, Log: c.LogDir + string(os.PathSeparator) + p.Spec.ID + ".log"}
					if deadline > 0 && p.Spec.Kind != KindInfra {
						c.StopAll()
						return prob
					}
					ui.Report(prob)
				}
			}
		case <-time.After(30 * time.Second):
			total := 0
			for _, p := range c.Procs {
				if p.Spec.Kind != KindInfra && p.Spec.Autostart && p.Spec.Missing == "" {
					total++
				}
			}
			var waiting []string
			for _, p := range c.pending() {
				waiting = append(waiting, p.Spec.ID)
			}
			ready := total - len(waiting)
			line := fmt.Sprintf("%s %d/%d", i18n.T("panel_ready_count"), ready, total)
			if len(waiting) > 0 && len(waiting) <= 6 {
				line += " · " + i18n.T("plain_waiting", strings.Join(waiting, ", "))
			}
			ui.Info(ansi.Strip(line))
		}
	}
}
