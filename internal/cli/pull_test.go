package cli

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMovedWorktree(t *testing.T) {
	root := t.TempDir()
	wt := filepath.Join(root, "features", "hello", "core", "forms-api")
	os.MkdirAll(wt, 0o755)
	// Git prints forward slashes on every OS.
	nw, ok := movedWorktree(root, filepath.ToSlash(wt), "core/forms-api", "apps/forms/forms-api")
	if !ok || filepath.Clean(nw) != filepath.Join(root, "features", "hello", "apps", "forms", "forms-api") {
		t.Fatalf("%q %v", nw, ok)
	}
	for _, other := range []string{
		filepath.Join(root, "core", "forms-api"),                         // the main clone itself
		filepath.Join(root, "features", "a", "b", "core", "forms-api"),   // not features/<slug>/core
		filepath.Join(t.TempDir(), "features", "x", "core", "forms-api"), // another workspace
	} {
		if _, ok := movedWorktree(root, filepath.ToSlash(other), "core/forms-api", "apps/forms/forms-api"); ok {
			t.Errorf("moved %s", other)
		}
	}
}
