package deps

import (
	"os"
	"path/filepath"
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

// A Go checkout never plans npm work.
func TestPlanGoCheckoutHasNoNpm(t *testing.T) {
	root := t.TempDir()
	r, _ := catalog.Get("virtual-assistant")
	os.MkdirAll(r.Path(root), 0o755)
	os.WriteFile(filepath.Join(r.Path(root), "go.mod"), []byte("module x\n"), 0o644)
	w := &workspace.Workspace{Root: root, Settings: workspace.NewSettings()}
	for _, ph := range Plan(w) {
		for _, task := range ph {
			if task.Action == "install" && task.Repo.Name == "virtual-assistant" && !task.Repo.IsGo(task.Dir) {
				t.Fatal("npm task for a Go repo")
			}
		}
	}
}
