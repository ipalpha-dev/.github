package deps

import (
	"os"
	"testing"

	"github.com/ipalpha-dev/tooling/internal/catalog"
	"github.com/ipalpha-dev/tooling/internal/workspace"
)

// A cloned repo whose master is empty (no package.json) is skipped by install planning.
func TestPlanSkipsReposWithoutPackageJSON(t *testing.T) {
	root := t.TempDir()
	for _, n := range []string{"drive-api", "drive-webapp", "virtual-assistant"} {
		r, _ := catalog.Get(n)
		if err := os.MkdirAll(r.Path(root), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	w := &workspace.Workspace{Root: root, Settings: workspace.NewSettings()}
	if phases := Plan(w); len(phases) != 0 {
		t.Fatalf("expected no install tasks, got %v", phases)
	}
}
