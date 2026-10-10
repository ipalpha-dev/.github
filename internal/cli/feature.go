package cli

import (
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/ipalpha-dev/tooling/internal/feature"
	"github.com/ipalpha-dev/tooling/internal/i18n"
	"github.com/ipalpha-dev/tooling/internal/infra"
	"github.com/ipalpha-dev/tooling/internal/sys"
	"github.com/ipalpha-dev/tooling/internal/ui"
	"github.com/ipalpha-dev/tooling/internal/workspace"
)

func featureCmd() *cobra.Command {
	var yes, noWait, force bool
	var baseline string
	c := &cobra.Command{
		Use:   "feature",
		Short: i18n.T("cmd_feature"),
		Long:  i18n.T("cmd_feature_long"),
	}
	slugOf := func(w *workspace.Workspace, args []string) (string, error) {
		if len(args) > 0 {
			return args[0], nil
		}
		if w.FeatureSlug != "" {
			return w.FeatureSlug, nil
		}
		return "", ui.NewProblem(i18n.T("cmd_feature"), i18n.T("feature_slug_required"), "./feature list")
	}
	newCmd := &cobra.Command{
		Use:   "new <slug>",
		Short: i18n.T("cmd_feature_new"),
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			w, err := open("feature")
			if err != nil {
				return err
			}
			ui.Header("./feature new")
			froot, err := feature.New(w, args[0], baseline, self(), materializeFeature)
			if err != nil {
				return err
			}
			fw, _ := workspace.Open(froot)
			lines := []string{
				i18n.T("feature_created", froot), "",
				"  cd " + quotePath(relTo(w.Main(), froot)) + "   # " + i18n.T("feature_hint_cd"),
				"  " + runHint() + "   # " + i18n.T("feature_hint_run", fw.Settings.Port("oikos-webapp")),
				"  ./publish        # " + i18n.T("feature_hint_publish"), "",
			}
			for _, h := range feature.Hosts(args[0]) {
				lines = append(lines, "  https://"+h)
			}
			lines = append(lines, "  https://"+feature.DevelopersHost(args[0])+"  ("+i18n.T("feature_hint_developers")+")")
			for _, h := range feature.AppHosts(args[0]) {
				lines = append(lines, "  https://"+h+"  ("+i18n.T("feature_hint_app")+")")
			}
			ui.Println(ui.InfoBox(i18n.T("feature_created_title"), lines...))
			return nil
		},
	}
	newCmd.Flags().StringVar(&baseline, "baseline", "", i18n.T("flag_baseline"))
	listCmd := &cobra.Command{
		Use: "list", Aliases: []string{"ls"}, Short: i18n.T("cmd_feature_list"),
		RunE: func(cmd *cobra.Command, args []string) error {
			w, err := open("feature")
			if err != nil {
				return err
			}
			var entries []feature.Entry
			_ = ui.Spinner(i18n.T("feature_listing"), func() error { entries = feature.List(w); return nil })
			if len(entries) == 0 {
				ui.Info(i18n.T("feature_list_empty"))
				return nil
			}
			ui.Printf("%-28s %-4s %-12s %-28s %s\n", "feature", "gen", "state", "expires", "local")
			for _, e := range entries {
				local := "-"
				if e.Local {
					local = "yes"
				}
				ui.Printf("%-28s %-4d %-12s %-28s %s\n", e.Slug, e.Gen, e.State, e.Expires, local)
				for _, h := range e.Hosts {
					ui.Println("  " + ui.Link.Render("https://"+h))
				}
				if e.State == "live" && len(e.Hosts) > 0 {
					ui.Println("  " + ui.Link.Render("https://"+e.Hosts[0]+"/mailbox"))
				}
			}
			if sys.Has("kubectl") {
				out, err := sys.Output("", "kubectl", "get", "ns", "-l", "ipalpha.dev/preview=true",
					"-o", `custom-columns=NAMESPACE:.metadata.name,EXPIRES:.metadata.annotations.ipalpha\.dev/expires-at`)
				if err == nil && out != "" {
					ui.Println("\n" + out)
				}
			}
			return nil
		},
	}
	action := func(name string, confirm bool) *cobra.Command {
		cc := &cobra.Command{
			Use: name + " [slug]", Short: i18n.T("cmd_feature_" + name), Args: cobra.MaximumNArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				w, err := open("feature")
				if err != nil {
					return err
				}
				slug, err := slugOf(w, args)
				if err != nil {
					return err
				}
				if confirm && !yes && !ui.TypeToConfirm(i18n.T("feature_confirm"), slug) {
					return ui.ErrCancelled
				}
				return feature.Request(w, slug, name, !noWait)
			},
		}
		return cc
	}
	rebaseCmd := &cobra.Command{
		Use: "rebase [slug]", Short: i18n.T("cmd_feature_rebase"), Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			w, err := open("feature")
			if err != nil {
				return err
			}
			slug, err := slugOf(w, args)
			if err != nil {
				return err
			}
			return feature.Rebase(w, slug)
		},
	}
	destroyCmd := &cobra.Command{
		Use: "destroy [slug]", Short: i18n.T("cmd_feature_destroy"), Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			w, err := open("feature")
			if err != nil {
				return err
			}
			slug, err := slugOf(w, args)
			if err != nil {
				return err
			}
			return feature.Destroy(w, slug, yes, !noWait, force, func(fw *workspace.Workspace) error {
				return infra.New(fw).Stop(true, true)
			})
		},
	}
	destroyCmd.Flags().BoolVar(&force, "force", false, i18n.T("flag_force"))
	c.PersistentFlags().BoolVarP(&yes, "yes", "y", false, i18n.T("flag_yes"))
	c.PersistentFlags().BoolVar(&noWait, "no-wait", false, i18n.T("flag_no_wait"))
	c.AddCommand(newCmd, listCmd, rebaseCmd, action("extend", false), action("reset", true), destroyCmd)
	return c
}

func relTo(base, p string) string {
	if r, err := filepath.Rel(base, p); err == nil {
		return r
	}
	return p
}
