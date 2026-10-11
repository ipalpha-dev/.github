package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/ipalpha-dev/tooling/internal/ai"
	"github.com/ipalpha-dev/tooling/internal/catalog"
	"github.com/ipalpha-dev/tooling/internal/feature"
	"github.com/ipalpha-dev/tooling/internal/i18n"
	"github.com/ipalpha-dev/tooling/internal/publish"
	"github.com/ipalpha-dev/tooling/internal/repos"
	"github.com/ipalpha-dev/tooling/internal/sys"
	"github.com/ipalpha-dev/tooling/internal/ui"
	"github.com/ipalpha-dev/tooling/internal/workspace"
)

func publishCmd() *cobra.Command {
	var o struct {
		dry, ci, initialize, npmOnly, resume, noWait, yes bool
		folder, engine, model, featureSlug, message       string
		deploymentPaths                                   []string
	}
	c := &cobra.Command{
		Use:   "publish",
		Short: i18n.T("cmd_publish"),
		Long:  i18n.T("cmd_publish_long"),
		Args:  cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			w, err := open("publish")
			if err != nil {
				return err
			}
			if len(args) == 1 && args[0] == "clean" {
				if err := publish.ClearCache(w); err != nil {
					return err
				}
				ui.Done(i18n.T("publish_cache_clean"))
				return nil
			}
			if len(args) > 0 {
				return ui.NewProblem(i18n.T("cmd_publish"), i18n.T("publish_unknown_arg", strings.Join(args, " ")), "./publish --help")
			}
			cfg := publish.LoadAI(w, o.engine, o.model)
			slug := o.featureSlug
			if slug == "" {
				slug = w.FeatureSlug
			}
			if slug != "" {
				if o.folder != "" || o.initialize || o.npmOnly || o.resume || len(o.deploymentPaths) > 0 {
					return ui.NewProblem(i18n.T("cmd_publish"), i18n.T("publish_feature_flags"))
				}
				ui.Header(i18n.T("publish_feature_title", slug))
				return feature.Publish(w, slug, !o.noWait, o.dry, func(r catalog.Repo, dir string) (string, error) {
					p := publish.Plan{Repo: r, Dir: dir, Current: publish.CurrentVersion(dir)}
					if !ui.Interactive() {
						// Feature branches have no version: without a terminal a neutral message is enough.
						if cfg.Enabled() {
							if d, _, err := publish.Decide(w, r, dir, cfg); err == nil && d.Message != "" {
								return d.Message, nil
							} else if err != nil {
								ui.Warning(i18n.T("publish_ai_failed", cfg.Label()) + ": " + firstLine(err.Error()))
							}
						}
						return "Update " + r.Name, nil
					}
					if err := decide(w, &p, cfg); err != nil {
						return "", err
					}
					return p.Message, nil
				})
			}
			switch {
			case o.npmOnly:
				return npmOnly(w, o.folder, o.dry)
			case o.resume:
				return resume(w, o.folder, o.dry, o.ci)
			case o.initialize:
				return initialize(w, o.folder, o.dry)
			case len(o.deploymentPaths) > 0:
				return deploymentPaths(w, o.deploymentPaths, o.message, o.dry)
			}
			return release(w, publish.Options{DryRun: o.dry, Folder: o.folder, CI: o.ci, Yes: o.yes, AI: cfg})
		},
	}
	f := c.Flags()
	f.BoolVarP(&o.dry, "dry-run", "d", false, i18n.T("flag_dry_run"))
	f.StringVarP(&o.folder, "folder", "f", "", i18n.T("flag_folder"))
	f.StringVar(&o.engine, "engine", "", i18n.T("flag_engine"))
	f.StringVar(&o.model, "model", "", i18n.T("flag_model"))
	f.BoolVarP(&o.yes, "yes", "y", false, i18n.T("flag_yes"))
	f.BoolVar(&o.ci, "ci", false, i18n.T("flag_ci"))
	f.BoolVar(&o.initialize, "initialize", false, i18n.T("flag_initialize"))
	f.BoolVar(&o.npmOnly, "npm-only", false, i18n.T("flag_npm_only"))
	f.BoolVar(&o.resume, "resume", false, i18n.T("flag_resume"))
	f.StringArrayVar(&o.deploymentPaths, "deployment-path", nil, i18n.T("flag_deployment_path"))
	f.StringVar(&o.message, "message", "Update deployment configuration", i18n.T("flag_message"))
	f.StringVar(&o.featureSlug, "feature", "", i18n.T("flag_feature"))
	f.BoolVar(&o.noWait, "no-wait", false, i18n.T("flag_no_wait"))
	return c
}

// decide fills bump/message: AI first; on failure show the engine's error and ask by hand.
func decide(w *workspace.Workspace, p *publish.Plan, cfg ai.Config) error {
	files := changedFilesOf(p.Dir)
	if cfg.Enabled() {
		var d ai.Decision
		var src string
		var err error
		label := i18n.T("publish_analyzing", p.Repo.Name, cfg.Label())
		start := time.Now()
		_ = ui.Spinner(label, func() error { d, src, err = publish.Decide(w, p.Repo, p.Dir, cfg); return nil })
		if err == nil {
			p.Bump, p.Message, p.Reason, p.Source = d.Bump, d.Message, d.Reason, src
			ui.Raw("%s decided in %s", p.Repo.Name, time.Since(start))
			p.Bump = publish.Clamp(files, p.Bump)
			p.Next = publish.Bump(p.Current, p.Bump)
			return nil
		}
		var ae *ai.Error
		detail, hint := err.Error(), ""
		if errors.As(err, &ae) {
			detail, hint = ae.Detail, ae.Hint
		}
		ui.Println(ui.WarnBox(i18n.T("publish_ai_failed", cfg.Label()), detail, hint, i18n.T("publish_ai_change", "./ipalpha ai")))
	}
	if !ui.Interactive() {
		return ui.NewProblem(i18n.T("publish_decide_step", p.Repo.Name), i18n.T("publish_ai_unavailable_ci"), "./publish --engine <cli>", "ipalpha ai")
	}
	d, err := publish.Manual(p.Repo, p.Current, files)
	if err != nil {
		return err
	}
	p.Bump, p.Message, p.Reason, p.Source = d.Bump, d.Message, d.Reason, "manual"
	p.Bump = publish.Clamp(files, p.Bump)
	p.Next = publish.Bump(p.Current, p.Bump)
	return nil
}

func changedFilesOf(dir string) []string {
	out, _ := sys.Git(dir, "status", "--porcelain")
	var files []string
	for _, l := range strings.Split(out, "\n") {
		if len(l) > 3 {
			files = append(files, strings.Trim(l[3:], `"`))
		}
	}
	return files
}

func release(w *workspace.Workspace, o publish.Options) error {
	ui.Header(i18n.T("publish_title"))
	if o.DryRun {
		ui.Println(ui.Warn.Render(i18n.T("publish_dry_banner")))
	}
	ui.Info(i18n.T("publish_ai_line", o.AI.Label()))
	var dirty []catalog.Repo
	if o.Folder != "" {
		r, ok := catalog.Get(o.Folder)
		if !ok || !r.Publishable() {
			return ui.NewProblem(i18n.T("publish_title"), i18n.T("publish_folder_unknown", o.Folder), i18n.T("publish_folder_list", strings.Join(publishable(), ", ")))
		}
		if !publish.Dirty(r.Path(w.Root)) {
			ui.Done(i18n.T("publish_no_dirty_one", o.Folder))
			return nil
		}
		dirty = []catalog.Repo{r}
	} else {
		dirty = publish.DirtyRepos(w)
	}
	if len(dirty) == 0 {
		ui.Done(i18n.T("publish_no_dirty"))
		return nil
	}
	dirs := []string{}
	for _, r := range dirty {
		dirs = append(dirs, r.Path(w.Root))
	}
	dirs = append(dirs, w.Repo("deployment"))
	if err := publish.Preflight(dirs, !o.DryRun); err != nil {
		return err
	}
	selected := dirty
	if o.Folder == "" && len(dirty) > 1 {
		var opts []ui.Option
		var def []string
		for _, r := range dirty {
			opts = append(opts, ui.Option{Value: r.Name, Label: r.Name, Hint: fmt.Sprintf("%d %s", len(changedFilesOf(r.Path(w.Root))), i18n.T("publish_files"))})
			def = append(def, r.Name)
		}
		names, err := ui.MultiSelect(i18n.T("publish_select"), i18n.T("publish_select_desc"), opts, def)
		if err != nil {
			return err
		}
		pick := map[string]bool{}
		for _, n := range names {
			pick[n] = true
		}
		selected = nil
		for _, r := range dirty {
			if pick[r.Name] {
				selected = append(selected, r)
			}
		}
		if len(selected) == 0 {
			ui.Info(i18n.T("publish_nothing_selected"))
			return nil
		}
	}
	var plans []publish.Plan
	for i, r := range selected {
		dir := r.Path(w.Root)
		p := publish.Plan{Repo: r, Dir: dir, Current: publish.CurrentVersion(dir), Changed: len(changedFilesOf(dir))}
		ui.Section(fmt.Sprintf("[%d/%d] %s", i+1, len(selected), r.Name))
		if err := decide(w, &p, o.AI); err != nil {
			return err
		}
		printPlan(p)
		plans = append(plans, p)
	}
	if o.DryRun {
		ui.Println("")
		ui.Println(ui.Warn.Render(i18n.T("publish_dry_done")))
		if !ui.Interactive() {
			return nil
		}
		ok, err := ui.Confirm(i18n.T("publish_apply_now"), i18n.T("publish_apply_now_desc"), false)
		if err != nil || !ok {
			return err
		}
		if err := publish.Preflight(dirs, true); err != nil {
			return err
		}
	} else if !o.Yes && ui.Interactive() {
		ok, err := ui.Confirm(i18n.T("publish_confirm", len(plans)), i18n.T("publish_confirm_desc"), true)
		if err != nil {
			return err
		}
		if !ok {
			return ui.ErrCancelled
		}
	}
	for _, p := range plans {
		ui.Section(i18n.T("publish_applying", p.Repo.Name))
		if err := publish.Apply(w, p, o.CI); err != nil {
			return err
		}
		ui.Done(fmt.Sprintf("%s %s", p.Repo.Name, versionText(p)))
	}
	ui.Done(i18n.T("publish_done"))
	return nil
}

func versionText(p publish.Plan) string {
	if p.Bump == "none" {
		return p.Current + " (" + i18n.T("publish_no_bump") + ")"
	}
	return p.Current + " → " + p.Next
}

func printPlan(p publish.Plan) {
	src := map[string]string{"ai": "AI", "cache": "AI (cache)", "manual": i18n.T("publish_manual")}[p.Source]
	ui.Printf("  %s %s\n", ui.Bold.Render(i18n.T("publish_reason")+":"), p.Reason)
	ui.Printf("  %s %s   %s\n", ui.Bold.Render(i18n.T("publish_bump")+":"), p.Bump, ui.Muted.Render(versionText(p)))
	ui.Printf("  %s %s   %s\n", ui.Bold.Render(i18n.T("publish_message")+":"), p.Message, ui.Muted.Render(src))
}

func publishable() []string {
	var out []string
	for _, r := range catalog.Repos {
		if r.Publishable() {
			out = append(out, r.Name)
		}
	}
	return out
}

func npmOnly(w *workspace.Workspace, folder string, dry bool) error {
	r, ok := catalog.Get(folder)
	if !ok || r.Kind != catalog.KindLibrary {
		return ui.NewProblem(i18n.T("cmd_publish"), i18n.T("publish_npm_only_folder"), "./publish --npm-only -f shared-js")
	}
	dir := w.Repo(folder)
	v := publish.CurrentVersion(dir)
	if st, _ := sys.Git(dir, "status", "--porcelain"); st != "" {
		return ui.NewProblem(i18n.T("cmd_publish"), i18n.T("publish_must_be_clean", folder))
	}
	tag, _ := sys.Git(dir, "rev-parse", "v"+v+"^{commit}")
	head, _ := sys.Git(dir, "rev-parse", "HEAD")
	if tag == "" || tag != head {
		return ui.NewProblem(i18n.T("cmd_publish"), i18n.T("publish_head_tag", folder, v))
	}
	ui.Info(i18n.T("publish_npm_only_plan", folder, v))
	if dry {
		return nil
	}
	return publish.NpmPublish(folder, dir)
}

func resume(w *workspace.Workspace, folder string, dry, ci bool) error {
	r, ok := catalog.Get(folder)
	if !ok || !r.HasImage() || r.Kind == catalog.KindOptional {
		return ui.NewProblem(i18n.T("cmd_publish"), i18n.T("publish_resume_folder"))
	}
	dir := r.Path(w.Root)
	if st, _ := sys.Git(dir, "status", "--porcelain"); st != "" {
		return ui.NewProblem(i18n.T("cmd_publish"), i18n.T("publish_must_be_clean", folder))
	}
	v := publish.CurrentVersion(dir)
	ui.Info(i18n.T("publish_resume_plan", folder, v))
	if dry {
		return nil
	}
	if res := (sys.Cmd{Name: "git", Args: []string{"-C", dir, "push", "--set-upstream", "origin", "HEAD"}, Log: ui.LogWriter()}).Run(); res.Err != nil {
		return &ui.Problem{Step: "git push (" + folder + ")", Tail: ui.Tail(res.Output, 6)}
	}
	if !ci {
		if err := publish.BuildImage(w, r, v, dir); err != nil {
			return err
		}
	}
	return publish.BumpDeployment(w, folder, v)
}

func initialize(w *workspace.Workspace, folder string, dry bool) error {
	r, ok := catalog.Get(folder)
	if folder == "" || !ok || !r.Publishable() {
		return ui.NewProblem(i18n.T("cmd_publish"), i18n.T("publish_initialize_folder"))
	}
	dir := r.Path(w.Root)
	for _, f := range []string{"package.json", ".gitignore"} {
		if _, err := os.Stat(filepath.Join(dir, f)); err != nil {
			return ui.NewProblem(i18n.T("cmd_publish"), i18n.T("publish_initialize_needs", folder))
		}
	}
	if repos.IsGit(dir) {
		ui.Done(i18n.T("publish_initialize_already", folder))
		return nil
	}
	org := w.Settings.Org
	if dry {
		ui.Info(i18n.T("publish_initialize_plan", folder, org))
		return nil
	}
	if res := (sys.Cmd{Name: "git", Args: []string{"init", "-q", "-b", "master", dir}}).Run(); res.Err != nil {
		return &ui.Problem{Step: "git init", Tail: ui.Tail(res.Output, 4)}
	}
	if _, err := sys.Output("", "gh", "repo", "view", org+"/"+folder, "--json", "name"); err == nil {
		_, err = sys.Git(dir, "remote", "add", "origin", repos.URL(org, folder))
		return err
	}
	if res := (sys.Cmd{Name: "gh", Args: []string{"repo", "create", org + "/" + folder, "--private", "--source", dir, "--remote", "origin"}}).Run(); res.Err != nil {
		return &ui.Problem{Step: "gh repo create", Tail: ui.Tail(res.Output, 4), Fix: []string{"gh auth login"}}
	}
	_, err := sys.Git(dir, "remote", "set-url", "origin", repos.URL(org, folder))
	return err
}

func deploymentPaths(w *workspace.Workspace, paths []string, message string, dry bool) error {
	dep := w.Repo("deployment")
	if staged, _ := sys.Git(dep, "diff", "--cached", "--name-only"); staged != "" {
		return ui.NewProblem(i18n.T("cmd_publish"), i18n.T("publish_deployment_index"))
	}
	for _, p := range paths {
		clean := filepath.ToSlash(filepath.Clean(p))
		low := strings.ToLower(clean)
		if filepath.IsAbs(p) || strings.HasPrefix(clean, "../") || clean == ".." || strings.Contains(low, "secret") {
			return ui.NewProblem(i18n.T("cmd_publish"), i18n.T("publish_unsafe_path", p))
		}
		if _, err := os.Stat(filepath.Join(dep, p)); err != nil {
			return ui.NewProblem(i18n.T("cmd_publish"), i18n.T("publish_missing_path", p))
		}
	}
	if dry {
		ui.Info(i18n.T("publish_deployment_plan", strings.Join(paths, " ")))
		out, _ := sys.Git(dep, append([]string{"diff", "--stat", "--"}, paths...)...)
		ui.Println(out)
		return nil
	}
	if _, err := sys.Git(dep, append([]string{"add", "--"}, paths...)...); err != nil {
		return err
	}
	if sys.GitOK(dep, "diff", "--cached", "--quiet") {
		return nil
	}
	if res := (sys.Cmd{Name: "git", Args: []string{"-C", dep, "commit", "-q", "-m", message}}).Run(); res.Err != nil {
		return &ui.Problem{Step: "git commit", Tail: ui.Tail(res.Output, 4)}
	}
	if res := (sys.Cmd{Name: "git", Args: []string{"-C", dep, "push"}}).Run(); res.Err != nil {
		return &ui.Problem{Step: "git push", Tail: ui.Tail(res.Output, 4)}
	}
	ui.Done(i18n.T("publish_done"))
	return nil
}
