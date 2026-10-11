package cli

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"github.com/ipalpha-dev/tooling/internal/ai"
	"github.com/ipalpha-dev/tooling/internal/deps"
	"github.com/ipalpha-dev/tooling/internal/env"
	"github.com/ipalpha-dev/tooling/internal/i18n"
	"github.com/ipalpha-dev/tooling/internal/infra"
	"github.com/ipalpha-dev/tooling/internal/materialize"
	"github.com/ipalpha-dev/tooling/internal/repos"
	"github.com/ipalpha-dev/tooling/internal/sys"
	"github.com/ipalpha-dev/tooling/internal/tools"
	"github.com/ipalpha-dev/tooling/internal/ui"
	"github.com/ipalpha-dev/tooling/internal/workspace"
)

func setupCmd() *cobra.Command {
	var dir string
	var skipTools, skipInstall, yes bool
	c := &cobra.Command{
		Use:   "setup [folder]",
		Short: i18n.T("cmd_setup"),
		Long:  i18n.T("cmd_setup_long"),
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 1 {
				dir = args[0]
			}
			return setup(dir, skipTools || os.Getenv("IPALPHA_SKIP_TOOLS") == "1", skipInstall || os.Getenv("IPALPHA_SKIP_INSTALL") == "1", yes)
		},
	}
	c.Flags().BoolVar(&skipTools, "skip-tools", false, i18n.T("flag_skip_tools"))
	c.Flags().BoolVar(&skipInstall, "skip-install", false, i18n.T("flag_skip_install"))
	c.Flags().BoolVarP(&yes, "yes", "y", false, i18n.T("flag_yes"))
	return c
}

func defaultFolder() string {
	if d := os.Getenv("IPALPHA_TARGET_DIR"); d != "" {
		return d
	}
	if d := os.Getenv("IPALPHA_DEFAULT_ROOT"); d != "" {
		return d
	}
	if root, err := workspace.Find("."); err == nil {
		return root
	}
	wd, _ := os.Getwd()
	return filepath.Join(wd, "IpAlpha")
}

func setup(dir string, skipTools, skipInstall, yes bool) error {
	ui.Header(i18n.T("setup_title"))
	ui.Info(i18n.T("setup_intro", sys.OSName(), i18n.LangName(i18n.Current())))

	// 1. Folder.
	if dir == "" {
		def := defaultFolder()
		if os.Getenv("IPALPHA_TARGET_DIR") != "" || yes {
			dir = def
		} else {
			var err error
			dir, err = ui.Input(i18n.T("setup_folder"), i18n.T("setup_folder_desc"), def, func(s string) error {
				if strings.TrimSpace(s) == "" {
					return errors.New(i18n.T("setup_folder_required"))
				}
				return nil
			})
			if err != nil {
				return err
			}
		}
	}
	dir = expandHome(dir)
	abs, err := filepath.Abs(dir)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(abs, 0o755); err != nil {
		return ui.NewProblem(i18n.T("setup_folder"), i18n.T("setup_folder_create", abs, err.Error()))
	}
	if sys.Windows && len(abs) > 60 {
		ui.Warning(i18n.T("setup_long_path_windows", abs))
	}
	ui.OpenLog(filepath.Join(abs, workspace.DirName, "logs"), "setup")
	ui.Done(i18n.T("setup_folder_ok", abs))

	// 2. Tools.
	if !skipTools {
		ui.Section(i18n.T("setup_tools"))
		if err := ensureTools(yes); err != nil {
			return err
		}
	}

	// 3. Workspace settings (keep a previous setup's choices).
	w, err := workspace.Open(abs)
	if err != nil {
		return err
	}
	if w.Settings.InfraName == "" || w.Settings.InfraName == "ipalpha" {
		w.Settings.InfraName = workspace.InfraNameFor(abs)
	}
	if w.Settings.Runtime == "" {
		w.Settings.Runtime = infra.Detect("")
	}
	if err := os.MkdirAll(w.Dir(), 0o755); err != nil {
		return err
	}
	if err := w.Save(); err != nil {
		return err
	}

	// 4. GitHub access and repositories.
	ui.Section(i18n.T("setup_repos"))
	access, err := ensureGitHub(yes)
	if err != nil {
		return err
	}
	var failed []string
	repos.CloneAll(w.Settings.Org, abs, access, func(o repos.Outcome) {
		switch o.Status {
		case "cloned":
			ui.Item(o.Repo.Rel(), ui.OK.Render(i18n.T("repos_cloned")))
		case "kept":
			ui.Item(o.Repo.Rel(), ui.Muted.Render(i18n.T("repos_kept")))
		case "skipped":
			ui.Item(o.Repo.Rel(), ui.Muted.Render(o.Detail))
		case "failed":
			ui.Item(o.Repo.Rel(), ui.Bad.Render(i18n.T("repos_failed")))
			failed = append(failed, o.Repo.Name+": "+o.Detail)
		}
	})
	if len(failed) > 0 {
		return &ui.Problem{Step: i18n.T("setup_repos"), Cause: i18n.T("setup_clone_failed"), Tail: failed,
			Fix: []string{i18n.T("setup_fix_membership", w.Settings.Org), i18n.T("setup_fix_gh_login"), i18n.T("setup_fix_rerun")}}
	}

	// 5. First sign-in account.
	ui.Section(i18n.T("setup_account"))
	if _, err := env.Prepare(w); err != nil {
		return ui.Wrap(i18n.T("setup_env"), err)
	}
	if err := askSuperuser(w, yes); err != nil {
		return err
	}

	// 6. AI for ./publish.
	ui.Section(i18n.T("setup_ai"))
	if err := chooseAI(w, yes, true); err != nil {
		return err
	}

	// 7. Generated files and dependencies.
	ui.Section(i18n.T("setup_files"))
	if err := materialize.Write(w, self()); err != nil {
		return ui.Wrap(i18n.T("setup_files"), err)
	}
	ui.Done(i18n.T("setup_files_ok"))
	if !skipInstall {
		ui.Section(i18n.T("setup_deps"))
		if err := ui.Spinner(i18n.T("setup_deps_running"), func() error {
			return deps.Run(w, func(s string) { ui.Raw("%s", s) })
		}); err != nil {
			ui.Report(err)
			ui.Warning(i18n.T("setup_deps_later"))
		} else {
			ui.Done(i18n.T("setup_deps_ok"))
		}
	}

	ui.Println("")
	ui.Println(ui.InfoBox(i18n.T("setup_done"),
		i18n.T("setup_done_folder", abs),
		"",
		i18n.T("setup_done_next"),
		"  cd "+quotePath(abs),
		"  "+runHint(),
		"",
		i18n.T("setup_done_more")))
	return nil
}

func expandHome(p string) string {
	if p == "~" || strings.HasPrefix(p, "~/") || strings.HasPrefix(p, `~\`) {
		if h, err := os.UserHomeDir(); err == nil {
			return filepath.Join(h, p[1:])
		}
	}
	return p
}

func quotePath(p string) string {
	if strings.ContainsAny(p, " '\"") {
		return "\"" + p + "\""
	}
	return p
}

// runHint is how to call ./run in the current shell.
func runHint() string {
	if sys.Windows {
		return `.\run`
	}
	return "./run"
}

func ensureTools(yes bool) error {
	mgr := tools.DetectManager()
	for _, st := range tools.CheckAll() {
		t := st.Tool
		if st.Err == nil {
			ui.Item(t.Name, ui.OK.Render("✔ "+st.Version))
			continue
		}
		label := ui.Bad.Render("✖ " + i18n.T("tool_missing"))
		if !t.Required {
			label = ui.Warn.Render("○ " + i18n.T("tool_optional_missing"))
		}
		ui.Item(t.Name, label+ui.Muted.Render("  "+firstLine(st.Err.Error())))
		for {
			argv := tools.InstallCommand(mgr, t)
			opts := []ui.Option{}
			if argv != nil {
				opts = append(opts, ui.Option{Value: "install", Label: i18n.T("tool_install_with", mgr.Name), Hint: strings.Join(argv, " ")})
			}
			opts = append(opts, ui.Option{Value: "manual", Label: i18n.T("tool_installed_myself"), Hint: t.URL})
			if !t.Required {
				opts = append(opts, ui.Option{Value: "skip", Label: i18n.T("tool_skip"), Hint: i18n.T(t.Why)})
			}
			opts = append(opts, ui.Option{Value: "quit", Label: i18n.T("tool_quit")})
			def := "manual"
			if argv != nil {
				def = "install"
			}
			if !t.Required && (yes || !ui.Interactive()) {
				def = "skip"
			}
			if t.Required && !ui.Interactive() && !yes {
				return toolProblem(t, mgr)
			}
			choice, err := ui.Select(i18n.T("tool_missing_title", t.Name), i18n.T(t.Why), opts, def)
			if err != nil {
				return err
			}
			if choice == "quit" {
				return ui.ErrCancelled
			}
			if choice == "skip" {
				break
			}
			if choice == "install" {
				ui.Info("$ " + strings.Join(argv, " "))
				if err := tools.Install(argv); err != nil {
					ui.Warning(i18n.T("tool_install_failed", err.Error()))
				}
			}
			v, err := t.Check()
			if err == nil {
				ui.Item(t.Name, ui.OK.Render("✔ "+v))
				break
			}
			ui.Warning(i18n.T("tool_still_missing", t.Name, firstLine(err.Error())))
			if t.ID == "docker" || t.ID == "node" {
				ui.Info(i18n.T("tool_new_terminal"))
			}
		}
	}
	if runtime.GOOS == "linux" && !sys.IsWSL() && sys.Has("docker") {
		if _, err := sys.Output("", "docker", "info"); err != nil && strings.Contains(err.Error(), "permission denied") {
			ui.Println(ui.WarnBox(i18n.T("tool_docker_group_title"), i18n.T("tool_docker_group")))
		}
	}
	return nil
}

func toolProblem(t tools.Tool, mgr tools.Manager) *ui.Problem {
	fix := []string{}
	if argv := tools.InstallCommand(mgr, t); argv != nil {
		fix = append(fix, strings.Join(argv, " "))
	}
	if t.Manual != "" {
		fix = append(fix, i18n.T(t.Manual))
	}
	fix = append(fix, t.URL)
	return &ui.Problem{Step: i18n.T("setup_tools"), Cause: i18n.T("tool_required_missing", t.Name), Fix: fix}
}

func ensureGitHub(yes bool) (repos.Access, error) {
	if os.Getenv("IPALPHA_CLONE_COMMAND") != "" || os.Getenv("IPALPHA_CLONE_FROM") != "" {
		return repos.Access{SSH: true}, nil
	}
	var a repos.Access
	_ = ui.Spinner(i18n.T("setup_github_check"), func() error { a = repos.Probe(); return nil })
	switch {
	case a.SSH:
		ui.Done(i18n.T("setup_github_ssh"))
		return a, nil
	case a.GH:
		ui.Done(i18n.T("setup_github_gh"))
		return a, nil
	}
	if !sys.Has("gh") {
		return a, ui.NewProblem(i18n.T("setup_github"), i18n.T("setup_github_none"),
			i18n.T("setup_fix_install_gh"), i18n.T("setup_fix_ssh_key"))
	}
	if !ui.Interactive() || yes {
		return a, ui.NewProblem(i18n.T("setup_github"), i18n.T("setup_github_none"), "gh auth login --web --git-protocol https", i18n.T("setup_fix_ssh_key"))
	}
	ok, err := ui.Confirm(i18n.T("setup_github_login"), i18n.T("setup_github_login_desc"), true)
	if err != nil {
		return a, err
	}
	if !ok {
		return a, ui.NewProblem(i18n.T("setup_github"), i18n.T("setup_github_none"), "gh auth login --web --git-protocol https")
	}
	if err := tools.Install([]string{"gh", "auth", "login", "--web", "--git-protocol", "https", "--hostname", "github.com"}); err != nil {
		return a, ui.Wrap(i18n.T("setup_github"), err, "gh auth login")
	}
	_ = tools.Install([]string{"gh", "auth", "setup-git"})
	a = repos.Probe()
	if !a.GH && !a.SSH {
		return a, ui.NewProblem(i18n.T("setup_github"), i18n.T("setup_github_none"), "gh auth status")
	}
	ui.Done(i18n.T("setup_github_gh"))
	return a, nil
}

func askSuperuser(w *workspace.Workspace, yes bool) error {
	name, phone := env.Superuser(w)
	if v := os.Getenv("IPALPHA_SUPERUSER_NAME"); v != "" {
		name = v
	}
	if v := os.Getenv("IPALPHA_SUPERUSER_PHONE"); v != "" {
		phone = v
	}
	if name == "" {
		name = "Joao Silva Costa"
	}
	ui.Info(i18n.T("setup_account_desc"))
	interactive := ui.Interactive() && !yes
	var err error
	if interactive && os.Getenv("IPALPHA_SUPERUSER_NAME") == "" {
		name, err = ui.Input(i18n.T("seed_name"), i18n.T("seed_name_hint"), name, func(s string) error {
			if _, err := env.NormalizeName(s); err != nil {
				return errors.New(i18n.T("seed_name_invalid"))
			}
			return nil
		})
		if err != nil {
			return err
		}
	}
	n, err := env.NormalizeName(name)
	if err != nil {
		return ui.NewProblem(i18n.T("setup_account"), i18n.T("seed_name_invalid"))
	}
	if interactive && os.Getenv("IPALPHA_SUPERUSER_PHONE") == "" {
		phone, err = ui.Input(i18n.T("seed_phone"), i18n.T("seed_phone_hint"), strings.TrimPrefix(phone, "+55"), func(s string) error {
			if _, err := env.NormalizePhone(s); err != nil {
				return errors.New(i18n.T("seed_phone_invalid"))
			}
			return nil
		})
		if err != nil {
			return err
		}
	}
	if phone == "" {
		ui.Warning(i18n.T("setup_account_later"))
		return nil
	}
	p, err := env.NormalizePhone(phone)
	if err != nil {
		return ui.NewProblem(i18n.T("setup_account"), i18n.T("seed_phone_invalid"))
	}
	if err := env.WriteSuperuser(w, n, p); err != nil {
		return ui.Wrap(i18n.T("seed_save_failed"), err)
	}
	ui.Done(i18n.T("setup_account_ok", n, p))
	return nil
}

// chooseAI asks which AI CLI and model ./publish uses, then tests it. Never blocks setup.
func chooseAI(w *workspace.Workspace, yes, _ bool) error {
	s := w.Settings
	if v := os.Getenv("IPALPHA_AI_CLI"); v != "" {
		s.AICLI, s.AIModel = v, os.Getenv("IPALPHA_AI_MODEL")
		return w.Save()
	}
	installed := ai.Installed()
	if !ui.Interactive() || yes {
		if s.AICLI == "" {
			s.AICLI = "none"
			if len(installed) > 0 {
				s.AICLI = installed[0].ID
			}
		}
		ui.Info(i18n.T("ai_selected", ai.Config{CLI: s.AICLI, Model: s.AIModel, Command: s.AICommand}.Label()))
		return w.Save()
	}
	ui.Info(i18n.T("ai_explain"))
	// Only the CLIs already on this computer; every preset only when none is installed.
	var opts []ui.Option
	desc := i18n.T("ai_choose_desc")
	for _, p := range installed {
		opts = append(opts, ui.Option{Value: p.ID, Label: p.Name, Hint: "✔ " + i18n.T("ai_installed")})
	}
	if len(installed) == 0 {
		desc = i18n.T("ai_choose_desc_none")
		for _, p := range ai.Presets {
			opts = append(opts, ui.Option{Value: p.ID, Label: p.Name, Hint: i18n.T("ai_not_installed_hint")})
		}
	}
	opts = append(opts,
		ui.Option{Value: "custom", Label: i18n.T("ai_custom"), Hint: i18n.T("ai_custom_hint")},
		ui.Option{Value: "none", Label: i18n.T("ai_none_option"), Hint: i18n.T("ai_none_hint")})
	def := s.AICLI
	if !hasOption(opts, def) {
		def = "none"
		if len(installed) > 0 {
			def = installed[0].ID
		}
	}
	for {
		cli, err := ui.Select(i18n.T("ai_choose"), desc, opts, def)
		if err != nil {
			return err
		}
		cfg := ai.Config{CLI: cli}
		switch cli {
		case "none":
			s.AICLI, s.AIModel, s.AICommand = "none", "", ""
			ui.Info(i18n.T("ai_none_saved"))
			return w.Save()
		case "custom":
			cmdline, err := ui.Input(i18n.T("ai_custom_command"), i18n.T("ai_custom_command_desc"), s.AICommand, func(v string) error {
				if strings.TrimSpace(v) == "" {
					return errors.New(i18n.T("ai_custom_empty"))
				}
				return nil
			})
			if err != nil {
				return err
			}
			cfg.Command = cmdline
		default:
			p, _ := ai.Find(cli)
			if !sys.Has(p.Command) {
				ui.Println(ui.WarnBox(i18n.T("ai_install_title", p.Name), i18n.T("ai_install_cmd", p.Install), i18n.T("ai_login_cmd", p.Login)))
				again, err := ui.Confirm(i18n.T("ai_installed_now"), "", false)
				if err != nil {
					return err
				}
				if !again || !sys.Has(p.Command) {
					def = cli
					continue
				}
			}
		}
		prevModel := ""
		if cli == s.AICLI {
			prevModel = s.AIModel
		}
		if cli == "custom" && strings.Contains(cfg.Command, "{model}") {
			model, err := ui.Input(i18n.T("ai_model"), "", prevModel, nil)
			if err != nil {
				return err
			}
			cfg.Model = model
		} else if cli != "custom" {
			p, _ := ai.Find(cli)
			model, err := chooseModel(p, prevModel)
			if err != nil {
				return err
			}
			cfg.Model = model
		}
		var testErr error
		_ = ui.Spinner(i18n.T("ai_testing", cfg.Label()), func() error { testErr = ai.Test(cfg); return nil })
		if testErr == nil {
			ui.Done(i18n.T("ai_test_ok", cfg.Label()))
			s.AICLI, s.AIModel, s.AICommand = cfg.CLI, cfg.Model, cfg.Command
			return w.Save()
		}
		var ae *ai.Error
		detail, hint := testErr.Error(), ""
		if errors.As(testErr, &ae) {
			detail, hint = ae.Detail, ae.Hint
		}
		ui.Println(ui.WarnBox(i18n.T("ai_test_failed", cfg.Label()), detail, hint))
		choice, err := ui.Select(i18n.T("ai_test_failed_what"), "", []ui.Option{
			{Value: "retry", Label: i18n.T("ai_choose_again")},
			{Value: "keep", Label: i18n.T("ai_keep_anyway"), Hint: i18n.T("ai_keep_anyway_hint")},
			{Value: "none", Label: i18n.T("ai_none_option")},
		}, "retry")
		if err != nil {
			return err
		}
		switch choice {
		case "keep":
			s.AICLI, s.AIModel, s.AICommand = cfg.CLI, cfg.Model, cfg.Command
			return w.Save()
		case "none":
			s.AICLI, s.AIModel, s.AICommand = "none", "", ""
			return w.Save()
		}
		def = cli
	}
}

func hasOption(opts []ui.Option, v string) bool {
	for _, o := range opts {
		if o.Value == v {
			return true
		}
	}
	return false
}

// modelOther is the select value that falls back to typing a model id.
const modelOther = "\x00other"

// chooseModel lists the models the CLI offers (the CLI's default first) so nobody has to type a
// slug. Typing stays available as the last option, and is the fallback when the list is unreadable.
func chooseModel(p ai.Preset, prev string) (string, error) {
	var models []string
	var listErr error
	_ = ui.Spinner(i18n.T("ai_models_loading", p.Name), func() error { models, listErr = ai.ListModels(p); return nil })
	if listErr != nil || len(models) == 0 {
		if listErr != nil {
			ui.Warning(i18n.T("ai_models_failed", p.Name, firstLine(listErr.Error())))
		}
		return ui.Input(i18n.T("ai_model"), i18n.T("ai_model_desc", p.Models), prev, nil)
	}
	opts := []ui.Option{{Value: "", Label: i18n.T("ai_default_model"), Hint: i18n.T("ai_model_default_hint")}}
	if prev != "" && !slices.Contains(models, prev) {
		opts = append(opts, ui.Option{Value: prev, Label: prev, Hint: i18n.T("ai_model_current")})
	}
	for _, m := range models {
		opts = append(opts, ui.Option{Value: m, Label: m})
	}
	opts = append(opts, ui.Option{Value: modelOther, Label: i18n.T("ai_model_other")})
	model, err := ui.Select(i18n.T("ai_model_choose", p.Name), i18n.T("ai_model_choose_desc", len(models)), opts, prev)
	if err != nil || model != modelOther {
		return model, err
	}
	return ui.Input(i18n.T("ai_model"), i18n.T("ai_model_desc", p.Models), prev, nil)
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i > 0 {
		return s[:i]
	}
	return s
}

// materializeFeature is the callback feature.New uses to write a feature workspace.
func materializeFeature(w *workspace.Workspace) error { return materialize.Write(w, self()) }
