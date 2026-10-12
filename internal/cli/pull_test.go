package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/ipalpha-dev/tooling/internal/workspace"
)

func TestMovedWorktree(t *testing.T) {
	root := t.TempDir()
	wt := filepath.Join(root, "features", "hello", "apps", "forms", "forms-api")
	os.MkdirAll(wt, 0o755)
	// Git prints forward slashes on every OS.
	nw, ok := movedWorktree(root, filepath.ToSlash(wt), "apps/forms/forms-api", "core/forms-api")
	if !ok || filepath.Clean(nw) != filepath.Join(root, "features", "hello", "core", "forms-api") {
		t.Fatalf("%q %v", nw, ok)
	}
	for _, other := range []string{
		filepath.Join(root, "apps", "forms", "forms-api"),                         // the main clone itself
		filepath.Join(root, "features", "a", "b", "apps", "forms", "forms-api"),   // not features/<slug>/apps
		filepath.Join(t.TempDir(), "features", "x", "apps", "forms", "forms-api"), // another workspace
	} {
		if _, ok := movedWorktree(root, filepath.ToSlash(other), "apps/forms/forms-api", "core/forms-api"); ok {
			t.Errorf("moved %s", other)
		}
	}
}

func gitInit(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("git", "-C", dir, "init", "-q").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v %s", err, out)
	}
}

// Forms and Sign left apps/ for core/: the old layout is moved (never deleted), work and .env kept,
// emptied app folders removed, and a second run changes nothing.
func TestMigrateLayoutFormsSignToCore(t *testing.T) {
	root := t.TempDir()
	for _, rel := range []string{"apps/forms/forms-api", "apps/forms/forms-webapp", "apps/sign/sign-api", "apps/sign/sign-webapp"} {
		dir := filepath.Join(root, filepath.FromSlash(rel))
		gitInit(t, dir)
		os.WriteFile(filepath.Join(dir, ".env"), []byte("KEEP=1\n"), 0o644)
	}
	os.MkdirAll(filepath.Join(root, "apps", "acampa-kids", "backend"), 0o755)
	w := &workspace.Workspace{Root: root, Settings: workspace.NewSettings()}
	for i := 0; i < 2; i++ {
		migrateLayout(w)
		for _, name := range []string{"forms-api", "forms-webapp", "sign-api", "sign-webapp"} {
			b, err := os.ReadFile(filepath.Join(root, "core", name, ".env"))
			if err != nil || string(b) != "KEEP=1\n" {
				t.Fatalf("run %d: core/%s/.env: %q %v", i, name, b, err)
			}
		}
		for _, gone := range []string{"apps/forms", "apps/sign"} {
			if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(gone))); !os.IsNotExist(err) {
				t.Errorf("run %d: %s still exists (%v)", i, gone, err)
			}
		}
		if _, err := os.Stat(filepath.Join(root, "apps", "acampa-kids", "backend")); err != nil {
			t.Fatalf("other apps must stay: %v", err)
		}
	}
}

// Both layouts present: nothing moves, nothing is deleted.
func TestMigrateLayoutKeepsBoth(t *testing.T) {
	root := t.TempDir()
	old := filepath.Join(root, "apps", "forms", "forms-api")
	gitInit(t, old)
	os.WriteFile(filepath.Join(old, "work.txt"), []byte("old"), 0o644)
	gitInit(t, filepath.Join(root, "core", "forms-api"))
	migrateLayout(&workspace.Workspace{Root: root, Settings: workspace.NewSettings()})
	if b, err := os.ReadFile(filepath.Join(old, "work.txt")); err != nil || string(b) != "old" {
		t.Fatalf("old copy touched: %q %v", b, err)
	}
	if _, err := os.Stat(filepath.Join(root, "core", "forms-api", "work.txt")); !os.IsNotExist(err) {
		t.Fatalf("moved into an existing folder: %v", err)
	}
}
