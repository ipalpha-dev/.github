package materialize

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ipalpha-dev/tooling/internal/workspace"
)

// The tools never ship agent instructions: generated pointers from older versions are removed,
// files a developer wrote are kept, nothing new is created.
func TestRemoveAgentPointers(t *testing.T) {
	root := t.TempDir()
	w := &workspace.Workspace{Root: root, Settings: workspace.NewSettings()}
	write := func(rel, body string) string {
		p := filepath.Join(root, filepath.FromSlash(rel))
		os.MkdirAll(filepath.Dir(p), 0o755)
		os.WriteFile(p, []byte(body), 0o644)
		return p
	}
	gen := "<!-- ipalpha: points to AGENTS.md -->\n@AGENTS.md\n"
	generated := []string{
		write("CLAUDE.md", gen),
		write("core/oikos-webapp/GEMINI.md", gen),
		write("core/oikos-webapp/.github/copilot-instructions.md", gen),
		write("features/x/apps/forms/forms-api/CLAUDE.md", gen),
	}
	mine := []string{
		write("AGENTS.md", "my own rules\n"),
		write("core/auth-api/CLAUDE.md", "my own claude notes\n"),
	}
	removeAgentPointers(w)
	for _, p := range generated {
		if _, err := os.Stat(p); err == nil {
			t.Errorf("generated pointer kept: %s", p)
		}
	}
	for _, p := range mine {
		if _, err := os.Stat(p); err != nil {
			t.Errorf("hand-written file removed: %s", p)
		}
	}
	if _, err := os.Stat(filepath.Join(root, "core", "auth-api", "GEMINI.md")); err == nil {
		t.Error("a pointer was created")
	}
}
