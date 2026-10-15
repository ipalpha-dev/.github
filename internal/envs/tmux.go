package envs

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"

	"github.com/ipalpha-dev/tooling/internal/i18n"
	"github.com/ipalpha-dev/tooling/internal/ui"
)

// Runner runs tmux. Output captures it; Interactive hands it the terminal (attach).
type Runner interface {
	Output(args ...string) (string, error)
	Interactive(args ...string) error
}

// ExecRunner is the real tmux.
type ExecRunner struct{}

// Output runs tmux and returns stdout.
func (ExecRunner) Output(args ...string) (string, error) {
	out, err := exec.Command("tmux", args...).Output()
	return strings.TrimSpace(string(out)), err
}

// Interactive runs tmux attached to this terminal.
func (ExecRunner) Interactive(args ...string) error {
	c := exec.Command("tmux", args...)
	c.Stdin, c.Stdout, c.Stderr = os.Stdin, os.Stdout, os.Stderr
	return c.Run()
}

// Tmux builds and runs the tmux commands of the picker.
type Tmux struct {
	R         Runner
	Installed bool
	Inside    bool // this terminal is already inside tmux ($TMUX)
}

// NewTmux detects tmux on this computer.
func NewTmux() *Tmux {
	_, err := exec.LookPath("tmux")
	return &Tmux{R: ExecRunner{}, Installed: err == nil, Inside: os.Getenv("TMUX") != ""}
}

// ErrNoTmux explains how to install tmux.
func ErrNoTmux() error {
	hint := "brew install tmux"
	switch runtime.GOOS {
	case "linux":
		hint = "sudo apt install tmux"
	case "windows":
		hint = "WSL: sudo apt install tmux"
	}
	return ui.NewProblem("tmux", i18n.T("envs_no_tmux"), hint)
}

func exact(session string) string { return "=" + session }

// Sessions maps each tmux session to its window names in window-index order (empty when tmux
// is missing or no server runs). A session always has at least one window.
func (t *Tmux) Sessions() map[string][]string {
	out := map[string][]string{}
	if !t.Installed {
		return out
	}
	s, err := t.R.Output("list-windows", "-a", "-F", "#{session_name}\t#{window_index}\t#{window_name}")
	if err != nil {
		return out
	}
	type win struct {
		idx  int
		name string
	}
	by := map[string][]win{}
	for _, line := range strings.Split(s, "\n") {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "\t", 3)
		if len(parts) != 3 {
			continue
		}
		idx, _ := strconv.Atoi(parts[1])
		by[parts[0]] = append(by[parts[0]], win{idx, parts[2]})
	}
	for session, wins := range by {
		sort.SliceStable(wins, func(i, j int) bool { return wins[i].idx < wins[j].idx })
		names := make([]string, len(wins))
		for i, w := range wins {
			names[i] = w.name
		}
		out[session] = names
	}
	return out
}

// Ensure creates the detached session in dir when it does not exist.
func (t *Tmux) Ensure(session, dir string) error {
	if _, err := t.R.Output("has-session", "-t", exact(session)); err == nil {
		return nil
	}
	_, err := t.R.Output("new-session", "-d", "-s", session, "-c", dir)
	return err
}

// hasWindow reports whether the session has a window with that name.
func (t *Tmux) hasWindow(session, name string) bool {
	s, err := t.R.Output("list-windows", "-t", exact(session), "-F", "#{window_name}")
	if err != nil {
		return false
	}
	for _, n := range strings.Split(s, "\n") {
		if n == name {
			return true
		}
	}
	return false
}

// Window selects window name of the session, creating it (running command in dir) when missing.
// An empty command opens the shell. Reports whether the window was created.
func (t *Tmux) Window(session, name, dir, command string) (bool, error) {
	if name != "" && t.hasWindow(session, name) {
		_, err := t.R.Output("select-window", "-t", exact(session)+":="+name)
		return false, err
	}
	args := []string{"new-window", "-t", exact(session) + ":", "-c", dir}
	if name != "" {
		args = append(args, "-n", name)
	}
	if command != "" {
		args = append(args, command)
	}
	_, err := t.R.Output(args...)
	return true, err
}

// Hop moves this terminal to the session: switch-client inside tmux, attach outside.
func (t *Tmux) Hop(session string) error {
	if t.Inside {
		_, err := t.R.Output("switch-client", "-t", exact(session))
		return err
	}
	return t.R.Interactive("attach-session", "-t", exact(session))
}

// Kind of action the picker can run on a row.
type Kind int

// Actions of the picker.
const (
	ActHop Kind = iota
	ActShell
	ActLogs
	ActRun
)

// Do runs an action on a row: make sure its session exists, set up the window, then hop.
func (t *Tmux) Do(act Kind, name, root string) error {
	if !t.Installed {
		return ErrNoTmux()
	}
	session := SessionName(name)
	if err := t.Ensure(session, root); err != nil {
		return err
	}
	var err error
	switch act {
	case ActShell:
		_, err = t.Window(session, "", root, "")
	case ActLogs:
		_, err = t.Window(session, "logs", root, keepOpen(shellQuote(filepath.Join(root, "logs"))))
	case ActRun:
		_, err = t.Window(session, "run", root, keepOpen(shellQuote(filepath.Join(root, "run"))))
	}
	if err != nil {
		return err
	}
	return t.Hop(session)
}

// StartRun is `./run --tmux`: when this terminal is outside tmux, start ./run (args) in the `run`
// window of the workspace session and hop there. It reports false when ./run should just run here
// (already inside tmux, or tmux is missing).
func (t *Tmux) StartRun(name, root string, args []string) (bool, error) {
	if t.Inside || !t.Installed {
		return false, nil
	}
	session := SessionName(name)
	if err := t.Ensure(session, root); err != nil {
		return false, err
	}
	cmd := shellQuote(filepath.Join(root, "run"))
	for _, a := range args {
		cmd += " " + shellQuote(a)
	}
	if _, err := t.Window(session, "run", root, keepOpen(cmd)); err != nil {
		return false, err
	}
	return true, t.Hop(session)
}

// keepOpen leaves the window readable when the command fails.
func keepOpen(cmd string) string {
	return cmd + ` || { s=$?; echo; echo "exit $s"; read x; }`
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
