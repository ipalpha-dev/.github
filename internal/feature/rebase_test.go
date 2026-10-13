package feature

import (
	"os"
	"path/filepath"
	"testing"
)

func rebaseRepo(t *testing.T) (dir, base string) {
	dir = t.TempDir()
	gitIn(t, dir, "init", "-q", "-b", "master")
	base = commit(t, dir, "a", "base")
	gitIn(t, dir, "checkout", "-q", "-b", "feat/x")
	gitIn(t, dir, "checkout", "-q", "master")
	return dir, base
}

// The feature branch already equals master but its stored base is older: nothing to replay.
func TestRebaseOntoFeatureEqualsMaster(t *testing.T) {
	dir, _ := rebaseRepo(t)
	commit(t, dir, "b", "owner one")
	tip := commit(t, dir, "c", "owner two")
	gitIn(t, dir, "checkout", "-q", "feat/x")
	gitIn(t, dir, "merge", "-q", "--ff-only", "master")
	if res := rebaseOnto(dir, tip); res.Err != nil {
		t.Fatalf("rebase: %v\n%s", res.Err, res.Output)
	}
	if got := gitIn(t, dir, "rev-parse", "HEAD"); got != tip {
		t.Fatalf("HEAD %s want %s", got, tip)
	}
}

// A feature branch with no commits beyond its old base fast-forwards.
func TestRebaseOntoFastForwards(t *testing.T) {
	dir, _ := rebaseRepo(t)
	tip := commit(t, dir, "b", "owner")
	gitIn(t, dir, "checkout", "-q", "feat/x")
	if res := rebaseOnto(dir, tip); res.Err != nil {
		t.Fatalf("rebase: %v\n%s", res.Err, res.Output)
	}
	if got := gitIn(t, dir, "rev-parse", "HEAD"); got != tip {
		t.Fatalf("HEAD %s want %s", got, tip)
	}
}

// A feature commit whose patch already landed on master (cherry-pick) is skipped; the rest is replayed.
func TestRebaseOntoSkipsAppliedPatches(t *testing.T) {
	dir, _ := rebaseRepo(t)
	gitIn(t, dir, "checkout", "-q", "feat/x")
	_ = os.WriteFile(filepath.Join(dir, "shared"), []byte("s"), 0o644)
	gitIn(t, dir, "add", ".")
	gitIn(t, dir, "commit", "-q", "-m", "owner change")
	owner := gitIn(t, dir, "rev-parse", "HEAD")
	_ = os.WriteFile(filepath.Join(dir, "mine"), []byte("m"), 0o644)
	gitIn(t, dir, "add", ".")
	gitIn(t, dir, "commit", "-q", "-m", "feature work")
	gitIn(t, dir, "checkout", "-q", "master")
	gitIn(t, dir, "cherry-pick", owner)
	tip := gitIn(t, dir, "rev-parse", "HEAD")
	gitIn(t, dir, "checkout", "-q", "feat/x")
	if res := rebaseOnto(dir, tip); res.Err != nil {
		t.Fatalf("rebase: %v\n%s", res.Err, res.Output)
	}
	if n := gitIn(t, dir, "rev-list", "--count", tip+"..HEAD"); n != "1" {
		t.Fatalf("want 1 replayed commit, got %s", n)
	}
}

// A real conflict is aborted: no rebase stays in progress.
func TestRebaseOntoConflictAborts(t *testing.T) {
	dir, _ := rebaseRepo(t)
	gitIn(t, dir, "checkout", "-q", "feat/x")
	mine := commit(t, dir, "mine", "feature edit")
	gitIn(t, dir, "checkout", "-q", "master")
	tip := commit(t, dir, "theirs", "owner edit")
	gitIn(t, dir, "checkout", "-q", "feat/x")
	if res := rebaseOnto(dir, tip); res.Err == nil {
		t.Fatal("want conflict")
	}
	if _, err := os.Stat(filepath.Join(dir, ".git", "rebase-merge")); err == nil {
		t.Fatal("rebase left in progress")
	}
	if got := gitIn(t, dir, "rev-parse", "HEAD"); got != mine {
		t.Fatalf("HEAD moved: %s", got)
	}
}
