package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/ipalpha-dev/tooling/internal/ai"
	"github.com/ipalpha-dev/tooling/internal/catalog"
	"github.com/ipalpha-dev/tooling/internal/deps"
	"github.com/ipalpha-dev/tooling/internal/env"
	"github.com/ipalpha-dev/tooling/internal/envfile"
	"github.com/ipalpha-dev/tooling/internal/i18n"
	"github.com/ipalpha-dev/tooling/internal/infra"
	"github.com/ipalpha-dev/tooling/internal/ports"
	"github.com/ipalpha-dev/tooling/internal/repos"
	"github.com/ipalpha-dev/tooling/internal/sys"
	"github.com/ipalpha-dev/tooling/internal/tools"
	"github.com/ipalpha-dev/tooling/internal/ui"
)

type check struct {
	name   string
	status string // ok | warn | fail
	detail string
	fix    string
}

func doctorCmd() *cobra.Command {
	var testAI, copyReport bool
	c := &cobra.Command{
		Use:   "doctor",
		Short: i18n.T("cmd_doctor"),
		Long:  i18n.T("cmd_doctor_long"),
		RunE: func(cmd *cobra.Command, args []string) error {
			w, werr := open("doctor")
			ui.Header("doctor")
			var checks []check
			add := func(c check) {
				checks = append(checks, c)
				icon := map[string]string{"ok": ui.OK.Render("✔"), "info": ui.Muted.Render("·"), "warn": ui.Warn.Render("!"), "fail": ui.Bad.Render("✖")}[c.status]
				ui.Printf("%s %-28s %s\n", icon, c.name, c.detail)
				if c.fix != "" && (c.status == "warn" || c.status == "fail") {
					ui.Printf("  %s %s\n", ui.Muted.Render("→"), c.fix)
				}
			}
			ui.Section(i18n.T("doctor_system"))
			add(check{name: "OS", status: "ok", detail: fmt.Sprintf("%s %s/%s · ipalpha %s", sys.OSName(), runtime.GOOS, runtime.GOARCH, Version)})
			mgr := tools.DetectManager()
			for _, st := range tools.CheckAll() {
				c := check{name: st.Tool.Name, status: "ok", detail: st.Version}
				if st.Err != nil {
					c.status, c.detail = "fail", firstLine(st.Err.Error())
					if !st.Tool.Required {
						c.status = "warn"
					}
					if argv := tools.InstallCommand(mgr, st.Tool); argv != nil {
						c.fix = strings.Join(argv, " ")
					} else {
						c.fix = st.Tool.URL
					}
				}
				add(c)
			}
			if sys.Windows {
				if sh := sys.ScriptShell(); sh == "" {
					add(check{name: "Git Bash", status: "fail", detail: i18n.T("doctor_no_bash"), fix: "winget install Git.Git"})
				} else {
					add(check{name: "Git Bash", status: "ok", detail: sh})
				}
				if out, err := sys.Output("", "git", "config", "--get", "core.longpaths"); err != nil || out != "true" {
					add(check{name: "git core.longpaths", status: "warn", detail: i18n.T("doctor_longpaths"), fix: "git config --global core.longpaths true"})
				}
			}
			if werr != nil {
				ui.Report(werr)
				return exitCode(1)
			}

			ui.Section(i18n.T("doctor_workspace"))
			add(check{name: i18n.T("doctor_folder"), status: "ok", detail: w.Root})
			eng := infra.New(w)
			if eng.Runtime == "" {
				add(check{name: i18n.T("doctor_runtime"), status: "fail", detail: i18n.T("infra_no_runtime"), fix: i18n.T("infra_fix_install_docker")})
			} else if err := eng.EnsureRunning(); err != nil {
				add(check{name: i18n.T("doctor_runtime"), status: "fail", detail: firstLine(err.Error()), fix: strings.Join(dockerFixes(err), " · ")})
			} else {
				add(check{name: i18n.T("doctor_runtime"), status: "ok", detail: eng.Runtime})
				health := eng.Health()
				for _, s := range []string{"mongo", "redis", "rabbitmq", "mailpit"} {
					h := health[s]
					st := "ok"
					if h != "healthy" && h != "running" {
						st = "warn"
					}
					if h == "" {
						h = "stopped"
					}
					add(check{name: "  " + s, status: st, detail: i18n.T("health_" + h), fix: "./run"})
				}
			}
			for _, r := range catalog.Repos {
				dir := r.Path(w.Root)
				if _, err := os.Stat(dir); err != nil {
					if r.Kind == catalog.KindOptional {
						continue
					}
					add(check{name: r.Rel(), status: "fail", detail: i18n.T("doctor_not_cloned"), fix: "./pull"})
					continue
				}
				if !repos.IsGit(dir) {
					add(check{name: r.Rel(), status: "warn", detail: i18n.T("repos_not_git")})
					continue
				}
				detail := ""
				if b, err := sys.Git(dir, "branch", "--show-current"); err == nil {
					detail = b
				}
				st := "ok"
				fix := ""
				if r.Runs() || r.Kind == catalog.KindLibrary {
					if _, err := os.Stat(filepath.Join(dir, "package.json")); err == nil && deps.NeedsInstall(dir) {
						st, fix = "warn", i18n.T("doctor_deps_fix")
						detail += " · " + i18n.T("doctor_deps_missing")
					}
				}
				if r.HasEnv() {
					if _, err := os.Stat(filepath.Join(dir, ".env")); err != nil {
						st, fix = "warn", "./pull"
						detail += " · " + i18n.T("doctor_no_env")
					}
				}
				add(check{name: r.Rel(), status: st, detail: detail, fix: fix})
			}
			n, p := env.Superuser(w)
			if p == "" {
				add(check{name: i18n.T("doctor_account"), status: "warn", detail: i18n.T("doctor_no_account"), fix: "./set-keys"})
			} else {
				add(check{name: i18n.T("doctor_account"), status: "ok", detail: n + " " + p})
			}
			if auth := envfile.Read(filepath.Join(w.Repo("auth-api"), ".env")); auth["SEED_CLIENTS_JSON"] == "" && repos.IsGit(w.Repo("auth-api")) {
				add(check{name: "SEED_CLIENTS_JSON", status: "warn", detail: i18n.T("doctor_no_clients"), fix: "./pull"})
			}

			ui.Section(i18n.T("doctor_ports"))
			plan, _ := ports.Load(w.StateDir())
			owned := eng.OwnedPorts()
			for _, k := range catalog.PortKeys() {
				pref := w.Settings.Port(k)
				if sys.PortBusy(pref) && !owned[pref] {
					holder := ""
					for _, h := range sys.PortHolders(pref) {
						holder = fmt.Sprintf("%s (pid %d)", h.Command, h.PID)
						break
					}
					st := "info"
					detail := i18n.T("doctor_port_busy", pref, holder)
					if plan.Actual == nil || plan.Actual[k] == pref {
						st = "warn"
					}
					add(check{name: k, status: st, detail: detail, fix: i18n.T("doctor_port_fix")})
				}
			}

			ui.Section(i18n.T("doctor_ai"))
			cfg := publishCfg(w)
			switch {
			case !cfg.Enabled():
				add(check{name: "AI", status: "warn", detail: i18n.T("ai_none"), fix: "ipalpha ai"})
			case testAI:
				var err error
				_ = ui.Spinner(i18n.T("ai_testing", cfg.Label()), func() error { err = ai.Test(cfg); return nil })
				if err != nil {
					add(check{name: "AI", status: "fail", detail: firstLine(err.Error()), fix: "ipalpha ai"})
				} else {
					add(check{name: "AI", status: "ok", detail: cfg.Label()})
				}
			default:
				name := cfg.CLI
				if p, ok := ai.Find(cfg.CLI); ok {
					name = p.Command
				}
				if cfg.CLI != "custom" && !sys.Has(name) {
					add(check{name: "AI", status: "fail", detail: i18n.T("ai_not_installed", name), fix: "ipalpha ai"})
				} else {
					add(check{name: "AI", status: "ok", detail: cfg.Label() + " · " + i18n.T("doctor_ai_test_hint")})
				}
			}

			fails, warns := 0, 0
			for _, c := range checks {
				switch c.status {
				case "fail":
					fails++
				case "warn":
					warns++
				}
			}
			ui.Println("")
			ui.Info(i18n.T("doctor_summary", fails, warns, ui.Log().Path))
			if copyReport {
				var b strings.Builder
				fmt.Fprintf(&b, "ipalpha doctor %s — %s\n", time.Now().Format(time.RFC3339), sys.OSName())
				for _, c := range checks {
					fmt.Fprintf(&b, "[%s] %s: %s\n", c.status, c.name, c.detail)
				}
				if err := sys.Clipboard(b.String()); err == nil {
					ui.Done(i18n.T("doctor_copied"))
				}
			}
			if fails > 0 {
				return exitCode(1)
			}
			return nil
		},
	}
	c.Flags().BoolVar(&testAI, "ai", false, i18n.T("flag_doctor_ai"))
	c.Flags().BoolVar(&copyReport, "copy", false, i18n.T("flag_doctor_copy"))
	return c
}

func dockerFixes(err error) []string {
	if p, ok := err.(*ui.Problem); ok {
		return p.Fix
	}
	return nil
}

func setKeysCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "set-keys",
		Short: i18n.T("cmd_set_keys"),
		Long:  i18n.T("cmd_set_keys_long"),
		RunE: func(cmd *cobra.Command, args []string) error {
			w, err := open("set-keys")
			if err != nil {
				return err
			}
			ui.Header("set-keys")
			if !ui.Interactive() {
				return ui.NewProblem(i18n.T("cmd_set_keys"), i18n.T("set_keys_tty"))
			}
			ui.Info(i18n.T("set_keys_intro"))
			if err := askSuperuser(w, false); err != nil {
				return err
			}
			email, err := ui.Input(i18n.T("set_keys_email"), i18n.T("set_keys_optional"), envfile.Read(filepath.Join(w.Repo("auth-api"), ".env"))["SUPERUSER_EMAIL"], nil)
			if err != nil {
				return err
			}
			if email != "" {
				if err := setEnvKey(w.Repo("auth-api"), "SUPERUSER_EMAIL", email); err != nil {
					return err
				}
			}
			ui.Println(ui.InfoBox(i18n.T("set_keys_sms_title"), i18n.T("set_keys_sms_note")))
			for _, k := range []struct{ key, label string }{{"SMSBARATO_KEY", "SMS Barato"}, {"COMTELE_TOKEN", "Comtele token"}} {
				v, err := ui.Secret(k.label, i18n.T("set_keys_keep_blank"))
				if err != nil {
					return err
				}
				if v != "" {
					if err := setEnvKey(w.Repo("notifications-api"), k.key, v); err != nil {
						return err
					}
				}
			}
			ui.Done(i18n.T("set_keys_saved"))
			return nil
		},
	}
}

func setEnvKey(dir, key, value string) error {
	f, err := envfile.Load(filepath.Join(dir, ".env"))
	if err != nil {
		return err
	}
	f.Set(key, envfile.Quote(value))
	return f.Save()
}
