package repos

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/ipalpha-dev/tooling/internal/catalog"
)

var newRepos = []string{"drive-api", "drive-webapp", "services-kids-api", "services-kids-webapp", "virtual-assistant"}

func git(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir, "-c", "user.name=t", "-c", "user.email=t@t"}, args...)...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v %s", args, err, out)
	}
}

// The five new repos have an origin (no NoRemote), are cloned by pull, and an empty "chore: initial" master
// (no package.json) clones and fast-forwards without failing.
func TestNewReposCloneAndPullFromEmptyMaster(t *testing.T) {
	from := t.TempDir()
	for _, n := range newRepos {
		bare := filepath.Join(from, n+".git")
		git(t, from, "init", "-q", "--bare", "-b", "master", bare)
		work := filepath.Join(t.TempDir(), "w")
		git(t, from, "clone", "-q", bare, work)
		git(t, work, "checkout", "-q", "-B", "master")
		git(t, work, "commit", "-q", "--allow-empty", "-m", "chore: initial")
		git(t, work, "push", "-q", "origin", "master")
		git(t, work, "push", "-q", "origin", "master:refs/heads/feat/x")
	}
	t.Setenv("IPALPHA_CLONE_FROM", from)
	root := t.TempDir()
	for _, name := range newRepos {
		r, ok := catalog.Get(name)
		if !ok || r.SkippedWhenMissing() {
			t.Fatalf("%s: missing or still skipped-when-missing", name)
		}
	}
	for _, mode := range []string{"clone", "pull"} {
		outs := PullAll("o", root, false, Access{}, nil) // first run clones, second fast-forwards
		for _, o := range outs {
			for _, n := range newRepos {
				if o.Repo.Name == n && o.Status != "cloned" && o.Status != "current" {
					t.Fatalf("%s %s: status %s %s", mode, n, o.Status, o.Detail)
				}
			}
		}
	}
	for _, n := range newRepos {
		r, _ := catalog.Get(n)
		dir := r.Path(root)
		if _, err := os.Stat(filepath.Join(dir, "package.json")); err == nil {
			t.Fatalf("%s should have no package.json", n)
		}
		if out, _ := exec.Command("git", "-C", dir, "rev-parse", "--verify", "-q", "origin/feat/x").Output(); len(out) == 0 {
			t.Fatalf("%s: origin/feat/x not fetched", n)
		}
	}
}
