package main

import (
	"fmt"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// portHolder is a process listening on a TCP port. pid is the one lsof reports
// (the listener); pgid is its process group, which is what we signal so a
// parent shell or npm wrapper dies with it.
type portHolder struct {
	pid  int
	pgid int
	cmd  string
}

// listenersOn reports who is listening on a TCP port, excluding the panel's
// own copy of that service. A stale copy from an earlier ./run is what blocks
// a restart; the current one must survive.
func listenersOn(port string, ownPID int) ([]portHolder, error) {
	if _, err := strconv.Atoi(port); err != nil {
		return nil, fmt.Errorf("bad port %q", port)
	}
	var out []byte
	var err error
	switch runtime.GOOS {
	case "darwin", "linux":
		out, err = exec.Command("lsof", "-nP", "-iTCP:"+port, "-sTCP:LISTEN", "-Fpc").Output()
	default:
		return nil, fmt.Errorf("unsupported os %s", runtime.GOOS)
	}
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok && ee.ExitCode() == 1 {
			return nil, nil
		}
		return nil, err
	}
	return parseLsof(string(out), ownPID), nil
}

func parseLsof(raw string, skipPID int) []portHolder {
	var holders []portHolder
	seen := map[int]bool{}
	var cur portHolder
	flush := func() {
		if cur.pid == 0 || seen[cur.pid] || cur.pid == skipPID {
			cur = portHolder{}
			return
		}
		if g, err := syscall.Getpgid(cur.pid); err == nil {
			cur.pgid = g
		}
		seen[cur.pid] = true
		holders = append(holders, cur)
		cur = portHolder{}
	}
	for _, line := range strings.Split(raw, "\n") {
		if line == "" {
			continue
		}
		switch line[0] {
		case 'p':
			flush()
			cur.pid, _ = strconv.Atoi(line[1:])
		case 'c':
			cur.cmd = line[1:]
		}
	}
	flush()
	return holders
}

// freePort signals every other listener on the port: TERM first, then KILL for
// whoever is still there. It returns how many were signaled.
func freePort(port string, ownPID int) (int, error) {
	holders, err := listenersOn(port, ownPID)
	if err != nil || len(holders) == 0 {
		return 0, err
	}
	for _, h := range holders {
		signalGroup(h, syscall.SIGTERM)
	}
	deadline := time.Now().Add(1500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if !anyAlive(holders) {
			return len(holders), nil
		}
		time.Sleep(50 * time.Millisecond)
	}
	for _, h := range holders {
		if alive(h.pid) {
			signalGroup(h, syscall.SIGKILL)
		}
	}
	return len(holders), nil
}

func anyAlive(holders []portHolder) bool {
	for _, h := range holders {
		if alive(h.pid) {
			return true
		}
	}
	return false
}

func alive(pid int) bool {
	return syscall.Kill(pid, 0) == nil
}

// signalGroup aims at the listener itself, not its process group: lsof's pid
// is the one holding the socket, while its group often belongs to a parent
// shell that must stay alive.
func signalGroup(h portHolder, sig syscall.Signal) error {
	return syscall.Kill(h.pid, sig)
}
