package feature

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

func gitIn(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir, "-c", "user.email=t@t", "-c", "user.name=t"}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func commit(t *testing.T, dir, content, msg string) string {
	t.Helper()
	_ = os.WriteFile(dir+"/a", []byte(content), 0o644)
	gitIn(t, dir, "add", ".")
	gitIn(t, dir, "commit", "-q", "-m", msg)
	return gitIn(t, dir, "rev-parse", "HEAD")
}

func TestLocalTipIsLocalDefaultBranch(t *testing.T) {
	for _, def := range []string{"master", "main"} {
		dir := t.TempDir()
		gitIn(t, dir, "init", "-q", "-b", def)
		commit(t, dir, "a", "one")
		tip := commit(t, dir, "b", "two") // local, never pushed
		// unrelated history on another branch (e.g. an old "public baseline") must not matter
		gitIn(t, dir, "checkout", "-q", "--orphan", "old-baseline")
		commit(t, dir, "x", "old baseline")
		gitIn(t, dir, "checkout", "-q", def)

		got, err := LocalTip(dir, "persons-api")
		if err != nil || got != tip {
			t.Fatalf("%s: got %q err %v, want local tip %q", def, got, err, tip)
		}
	}
}

func TestLocalTipPrefersOriginHeadBranch(t *testing.T) {
	origin := t.TempDir()
	gitIn(t, origin, "init", "-q", "-b", "main")
	commit(t, origin, "a", "one")
	clone := t.TempDir()
	gitIn(t, clone, "clone", "-q", origin, ".")
	gitIn(t, clone, "branch", "master") // stray branch; origin/HEAD says main
	tip := commit(t, clone, "b", "local")
	got, err := LocalTip(clone, "x")
	if err != nil || got != tip {
		t.Fatalf("got %q err %v, want %q", got, err, tip)
	}
}

func TestLocalTipWithoutDefaultBranch(t *testing.T) {
	dir := t.TempDir()
	gitIn(t, dir, "init", "-q", "-b", "trunk")
	commit(t, dir, "a", "one")
	if _, err := LocalTip(dir, "persons-api"); err == nil || !strings.Contains(err.Error(), "persons-api") {
		t.Fatalf("want error naming the repo, got %v", err)
	}
}
