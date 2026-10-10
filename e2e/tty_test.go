//go:build !windows

package e2e

import (
	"bytes"
	"io"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/creack/pty"

	"github.com/ipalpha-dev/tooling/internal/envfile"
)

// The interactive prompts really return what the person typed (a terminal, not stdin pipes).
func TestInteractiveConfigAnswers(t *testing.T) {
	e := newEnv(t)
	e.setup()
	c := exec.Command(bin, "ai")
	c.Dir = e.root
	var vars []string
	for _, v := range e.vars {
		if !strings.HasPrefix(v, "CI=") && !strings.HasPrefix(v, "IPALPHA_AI_CLI=") {
			vars = append(vars, v)
		}
	}
	c.Env = append(vars, "TERM=xterm-256color")
	f, err := pty.StartWithSize(c, &pty.Winsize{Rows: 40, Cols: 120})
	if err != nil {
		t.Skip(err)
	}
	defer f.Close()
	var mu sync.Mutex
	var out bytes.Buffer
	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := f.Read(buf)
			chunk := string(buf[:n])
			// Answer like a terminal: background color (OSC 11) and cursor position (CSI 6n).
			if strings.Contains(chunk, "\x1b]11;?") {
				f.Write([]byte("\x1b]11;rgb:0000/0000/0000\x1b\\"))
			}
			if strings.Contains(chunk, "\x1b[6n") {
				f.Write([]byte("\x1b[1;1R"))
			}
			mu.Lock()
			out.Write(buf[:n])
			mu.Unlock()
			if err != nil {
				if err != io.EOF {
					return
				}
				return
			}
		}
	}()
	waitFor := func(s string) {
		t.Helper()
		deadline := time.Now().Add(20 * time.Second)
		for time.Now().Before(deadline) {
			mu.Lock()
			ok := strings.Contains(out.String(), s)
			mu.Unlock()
			if ok {
				return
			}
			time.Sleep(100 * time.Millisecond)
		}
		mu.Lock()
		defer mu.Unlock()
		t.Fatalf("timed out waiting for %q; screen:\n%s", s, out.String())
	}
	waitFor("Which AI")
	// "Another command" sits right above "None" (the default): go up one and type a custom command.
	f.Write([]byte("\x1b[A"))
	time.Sleep(200 * time.Millisecond)
	f.Write([]byte("\r"))
	waitFor("Command")
	f.Write([]byte("cat"))
	time.Sleep(200 * time.Millisecond)
	f.Write([]byte("\r"))
	waitFor("AI working: custom: cat") // cat echoes the prompt, whose example JSON is a valid decision
	done := make(chan error, 1)
	go func() { done <- c.Wait() }()
	select {
	case <-done:
	case <-time.After(20 * time.Second):
		c.Process.Kill()
		t.Fatal("ai did not exit")
	}
	s := envfile.Read(filepath.Join(e.root, ".ipalpha", "settings"))
	if s["ai_cli"] != "custom" || s["ai_command"] != "cat" {
		t.Fatalf("typed answers were not saved: ai_cli=%q ai_command=%q", s["ai_cli"], s["ai_command"])
	}
}
