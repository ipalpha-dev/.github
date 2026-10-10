//go:build !windows

package sys

import (
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// Detach puts the child in its own process group so the whole tree (npm → sh → tsc + node) can be
// stopped together and Ctrl+C in the panel does not reach it twice.
func Detach(c *exec.Cmd) {
	if c.SysProcAttr == nil {
		c.SysProcAttr = &syscall.SysProcAttr{}
	}
	c.SysProcAttr.Setpgid = true
}

// Attach is a no-op on Unix (process groups are set by Detach).
func Attach(c *exec.Cmd) {}

// KillTree stops a started command and everything it spawned: SIGTERM to the group, SIGKILL after 3s.
func KillTree(c *exec.Cmd) {
	if c == nil || c.Process == nil {
		return
	}
	pid := c.Process.Pid
	pgid, err := syscall.Getpgid(pid)
	if err != nil {
		_ = c.Process.Signal(syscall.SIGTERM)
		return
	}
	_ = syscall.Kill(-pgid, syscall.SIGTERM)
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if syscall.Kill(-pgid, 0) != nil {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	_ = syscall.Kill(-pgid, syscall.SIGKILL)
}

// KillPID stops one process (the listener itself, never its parent's group): TERM, then KILL.
func KillPID(pid int) {
	_ = syscall.Kill(pid, syscall.SIGTERM)
	deadline := time.Now().Add(1500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if !Alive(pid) {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	_ = syscall.Kill(pid, syscall.SIGKILL)
}

// KillGroup stops every process of a group created by Detach (the leader may be gone already:
// tsc --watch and node keep the group alive). TERM, then KILL after 3s.
func KillGroup(pgid int) {
	if pgid <= 1 {
		return
	}
	_ = syscall.Kill(-pgid, syscall.SIGTERM)
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if syscall.Kill(-pgid, 0) != nil {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	_ = syscall.Kill(-pgid, syscall.SIGKILL)
}

// GroupAlive reports whether any process of the group still runs.
func GroupAlive(pgid int) bool { return pgid > 1 && syscall.Kill(-pgid, 0) == nil }

// Alive reports whether a process exists.
func Alive(pid int) bool {
	return pid > 0 && syscall.Kill(pid, 0) == nil
}

// ProcessName returns the command name of a pid ("" when unknown).
func ProcessName(pid int) string { return processName(pid) }

func processName(pid int) string {
	out, err := exec.Command("ps", "-o", "comm=", "-p", strconv.Itoa(pid)).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
