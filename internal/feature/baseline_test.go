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

func TestResolveBaselineCommit(t *testing.T) {
	origin := t.TempDir()
	gitIn(t, origin, "init", "-q", "-b", "master")
	_ = os.WriteFile(origin+"/a", []byte("a"), 0o644)
	gitIn(t, origin, "add", ".")
	gitIn(t, origin, "commit", "-q", "-m", "one")
	first := gitIn(t, origin, "rev-parse", "HEAD")
	_ = os.WriteFile(origin+"/a", []byte("b"), 0o644)
	gitIn(t, origin, "commit", "-q", "-am", "two")
	tip := gitIn(t, origin, "rev-parse", "HEAD")

	clone := t.TempDir()
	gitIn(t, clone, "clone", "-q", origin, ".")

	got, warn, err := resolveBaselineCommit(clone, "persons-api", first)
	if err != nil || got != first || warn != "" {
		t.Fatalf("existing commit: got %q warn %q err %v", got, warn, err)
	}

	missing := strings.Repeat("deadbeef", 5)
	got, warn, err = resolveBaselineCommit(clone, "persons-api", missing)
	if err != nil || got != tip {
		t.Fatalf("missing commit: got %q err %v, want tip %q", got, err, tip)
	}
	if !strings.Contains(warn, "persons-api") || !strings.Contains(warn, missing[:12]) || !strings.Contains(warn, tip[:12]) {
		t.Fatalf("warning lacks repo/shas: %q", warn)
	}
}
