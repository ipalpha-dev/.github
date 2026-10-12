package e2e

import (
	"strings"
	"testing"
)

func TestStatusUsesSameGroupedProcessOrderAsPanel(t *testing.T) {
	e := newEnv(t)
	e.setup()
	output := e.must(e.root, "status")
	previous := -1
	for _, label := range []string{"Core services", "Frontends", "Apps", "Forms API", "Forms Web", "Sign API", "Sign Web", "Infrastructure"} {
		position := strings.Index(output, label)
		if position < 0 || position <= previous {
			t.Fatalf("%s missing or out of order in status:\n%s", label, output)
		}
		previous = position
	}
	for _, label := range []string{"Forms API", "Forms Web", "Sign API", "Sign Web"} {
		if strings.Count(output, label) != 1 {
			t.Fatalf("%s is duplicated in status", label)
		}
	}
}
