package feature

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPublishedOnlyWhenDeploymentMasterHasRecord(t *testing.T) {
	origin := t.TempDir()
	gitIn(t, origin, "init", "-q", "-b", "master")
	commit(t, origin, "a", "one")
	main := t.TempDir()
	dep := filepath.Join(main, "deployment")
	gitIn(t, main, "clone", "-q", origin, dep)
	if published(main, "ghost") {
		t.Fatal("never-published feature must be local only")
	}
	_ = os.MkdirAll(filepath.Join(origin, "previews", "real"), 0o755)
	_ = os.WriteFile(filepath.Join(origin, "previews", "real", "release.json"), []byte("{}"), 0o644)
	gitIn(t, origin, "add", ".")
	gitIn(t, origin, "commit", "-q", "-m", "rec")
	if !published(main, "real") {
		t.Fatal("published feature must be detected")
	}
}
