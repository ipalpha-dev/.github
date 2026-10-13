package deps

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ipalpha-dev/tooling/internal/catalog"
	"github.com/ipalpha-dev/tooling/internal/workspace"
)

func git(t *testing.T, dir string, args ...string) {
	t.Helper()
	out, err := exec.Command("git", append([]string{"-C", dir, "-c", "user.email=t@t", "-c", "user.name=t"}, args...)...).CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

// shared-js moving (new HEAD or edited source) makes the next Plan rebuild it and reinstall copies;
// an unchanged stamp plans nothing.
func TestSharedJsMovePlansRebuildAndReinstall(t *testing.T) {
	root := t.TempDir()
	lib, _ := catalog.Get("shared-js")
	api, _ := catalog.Get("persons-api")
	libDir, apiDir := lib.Path(root), api.Path(root)
	for _, d := range []string{filepath.Join(libDir, "src"), filepath.Join(libDir, "dist"),
		filepath.Join(libDir, "node_modules"), filepath.Join(apiDir, "node_modules", "@ipalpha", "shared-js", "dist")} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	_ = os.WriteFile(filepath.Join(libDir, "package.json"), []byte("{}"), 0o644)
	_ = os.WriteFile(filepath.Join(apiDir, "package.json"), []byte("{}"), 0o644)
	_ = os.WriteFile(filepath.Join(libDir, "src", "a.ts"), []byte("1"), 0o644)
	git(t, libDir, "init", "-q", "-b", "master")
	git(t, libDir, "add", "src", "package.json")
	git(t, libDir, "commit", "-q", "-m", "one")
	// make dist and node_modules newer than every source so only the stamp can trigger work
	future := mtimeFuture()
	for _, p := range []string{filepath.Join(libDir, "dist"), filepath.Join(libDir, "node_modules"), filepath.Join(apiDir, "node_modules"),
		filepath.Join(apiDir, "node_modules", "@ipalpha", "shared-js", "dist")} {
		_ = os.Chtimes(p, future, future)
	}
	w := &workspace.Workspace{Root: root, Settings: workspace.NewSettings()}

	if !SharedStale(w) {
		t.Fatal("no stamp yet: expected stale")
	}
	writeStamp(w, SharedStamp(w))
	if SharedStale(w) {
		t.Fatal("stamp written: expected current")
	}
	if p := Plan(w); len(p) != 0 {
		t.Fatalf("unchanged shared-js must plan nothing, got %v", p)
	}

	_ = os.WriteFile(filepath.Join(libDir, "src", "a.ts"), []byte("2"), 0o644)
	git(t, libDir, "commit", "-q", "-am", "two")
	_ = os.Chtimes(filepath.Join(libDir, "src", "a.ts"), future.Add(-1), future.Add(-1))
	if !SharedStale(w) {
		t.Fatal("new HEAD: expected stale")
	}
	var seen []string
	for _, phase := range Plan(w) {
		for _, tk := range phase {
			seen = append(seen, tk.Repo.Name+":"+tk.Action)
		}
	}
	got := strings.Join(seen, ",")
	if !strings.Contains(got, "shared-js:build") || !strings.Contains(got, "persons-api:install") {
		t.Fatalf("want shared-js build + persons-api install, got %s", got)
	}

	// uncommitted edits move the stamp too
	writeStamp(w, SharedStamp(w))
	_ = os.WriteFile(filepath.Join(libDir, "src", "a.ts"), []byte("3"), 0o644)
	_ = os.Chtimes(filepath.Join(libDir, "src", "a.ts"), future.Add(-1), future.Add(-1))
	if !SharedStale(w) {
		t.Fatal("dirty source: expected stale")
	}
}

// Without a shared-js checkout nothing is stale and SyncShared is a no-op.
func TestSharedStaleWithoutSharedJs(t *testing.T) {
	w := &workspace.Workspace{Root: t.TempDir(), Settings: workspace.NewSettings()}
	if SharedStale(w) || SyncShared(w, nil) != nil {
		t.Fatal("expected no-op")
	}
}

func mtimeFuture() time.Time { return time.Now().Add(time.Hour) }
