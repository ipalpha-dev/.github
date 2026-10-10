// Package cli wires every command. Each command opens its own log under .ipalpha/logs and reports
// failures as an error box with the cause, the last lines and how to fix it.
package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"

	"github.com/spf13/cobra"

	"github.com/ipalpha-dev/tooling/internal/i18n"
	"github.com/ipalpha-dev/tooling/internal/sys"
	"github.com/ipalpha-dev/tooling/internal/ui"
	"github.com/ipalpha-dev/tooling/internal/workspace"
)

// Version is set at build time (-ldflags "-X …/cli.Version=v1.2.3").
var Version = "dev"

// Main runs the CLI and returns the exit code.
func Main(args []string) (code int) {
	sys.AddToPath()
	i18n.Set(i18n.Detect())
	if root, err := workspace.Find("."); err == nil {
		if s, err := workspace.LoadSettings(filepath.Join(root, workspace.DirName, "settings")); err == nil && s.Lang != "" && os.Getenv("IPALPHA_LANG") == "" {
			i18n.Set(s.Lang)
		}
	}
	defer func() {
		if r := recover(); r != nil {
			ui.Report(&ui.Problem{Step: i18n.T("ui_unexpected"), Cause: fmt.Sprint(r), Tail: ui.Tail(string(debug.Stack()), 12),
				Fix: []string{i18n.T("ui_report_bug")}})
			code = 70
		}
		ui.CloseLog()
	}()
	cmd := newRoot()
	cmd.SetArgs(args)
	err := cmd.Execute()
	if err == nil {
		return 0
	}
	if errors.Is(err, ui.ErrCancelled) {
		ui.Println(ui.Muted.Render(i18n.T("ui_cancelled")))
		return 130
	}
	var exit exitCode
	if errors.As(err, &exit) {
		return int(exit)
	}
	ui.Report(err)
	return 1
}

type exitCode int

func (e exitCode) Error() string { return fmt.Sprintf("exit %d", int(e)) }

func newRoot() *cobra.Command {
	root := &cobra.Command{
		Use:           "ipalpha",
		Short:         i18n.T("cli_short"),
		Long:          i18n.T("cli_long"),
		Version:       Version,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.CompletionOptions.HiddenDefaultCmd = true
	root.SetFlagErrorFunc(func(c *cobra.Command, err error) error {
		return ui.NewProblem(c.CommandPath(), err.Error(), c.CommandPath()+" --help")
	})
	root.AddCommand(setupCmd(), runCmd(), stopCmd(), statusCmd(), logsCmd(), pullCmd(), publishCmd(), featureCmd(),
		aiCmd(), configCmd(), doctorCmd(), setKeysCmd(), appsCmd(), updateCmd())
	return root
}

// open finds the workspace around the current folder (or IPALPHA_ROOT) and starts the command log.
func open(name string) (*workspace.Workspace, error) {
	start := os.Getenv("IPALPHA_ROOT")
	if start == "" {
		start = "."
	}
	root, err := workspace.Find(start)
	if err != nil {
		exe, _ := os.Executable()
		// The workspace copy lives in <root>/.ipalpha/bin: use it when called from elsewhere.
		if dir := filepath.Dir(filepath.Dir(exe)); filepath.Base(dir) == workspace.DirName {
			root, err = workspace.Find(filepath.Dir(dir))
		}
	}
	if err != nil {
		return nil, ui.NewProblem(i18n.T("cli_no_workspace_step"), i18n.T("cli_no_workspace"), i18n.T("cli_fix_cd"), i18n.T("cli_fix_setup"))
	}
	w, err := workspace.Open(root)
	if err != nil {
		return nil, ui.Wrap(i18n.T("cli_no_workspace_step"), err)
	}
	if w.Settings.Lang != "" && os.Getenv("IPALPHA_LANG") == "" {
		i18n.Set(w.Settings.Lang)
	}
	ui.OpenLog(w.LogsDir(), name)
	return w, nil
}

func self() string {
	exe, err := os.Executable()
	if err != nil {
		return os.Args[0]
	}
	if r, err := filepath.EvalSymlinks(exe); err == nil {
		return r
	}
	return exe
}

func joinLines(lines ...string) string { return strings.Join(lines, "\n") }
