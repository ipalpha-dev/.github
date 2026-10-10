package cli

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/ipalpha-dev/tooling/internal/catalog"
	"github.com/ipalpha-dev/tooling/internal/env"
	"github.com/ipalpha-dev/tooling/internal/i18n"
	"github.com/ipalpha-dev/tooling/internal/materialize"
	"github.com/ipalpha-dev/tooling/internal/repos"
	"github.com/ipalpha-dev/tooling/internal/sys"
	"github.com/ipalpha-dev/tooling/internal/ui"
	"github.com/ipalpha-dev/tooling/internal/workspace"
)

func pullCmd() *cobra.Command {
	var noUpdate bool
	c := &cobra.Command{
		Use:   "pull",
		Short: i18n.T("cmd_pull"),
		Long:  i18n.T("cmd_pull_long"),
		RunE: func(cmd *cobra.Command, args []string) error {
			w, err := open("pull")
			if err != nil {
				return err
			}
			ui.Header("./pull")
			if !noUpdate && os.Getenv("IPALPHA_NO_SELF_UPDATE") != "1" {
				if updated, err := selfUpdate(w); err != nil {
					ui.Warning(i18n.T("update_failed", firstLine(err.Error())))
				} else if updated != "" {
					ui.Done(i18n.T("update_done", updated))
					// Continue with the new binary (new catalog, new env templates).
					return reexec(materialize.BinPath(w), append([]string{"pull", "--no-update"}, args...))
				}
			}
			return pull(w)
		},
	}
	c.Flags().BoolVar(&noUpdate, "no-update", false, i18n.T("flag_no_update"))
	return c
}

func pull(w *workspace.Workspace) error {
	migrateLayout(w)
	ui.Section(i18n.T("pull_repos"))
	access := repos.Access{SSH: true}
	if os.Getenv("IPALPHA_CLONE_COMMAND") == "" && os.Getenv("IPALPHA_CLONE_FROM") == "" {
		access = repos.Probe()
	}
	var problems []string
	repos.PullAll(w.Settings.Org, w.Root, w.IsFeature(), access, func(o repos.Outcome) {
		label := map[string]string{
			"pulled": ui.OK.Render(i18n.T("repos_pulled")), "current": ui.Muted.Render(i18n.T("repos_current")),
			"cloned": ui.OK.Render(i18n.T("repos_cloned")), "fetched": ui.Muted.Render(i18n.T("repos_fetched")),
			"conflict": ui.Warn.Render(i18n.T("repos_conflict")), "failed": ui.Bad.Render(i18n.T("repos_failed")),
			"skipped": ui.Muted.Render(i18n.T("repos_skipped")),
		}[o.Status]
		if o.Detail != "" {
			label += ui.Muted.Render("  " + o.Detail)
		}
		ui.Item(o.Repo.Rel(), label)
		if o.Status == "failed" || o.Status == "conflict" {
			problems = append(problems, o.Repo.Name+": "+o.Detail)
		}
	})
	ui.Section(i18n.T("pull_env"))
	changes, err := env.Prepare(w)
	if err != nil {
		return ui.Wrap(i18n.T("pull_env"), err)
	}
	for _, ch := range changes {
		switch {
		case ch.Created:
			ui.Item(ch.Repo, i18n.T("env_created"))
		case len(ch.Added) > 0:
			ui.Item(ch.Repo, i18n.T("env_added", strings.Join(ch.Added, ", ")))
		}
		if ch.Fallback {
			ui.Item(ch.Repo, ui.Muted.Render(i18n.T("env_fallback")))
		}
	}
	if err := materialize.Write(w, self()); err != nil {
		return ui.Wrap(i18n.T("setup_files"), err)
	}
	ui.Done(i18n.T("pull_done"))
	if len(problems) > 0 {
		ui.Println(ui.WarnBox(i18n.T("pull_attention"), append(problems, "", i18n.T("pull_conflict_hint"))...))
	}
	return nil
}

// migrateLayout moves repositories that left core/ (forms → apps/forms/), with their feature
// worktrees, keeping branches, .env files and uncommitted work.
func migrateLayout(w *workspace.Workspace) {
	root, _ := filepath.EvalSymlinks(w.Root)
	if root == "" {
		root = w.Root
	}
	for _, r := range catalog.Repos {
		if r.App == "" {
			continue
		}
		old := filepath.Join(root, "core", r.Name)
		dest := r.Path(root)
		if _, err := os.Stat(old); err != nil {
			continue
		}
		if _, err := os.Stat(dest); err == nil {
			continue
		}
		list, _ := sys.Git(old, "worktree", "list", "--porcelain")
		for _, l := range strings.Split(list, "\n") {
			wt := strings.TrimPrefix(l, "worktree ")
			if wt == l {
				continue
			}
			if nw, ok := movedWorktree(root, wt, "core/"+r.Name, r.Rel()); ok {
				_ = os.MkdirAll(filepath.Dir(nw), 0o755)
				_, _ = sys.Git(old, "worktree", "move", wt, nw)
			}
		}
		_ = os.MkdirAll(filepath.Dir(dest), 0o755)
		if os.Rename(old, dest) == nil {
			_, _ = sys.Git(dest, "worktree", "repair")
			ui.Item(r.Name, "core/"+r.Name+" → "+r.Rel())
		}
	}
}

// movedWorktree returns where a feature worktree of a moved repository goes. Git reports worktree
// paths with forward slashes (C:/Users/... on Windows) and resolved symlinks (/private/var on
// macOS), so both sides are compared in one canonical form (slashes, and case-insensitive on Windows).
func movedWorktree(root, wt, relOld, relNew string) (string, bool) {
	canon := func(p string) string {
		if r, err := filepath.EvalSymlinks(p); err == nil {
			p = r
		}
		p = strings.TrimSuffix(filepath.ToSlash(p), "/")
		if sys.Windows {
			p = strings.ToLower(p)
		}
		return p
	}
	w, base := canon(wt), canon(root)+"/features/"
	suffix := "/" + relOld
	if sys.Windows {
		suffix = strings.ToLower(suffix)
	}
	if !strings.HasPrefix(w, base) || !strings.HasSuffix(w, suffix) {
		return "", false
	}
	// features/<slug>/core/<repo>: exactly one folder between features/ and the repo.
	slug := strings.TrimSuffix(strings.TrimPrefix(w, base), suffix)
	if slug == "" || strings.Contains(slug, "/") {
		return "", false
	}
	cut := filepath.ToSlash(wt)
	cut = strings.TrimSuffix(cut, "/")
	cut = cut[:len(cut)-len(suffix)]
	return filepath.FromSlash(cut + "/" + relNew), true
}

// Release download location.
func releaseBase() string {
	if b := os.Getenv("IPALPHA_RELEASE_URL"); b != "" {
		return strings.TrimRight(b, "/")
	}
	return "https://github.com/" + catalog.Org + "/" + catalog.ToolingRepo + "/releases/latest/download"
}

// AssetName of this platform's binary.
func AssetName() string {
	name := fmt.Sprintf("ipalpha-%s-%s", runtime.GOOS, runtime.GOARCH)
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	return name
}

// selfUpdate downloads the latest release when it differs from the running binary (checksum).
// Returns the new version label, or "" when already current.
func selfUpdate(w *workspace.Workspace) (string, error) {
	client := &http.Client{Timeout: 120 * time.Second}
	sums, err := fetch(client, releaseBase()+"/checksums.txt")
	if err != nil {
		return "", err
	}
	want := ""
	for _, l := range strings.Split(string(sums), "\n") {
		f := strings.Fields(l)
		if len(f) == 2 && strings.TrimPrefix(f[1], "*") == AssetName() {
			want = f[0]
		}
	}
	if want == "" {
		return "", fmt.Errorf("no %s in checksums.txt", AssetName())
	}
	dest := materialize.BinPath(w)
	if cur, err := fileSHA(dest); err == nil && cur == want {
		return "", nil
	}
	if cur, err := fileSHA(self()); err == nil && cur == want {
		return "", nil
	}
	data, err := fetch(client, releaseBase()+"/"+AssetName())
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	if hex.EncodeToString(sum[:]) != want {
		return "", fmt.Errorf("checksum mismatch for %s", AssetName())
	}
	tmp := dest + ".download"
	if err := os.WriteFile(tmp, data, 0o755); err != nil {
		return "", err
	}
	defer os.Remove(tmp)
	if err := materialize.InstallBinary(tmp, dest); err != nil {
		return "", err
	}
	version := "latest"
	if v, err := fetch(client, releaseBase()+"/version.json"); err == nil {
		var doc struct {
			Version string `json:"version"`
		}
		if json.Unmarshal(v, &doc) == nil && doc.Version != "" {
			version = doc.Version
		}
	}
	return version, nil
}

func fetch(c *http.Client, url string) ([]byte, error) {
	resp, err := c.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("GET %s: %s", url, resp.Status)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 200<<20))
}

func fileSHA(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func updateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "update",
		Short: i18n.T("cmd_update"),
		RunE: func(cmd *cobra.Command, args []string) error {
			w, err := open("update")
			if err != nil {
				return err
			}
			v, err := selfUpdate(w)
			if err != nil {
				return ui.Wrap(i18n.T("cmd_update"), err, i18n.T("update_fix_manual"))
			}
			if v == "" {
				ui.Done(i18n.T("update_current", Version))
				return nil
			}
			ui.Done(i18n.T("update_done", v))
			return reexec(materialize.BinPath(w), []string{"pull", "--no-update"})
		},
	}
}
