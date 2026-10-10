package cli

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/ipalpha-dev/tooling/internal/ai"
	"github.com/ipalpha-dev/tooling/internal/catalog"
	"github.com/ipalpha-dev/tooling/internal/env"
	"github.com/ipalpha-dev/tooling/internal/envfile"
	"github.com/ipalpha-dev/tooling/internal/i18n"
	"github.com/ipalpha-dev/tooling/internal/infra"
	"github.com/ipalpha-dev/tooling/internal/ui"
	"github.com/ipalpha-dev/tooling/internal/workspace"
)

func aiCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "ai",
		Short: i18n.T("cmd_ai"),
		Long:  i18n.T("cmd_ai_long"),
		RunE: func(cmd *cobra.Command, args []string) error {
			w, err := open("ai")
			if err != nil {
				return err
			}
			ui.Header("ai")
			ui.Info(i18n.T("ai_current", publishCfg(w).Label()))
			return chooseAI(w, false, false)
		},
	}
	c.AddCommand(&cobra.Command{
		Use: "test", Short: i18n.T("cmd_ai_test"),
		RunE: func(cmd *cobra.Command, args []string) error {
			w, err := open("ai")
			if err != nil {
				return err
			}
			cfg := publishCfg(w)
			var testErr error
			_ = ui.Spinner(i18n.T("ai_testing", cfg.Label()), func() error { testErr = ai.Test(cfg); return nil })
			if testErr != nil {
				var ae *ai.Error
				p := &ui.Problem{Step: i18n.T("ai_test_failed", cfg.Label()), Cause: testErr.Error(), Fix: []string{"ipalpha ai"}}
				if errors.As(testErr, &ae) {
					p.Cause = ae.Detail
					p.Fix = append([]string{ae.Hint}, p.Fix...)
				}
				return p
			}
			ui.Done(i18n.T("ai_test_ok", cfg.Label()))
			return nil
		},
	})
	set := &cobra.Command{
		Use: "set <cli> [model]", Short: i18n.T("cmd_ai_set"), Args: cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			w, err := open("ai")
			if err != nil {
				return err
			}
			cli := args[0]
			if _, ok := ai.Find(cli); !ok && cli != "custom" && cli != "none" {
				var ids []string
				for _, p := range ai.Presets {
					ids = append(ids, p.ID)
				}
				return ui.NewProblem(i18n.T("cmd_ai_set"), i18n.T("ai_unknown", cli), i18n.T("ai_known", strings.Join(ids, ", ")))
			}
			w.Settings.AICLI = cli
			w.Settings.AIModel = ""
			if len(args) == 2 {
				if cli == "custom" {
					w.Settings.AICommand = args[1]
				} else {
					w.Settings.AIModel = args[1]
				}
			}
			if err := w.Save(); err != nil {
				return err
			}
			ui.Done(i18n.T("ai_selected", publishCfg(w).Label()))
			return nil
		},
	}
	c.AddCommand(set)
	return c
}

func publishCfg(w *workspace.Workspace) ai.Config {
	return ai.Config{CLI: w.Settings.AICLI, Model: w.Settings.AIModel, Command: w.Settings.AICommand}
}

func configCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "config",
		Short: i18n.T("cmd_config"),
		Long:  i18n.T("cmd_config_long"),
		RunE: func(cmd *cobra.Command, args []string) error {
			w, err := open("config")
			if err != nil {
				return err
			}
			ui.Header("config")
			for {
				s := w.Settings
				lang := s.Lang
				if lang == "" {
					lang = i18n.T("config_lang_auto", i18n.LangName(i18n.Detect()))
				} else {
					lang = i18n.LangName(lang)
				}
				rt := s.Runtime
				if rt == "" {
					rt = i18n.T("config_auto")
				}
				choice, err := ui.Select(i18n.T("config_what"), w.SettingsPath(), []ui.Option{
					{Value: "lang", Label: i18n.T("config_lang"), Hint: lang},
					{Value: "ai", Label: i18n.T("config_ai"), Hint: publishCfg(w).Label()},
					{Value: "apps", Label: i18n.T("config_apps"), Hint: strings.Join(s.Browsers(), ", ")},
					{Value: "ports", Label: i18n.T("config_ports"), Hint: i18n.T("config_ports_hint")},
					{Value: "runtime", Label: i18n.T("config_runtime"), Hint: rt},
					{Value: "account", Label: i18n.T("config_account"), Hint: accountHint(w)},
					{Value: "done", Label: i18n.T("config_done")},
				}, "done")
				if err != nil || choice == "done" {
					return err
				}
				switch choice {
				case "lang":
					opts := []ui.Option{{Value: "", Label: i18n.T("config_lang_auto", i18n.LangName(i18n.Detect()))}}
					for _, l := range i18n.Langs {
						opts = append(opts, ui.Option{Value: l, Label: i18n.LangName(l)})
					}
					v, err := ui.Select(i18n.T("config_lang"), "", opts, s.Lang)
					if err != nil {
						return err
					}
					s.Lang = v
					if v == "" {
						i18n.Set(i18n.Detect())
					} else {
						i18n.Set(v)
					}
				case "ai":
					if err := chooseAI(w, false, false); err != nil {
						return err
					}
				case "apps":
					if err := appsCmd().RunE(cmd, nil); err != nil {
						return err
					}
					w, _ = workspace.Open(w.Root)
					continue
				case "ports":
					if err := editPorts(w); err != nil {
						return err
					}
				case "runtime":
					v, err := ui.Select(i18n.T("config_runtime"), i18n.T("config_runtime_desc"), []ui.Option{
						{Value: "", Label: i18n.T("config_auto"), Hint: infra.Detect("")},
						{Value: infra.Docker, Label: "Docker"},
						{Value: infra.Apple, Label: "Apple container", Hint: "macOS"},
					}, s.Runtime)
					if err != nil {
						return err
					}
					s.Runtime = v
				case "account":
					if err := askSuperuser(w, false); err != nil {
						return err
					}
				}
				if err := w.Save(); err != nil {
					return err
				}
				ui.Done(i18n.T("config_saved"))
			}
		},
	}
	c.AddCommand(&cobra.Command{
		Use: "get [key]", Short: i18n.T("cmd_config_get"), Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			w, err := open("config")
			if err != nil {
				return err
			}
			values := envfile.Parse(w.Settings.Render())
			if len(args) == 1 {
				fmt.Println(values[args[0]])
				return nil
			}
			for _, k := range catalog.SortedKeys(values) {
				fmt.Printf("%s=%s\n", k, values[k])
			}
			return nil
		},
	})
	c.AddCommand(&cobra.Command{
		Use: "set <key> <value>", Short: i18n.T("cmd_config_set"), Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			w, err := open("config")
			if err != nil {
				return err
			}
			if err := setSetting(w.Settings, args[0], args[1]); err != nil {
				return err
			}
			if err := w.Save(); err != nil {
				return err
			}
			ui.Done(i18n.T("config_saved"))
			return nil
		},
	})
	return c
}

func accountHint(w *workspace.Workspace) string {
	n, p := env.Superuser(w)
	if n == "" && p == "" {
		return "-"
	}
	return n + " " + p
}

func setSetting(s *workspace.Settings, key, value string) error {
	switch key {
	case "lang":
		if value != "" {
			value = i18n.Normalize(value)
		}
		s.Lang = value
	case "runtime":
		if value != "" && value != infra.Docker && value != infra.Apple {
			return ui.NewProblem("config", i18n.T("config_bad_value", key, value), "docker | container")
		}
		s.Runtime = value
	case "ai_cli":
		s.AICLI = value
	case "ai_model":
		s.AIModel = value
	case "ai_command":
		s.AICommand = value
	case "browser_apps":
		s.SetBrowsers(strings.Fields(value))
	case "registry":
		s.Registry = value
	default:
		if strings.HasSuffix(key, "_port") {
			p, err := strconv.Atoi(value)
			if err != nil || p < 1024 || p > 65535 {
				return ui.NewProblem("config", i18n.T("config_bad_port", value))
			}
			k := strings.TrimSuffix(key, "_port")
			if k == "rabbitmq_mgmt" {
				k = "rabbitmq-mgmt"
			}
			if _, ok := catalog.DefaultPorts()[k]; !ok {
				return ui.NewProblem("config", i18n.T("config_unknown_key", key))
			}
			s.Ports[k] = p
			return nil
		}
		return ui.NewProblem("config", i18n.T("config_unknown_key", key), "ipalpha config get")
	}
	return nil
}

func editPorts(w *workspace.Workspace) error {
	var opts []ui.Option
	for _, k := range catalog.PortKeys() {
		opts = append(opts, ui.Option{Value: k, Label: k, Hint: strconv.Itoa(w.Settings.Port(k))})
	}
	opts = append(opts, ui.Option{Value: "reset", Label: i18n.T("config_ports_reset")})
	k, err := ui.Select(i18n.T("config_ports"), i18n.T("config_ports_desc"), opts, "")
	if err != nil {
		return err
	}
	if k == "reset" {
		w.Settings.Ports = map[string]int{}
		return nil
	}
	v, err := ui.Input(k, i18n.T("config_port_desc"), strconv.Itoa(w.Settings.Port(k)), func(s string) error {
		p, err := strconv.Atoi(strings.TrimSpace(s))
		if err != nil || p < 1024 || p > 65535 {
			return errors.New(i18n.T("config_bad_port", s))
		}
		return nil
	})
	if err != nil {
		return err
	}
	p, _ := strconv.Atoi(v)
	w.Settings.Ports[k] = p
	return nil
}
