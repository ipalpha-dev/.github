package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/ipalpha-dev/tooling/internal/catalog"
	"github.com/ipalpha-dev/tooling/internal/i18n"
	"github.com/ipalpha-dev/tooling/internal/infra"
	"github.com/ipalpha-dev/tooling/internal/materialize"
	"github.com/ipalpha-dev/tooling/internal/panel"
	"github.com/ipalpha-dev/tooling/internal/ports"
	"github.com/ipalpha-dev/tooling/internal/run"
	"github.com/ipalpha-dev/tooling/internal/sys"
	"github.com/ipalpha-dev/tooling/internal/ui"
	"github.com/ipalpha-dev/tooling/internal/workspace"
)

func runCmd() *cobra.Command {
	var plain, skipDeps, noBrowser bool
	var only []string
	var untilReady time.Duration
	c := &cobra.Command{
		Use:   "run",
		Short: i18n.T("cmd_run"),
		Long:  i18n.T("cmd_run_long"),
		RunE: func(cmd *cobra.Command, args []string) error {
			w, err := open("run")
			if err != nil {
				return err
			}
			if noBrowser {
				os.Setenv("IPALPHA_OPEN_BROWSERS", "0")
			}
			return doRun(w, plain || untilReady > 0 || !ui.Interactive() || os.Getenv("IPALPHA_RUNNER") == "background", skipDeps, only, untilReady)
		},
	}
	c.Flags().BoolVar(&plain, "plain", false, i18n.T("flag_plain"))
	c.Flags().BoolVar(&skipDeps, "skip-deps", false, i18n.T("flag_skip_deps"))
	c.Flags().BoolVar(&noBrowser, "no-browser", false, i18n.T("flag_no_browser"))
	c.Flags().StringSliceVar(&only, "only", nil, i18n.T("flag_only"))
	c.Flags().DurationVar(&untilReady, "until-ready", 0, i18n.T("flag_until_ready"))
	// ./run stop and ./run apps keep working as before (aliases of ./stop and ./apps).
	c.AddCommand(stopCmd(), appsCmd())
	return c
}

func doRun(w *workspace.Workspace, plain, skipDeps bool, only []string, untilReady time.Duration) error {
	if owner := run.RunningOwner(w); owner != 0 {
		return ui.NewProblem(i18n.T("run_title"), i18n.T("run_already_running", owner), i18n.T("run_fix_other_terminal"), "./stop")
	}
	// Keep the generated files current (a binary updated by ./pull rewrites wrappers and assets).
	if err := materialize.Write(w, self()); err != nil {
		ui.Raw("materialize: %v", err)
	}
	ui.Header(i18n.T("run_title"))
	var sess *run.Session
	err := ui.Spinner(i18n.T("run_preparing"), func() error {
		var err error
		sess, err = run.Prepare(w, run.Options{SkipDeps: skipDeps, Progress: func(s string) { ui.Raw("%s", s) }})
		return err
	})
	if err != nil {
		return err
	}
	if len(only) > 0 {
		keep := map[string]bool{}
		for _, o := range only {
			keep[o] = true
		}
		for _, s := range sess.Specs {
			if s.Kind != run.KindInfra {
				s.Autostart = keep[s.ID]
			}
		}
	}
	ctl := run.NewController(sess)
	if plain {
		return run.PlainUntil(ctl, untilReady)
	}
	return panel.Run(sess, ctl)
}

func stopCmd() *cobra.Command {
	var purge, volumes bool
	c := &cobra.Command{
		Use:   "stop",
		Short: i18n.T("cmd_stop"),
		Long:  i18n.T("cmd_stop_long"),
		RunE: func(cmd *cobra.Command, args []string) error {
			w, err := open("stop")
			if err != nil {
				return err
			}
			if volumes {
				purge = true
				if !ui.TypeToConfirm(i18n.T("stop_confirm_volumes"), w.Settings.InfraName) {
					return ui.ErrCancelled
				}
			}
			if owner := run.RunningOwner(w); owner != 0 {
				ui.Info(i18n.T("stop_stopping_run", owner))
				sys.KillPID(owner)
			}
			eng := infra.New(w)
			if eng.Runtime == "" {
				return ui.NewProblem(i18n.T("cmd_stop"), i18n.T("infra_no_runtime"))
			}
			if err := ui.Spinner(i18n.T("stop_running"), func() error { return eng.Stop(purge, volumes) }); err != nil {
				return err
			}
			ui.Done(i18n.T("stop_done"))
			return nil
		},
	}
	c.Flags().BoolVar(&purge, "purge", false, i18n.T("flag_purge"))
	c.Flags().BoolVar(&volumes, "volumes", false, i18n.T("flag_volumes"))
	return c
}

func statusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: i18n.T("cmd_status"),
		RunE: func(cmd *cobra.Command, args []string) error {
			w, err := open("status")
			if err != nil {
				return err
			}
			ui.Header("status")
			plan, ok := ports.Load(w.StateDir())
			if !ok {
				plan = ports.Plan{Actual: map[string]int{}}
				for _, k := range catalog.PortKeys() {
					plan.Actual[k] = w.Settings.Port(k)
				}
			}
			owner := run.RunningOwner(w)
			eng := infra.New(w)
			health := eng.Health()
			ui.Section(i18n.T("panel_section_infra"))
			for _, s := range catalog.InfraServices {
				h := health[s.Service]
				if h == "" {
					h = "stopped"
				}
				ui.Item(s.Display, fmt.Sprintf(":%d  %s", plan.Port(s.Key), i18n.T("health_"+h)))
			}
			for _, group := range []struct {
				title string
				kind  catalog.Kind
			}{{i18n.T("panel_section_core"), catalog.KindAPI}, {i18n.T("panel_section_web"), catalog.KindWeb}} {
				ui.Section(group.title)
				for _, r := range catalog.ByKind(group.kind) {
					p := plan.Port(r.Name)
					state := ui.Muted.Render(i18n.T("state_stopped"))
					if sys.PortBusy(p) {
						if owner != 0 {
							state = ui.OK.Render(i18n.T("state_listening"))
						} else {
							state = ui.Warn.Render(i18n.T("state_port_other"))
						}
					}
					url := fmt.Sprintf("http://localhost:%d", p)
					ui.Item(r.Label(), fmt.Sprintf("%-28s %s", url, state))
				}
			}
			for _, m := range plan.Moves {
				ui.Warning(i18n.T("run_port_moved", m.Key, m.Preferred, m.Actual))
			}
			if owner != 0 {
				ui.Info(i18n.T("status_run_open", owner))
			} else {
				ui.Info(i18n.T("status_run_closed"))
			}
			ui.Info(i18n.T("status_logs", filepath.Join(w.LogsDir(), "run")))
			return nil
		},
	}
}

func logsCmd() *cobra.Command {
	var follow bool
	c := &cobra.Command{
		Use:   "logs [service]",
		Short: i18n.T("cmd_logs"),
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			w, err := open("logs")
			if err != nil {
				return err
			}
			dir := filepath.Join(w.LogsDir(), "run")
			if len(args) == 0 {
				ui.Header("logs")
				entries, _ := os.ReadDir(dir)
				for _, e := range entries {
					ui.Item(strings.TrimSuffix(e.Name(), ".log"), filepath.Join(dir, e.Name()))
				}
				cmds, _ := filepath.Glob(filepath.Join(w.LogsDir(), "*.log"))
				sort.Sort(sort.Reverse(sort.StringSlice(cmds)))
				if len(cmds) > 8 {
					cmds = cmds[:8]
				}
				if len(cmds) > 0 {
					ui.Section(i18n.T("logs_commands"))
					for _, f := range cmds {
						ui.Info(f)
					}
				}
				return nil
			}
			name := args[0]
			for _, s := range catalog.InfraServices {
				if s.Service == name || s.Key == name {
					eng := infra.New(w)
					bin, a, env := eng.LogsCmd(s.Service)
					if !follow {
						a = filterFollow(a)
					}
					c := exec.Command(bin, a...)
					c.Env = append(os.Environ(), env...)
					c.Stdout, c.Stderr = os.Stdout, os.Stderr
					return c.Run()
				}
			}
			path := filepath.Join(dir, name+".log")
			if follow {
				return tailFollow(path)
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return ui.NewProblem(i18n.T("cmd_logs"), i18n.T("logs_missing", name), "./run")
			}
			os.Stdout.Write(data)
			return nil
		},
	}
	c.Flags().BoolVarP(&follow, "follow", "f", false, i18n.T("flag_follow"))
	return c
}

func filterFollow(a []string) []string {
	var out []string
	for _, x := range a {
		if x != "--follow" {
			out = append(out, x)
		}
	}
	return out
}

func appsCmd() *cobra.Command {
	c := &cobra.Command{
		Use:     "apps",
		Aliases: []string{"browsers"},
		Short:   i18n.T("cmd_apps"),
		Long:    i18n.T("cmd_apps_long"),
		RunE: func(cmd *cobra.Command, args []string) error {
			w, err := open("apps")
			if err != nil {
				return err
			}
			if !ui.Interactive() {
				fmt.Println(strings.Join(w.Settings.Browsers(), " "))
				return nil
			}
			var opts []ui.Option
			for _, r := range catalog.Webs() {
				opts = append(opts, ui.Option{Value: r.Name, Label: r.Label(), Hint: fmt.Sprintf("http://localhost:%d", w.Settings.Port(r.Name))})
			}
			opts = append(opts, ui.Option{Value: "mailpit", Label: "Mailpit", Hint: i18n.T("apps_mailpit_hint")})
			opts = append(opts, ui.Option{Value: "rabbitmq-mgmt", Label: "RabbitMQ UI", Hint: fmt.Sprintf("http://localhost:%d", w.Settings.Port("rabbitmq-mgmt"))})
			sel, err := ui.MultiSelect(i18n.T("apps_choose"), i18n.T("apps_choose_desc"), opts, w.Settings.Browsers())
			if err != nil {
				return err
			}
			w.Settings.SetBrowsers(sel)
			if err := w.Save(); err != nil {
				return err
			}
			ui.Done(i18n.T("apps_saved"))
			return nil
		},
	}
	c.AddCommand(&cobra.Command{Use: "set [app…]", Short: i18n.T("cmd_apps_set"), RunE: func(cmd *cobra.Command, args []string) error {
		w, err := open("apps")
		if err != nil {
			return err
		}
		valid := map[string]bool{"mailpit": true, "rabbitmq-mgmt": true}
		for _, r := range catalog.Webs() {
			valid[r.Name] = true
		}
		for _, a := range args {
			if !valid[a] {
				return ui.NewProblem(i18n.T("cmd_apps"), i18n.T("apps_unknown", a), "./run apps")
			}
		}
		w.Settings.SetBrowsers(args)
		if err := w.Save(); err != nil {
			return err
		}
		ui.Done(i18n.T("apps_saved"))
		return nil
	}})
	c.AddCommand(&cobra.Command{Use: "defaults", Short: i18n.T("cmd_apps_defaults"), RunE: func(cmd *cobra.Command, args []string) error {
		w, err := open("apps")
		if err != nil {
			return err
		}
		w.Settings.SetBrowsers(catalog.DefaultBrowserApps)
		if err := w.Save(); err != nil {
			return err
		}
		ui.Done(i18n.T("apps_saved"))
		return nil
	}})
	c.AddCommand(&cobra.Command{Use: "list", Short: i18n.T("cmd_apps_list"), RunE: func(cmd *cobra.Command, args []string) error {
		w, err := open("apps")
		if err != nil {
			return err
		}
		fmt.Println(strings.Join(w.Settings.Browsers(), " "))
		return nil
	}})
	return c
}
