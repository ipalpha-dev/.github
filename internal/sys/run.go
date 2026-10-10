package sys

import (
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
)

// Cmd describes a command whose output is captured for an error box and mirrored into a log.
type Cmd struct {
	Dir  string
	Env  []string // added to os.Environ()
	Name string
	Args []string
	Log  io.Writer // optional mirror (the command's log file)
}

// Result of a captured command.
type Result struct {
	Output string // combined stdout+stderr
	Err    error
}

// Run executes the command, capturing combined output (bounded to the last 512 KiB).
func (c Cmd) Run() Result {
	cmd := exec.Command(c.Name, c.Args...)
	cmd.Dir = c.Dir
	cmd.Env = append(os.Environ(), c.Env...)
	buf := &tailBuffer{max: 512 * 1024}
	var w io.Writer = buf
	if c.Log != nil {
		w = io.MultiWriter(buf, c.Log)
	}
	cmd.Stdout, cmd.Stderr = w, w
	err := cmd.Run()
	return Result{Output: buf.String(), Err: err}
}

type tailBuffer struct {
	mu  sync.Mutex
	b   []byte
	max int
}

func (t *tailBuffer) Write(p []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.b = append(t.b, p...)
	if len(t.b) > t.max {
		t.b = t.b[len(t.b)-t.max:]
	}
	return len(p), nil
}

func (t *tailBuffer) String() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return strings.ReplaceAll(string(t.b), "\r\n", "\n")
}

// Git runs git -C dir args… and returns trimmed stdout.
func Git(dir string, args ...string) (string, error) {
	return Output("", "git", append([]string{"-C", dir}, args...)...)
}

// GitOK runs git and reports success only.
func GitOK(dir string, args ...string) bool {
	c := exec.Command("git", append([]string{"-C", dir}, args...)...)
	return c.Run() == nil
}
