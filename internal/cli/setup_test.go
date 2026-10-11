package cli

import (
	"errors"
	"testing"
	"time"

	"github.com/ipalpha-dev/tooling/internal/tools"
)

// Unattended setup (--yes, CI, install.sh piped without a terminal) with a required tool that the package
// manager cannot install must stop with the fix, not retry the same default forever (it once wrote a 7.7 GB
// log on a machine without Node.js).
func TestEnsureToolsUnattendedStopsOnMissingRequiredTool(t *testing.T) {
	saved := tools.All
	t.Cleanup(func() { tools.All = saved })
	checks := 0
	tools.All = []tools.Tool{{ID: "node", Name: "Node.js", Required: true, Why: "tool_why_node", URL: "https://nodejs.org",
		Check: func() (string, error) { checks++; return "", errors.New(`exec: "node": executable file not found in $PATH`) }}}

	done := make(chan error, 1)
	go func() { done <- ensureTools(true) }()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("a missing required tool must fail an unattended setup")
		}
		if checks > 3 {
			t.Fatalf("checked %d times: the unattended default must be tried once", checks)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("ensureTools loops forever when unattended")
	}
}
