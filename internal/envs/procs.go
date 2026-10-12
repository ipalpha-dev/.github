package envs

import (
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/ipalpha-dev/tooling/internal/sys"
)

var agentNames = map[string]bool{"claude": true, "pi": true, "codex": true, "grok": true}

// listProcs finds agent processes (claude, pi, codex, grok) and their working folders with ps and
// lsof. Best effort: any failure (or Windows) yields no agents.
func listProcs() []Proc {
	if sys.Windows {
		return nil
	}
	out, err := exec.Command("ps", "-axo", "pid=,args=").Output()
	if err != nil {
		return nil
	}
	names := ParseAgents(string(out))
	if len(names) == 0 {
		return nil
	}
	pids := make([]string, 0, len(names))
	for pid := range names {
		pids = append(pids, pid)
	}
	raw, _ := exec.Command("lsof", "-a", "-d", "cwd", "-Fpn", "-p", strings.Join(pids, ",")).Output()
	return ParseCwds(string(raw), names)
}

// ParseAgents picks agent processes out of `ps -axo pid=,args=`: pid → agent name. The program is the
// first word of the command, or the second when a runtime (node, bun) starts a script.
func ParseAgents(ps string) map[string]string {
	out := map[string]string{}
	for _, line := range strings.Split(ps, "\n") {
		f := strings.Fields(line)
		if len(f) < 2 {
			continue
		}
		prog := filepath.Base(f[1])
		if (prog == "node" || prog == "bun") && len(f) > 2 {
			prog = filepath.Base(f[2])
		}
		if agentNames[prog] {
			out[f[0]] = prog
		}
	}
	return out
}

// ParseCwds reads `lsof -Fpn` output (p<pid> then n<path>) into processes.
func ParseCwds(lsof string, names map[string]string) []Proc {
	var procs []Proc
	pid := ""
	for _, line := range strings.Split(lsof, "\n") {
		switch {
		case strings.HasPrefix(line, "p"):
			pid = line[1:]
		case strings.HasPrefix(line, "n") && pid != "":
			if n := names[pid]; n != "" {
				procs = append(procs, Proc{Name: n, Cwd: line[1:]})
			}
		}
	}
	return procs
}
