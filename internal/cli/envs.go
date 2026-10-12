package cli

import (
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/ipalpha-dev/tooling/internal/envs"
	"github.com/ipalpha-dev/tooling/internal/i18n"
	"github.com/ipalpha-dev/tooling/internal/ui"
	"github.com/ipalpha-dev/tooling/internal/workspace"
)

func envsCmd() *cobra.Command {
	var list bool
	c := &cobra.Command{
		Use:   "envs [name]",
		Short: i18n.T("cmd_envs"),
		Long:  i18n.T("cmd_envs_long"),
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			w, err := open("envs")
			if err != nil {
				return err
			}
			t := envs.NewTmux()
			return runEnvs(w, t, envs.DefaultDeps(t), list, args)
		},
	}
	c.Flags().BoolVar(&list, "list", false, i18n.T("flag_envs_list"))
	return c
}

func runEnvs(w *workspace.Workspace, t *envs.Tmux, deps envs.Deps, list bool, args []string) error {
	gather := func() []envs.Row { return envs.Gather(w.Main(), w.Root, deps) }
	rows := gather()
	if len(args) == 1 {
		name := args[0]
		if name == "" || name == "master" {
			name = envs.MainName
		}
		row, ok := envs.Find(rows, name)
		if !ok {
			var names []string
			for _, r := range rows {
				names = append(names, r.Name)
			}
			return ui.NewProblem("envs", i18n.T("envs_unknown", args[0]), i18n.T("envs_known", strings.Join(names, ", ")))
		}
		return t.Do(envs.ActHop, row.Name, row.Root)
	}
	if list || !ui.Interactive() {
		ui.Printf("%s", envs.PlainTable(rows))
		return nil
	}
	choice, err := envs.Pick(rows, gather)
	if err != nil || choice == nil {
		return err
	}
	return t.Do(choice.Kind, choice.Row.Name, choice.Row.Root)
}

// envName is the tmux name of a workspace: its slug, or main.
func envName(w *workspace.Workspace) string {
	if w.IsFeature() {
		return w.FeatureSlug
	}
	return envs.MainName
}

// startRunInTmux is ./run --tmux (or settings tmux=on): outside tmux, ./run restarts itself in the
// `run` window of this workspace's session and this terminal attaches there. It reports false when
// ./run should simply run here (already inside tmux, or tmux missing).
func startRunInTmux(w *workspace.Workspace, asked bool) (bool, error) {
	t := envs.NewTmux()
	if t.Inside {
		return false, nil
	}
	if !t.Installed {
		if asked {
			ui.Warning(i18n.T("run_tmux_missing"))
		}
		return false, nil
	}
	var rest []string
	for _, a := range os.Args[2:] {
		if a != "--tmux" && a != "--tmux=true" {
			rest = append(rest, a)
		}
	}
	return t.StartRun(envName(w), w.Root, rest)
}
