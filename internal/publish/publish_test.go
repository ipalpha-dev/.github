package publish

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ipalpha-dev/tooling/internal/catalog"
	"github.com/ipalpha-dev/tooling/internal/workspace"
)

func TestBumpAndClamp(t *testing.T) {
	for _, c := range [][3]string{{"1.2.3", "patch", "1.2.4"}, {"1.2.3", "minor", "1.3.0"}, {"1.2.3", "major", "2.0.0"}, {"1.2.3", "none", "1.2.3"}, {"x", "patch", "0.0.1"}} {
		if got := Bump(c[0], c[1]); got != c[2] {
			t.Errorf("Bump(%s,%s)=%s", c[0], c[1], got)
		}
	}
	if Clamp([]string{"README.md", "docs/a.png"}, "minor") != "none" {
		t.Fatal("docs-only must clamp")
	}
	if Clamp([]string{"README.md", "src/a.ts"}, "minor") != "minor" {
		t.Fatal("code change must keep the bump")
	}
}

func TestSetVersionKeepsFormatting(t *testing.T) {
	dir := t.TempDir()
	pkg := "{\n    \"name\": \"x\",\n    \"version\": \"1.0.0\",\n    \"dependencies\": { \"y\": \"^1.0.0\" }\n}\n"
	lock := `{
  "name": "x",
  "version": "1.0.0",
  "lockfileVersion": 3,
  "packages": {
    "": {
      "name": "x",
      "version": "1.0.0"
    },
    "node_modules/y": {
      "version": "1.0.0"
    }
  }
}
`
	os.WriteFile(filepath.Join(dir, "package.json"), []byte(pkg), 0o644)
	os.WriteFile(filepath.Join(dir, "package-lock.json"), []byte(lock), 0o644)
	if err := SetVersion(dir, "1.1.0"); err != nil {
		t.Fatal(err)
	}
	gotPkg, _ := os.ReadFile(filepath.Join(dir, "package.json"))
	if string(gotPkg) != strings.Replace(pkg, "1.0.0\",\n", "1.1.0\",\n", 1) {
		t.Fatalf("package.json:\n%s", gotPkg)
	}
	gotLock, _ := os.ReadFile(filepath.Join(dir, "package-lock.json"))
	if strings.Count(string(gotLock), "1.1.0") != 2 || !strings.Contains(string(gotLock), "\"node_modules/y\": {\n      \"version\": \"1.0.0\"") {
		t.Fatalf("lock:\n%s", gotLock)
	}
}

func TestNpmBuildURL(t *testing.T) {
	if got := NpmBuildURL("shared-js"); got != "https://devops.kevyn.com.br/buildConfiguration/IpAlpha_Core_SharedJs_Publish" {
		t.Fatalf("NpmBuildURL = %s", got)
	}
}

func TestBuildContextFollowsEachDockerfile(t *testing.T) {
	root := "/w"
	core, _ := catalog.Get("auth-webapp")
	if got := BuildContext(root, core, "/w/core/auth-webapp"); got != filepath.Join(root, "core") {
		t.Fatalf("core repo context = %s", got)
	}
	// Forms and Sign are core tools: their Dockerfiles build from core/ like every core image.
	for _, name := range []string{"forms-api", "forms-webapp", "sign-api", "sign-webapp"} {
		r, _ := catalog.Get(name)
		if got := BuildContext(root, r, "/w/core/"+name); got != filepath.Join(root, "core") || r.Rel() != "core/"+name {
			t.Fatalf("%s context = %s (rel %s)", name, got, r.Rel())
		}
	}
	app := catalog.Repo{Name: "ministry-api", Kind: catalog.KindAPI, App: "ministry"}
	if got := BuildContext(root, app, "/w/apps/ministry/ministry-api"); got != "/w/apps/ministry/ministry-api" {
		t.Fatalf("ministry app context = %s", got)
	}
	optional := catalog.Repo{Name: "acampa-kids-backend", Kind: catalog.KindOptional, App: "acampa-kids"}
	if got := BuildContext(root, optional, "/w/apps/acampa-kids/backend"); got != "/w/apps/acampa-kids/backend" {
		t.Fatalf("optional app context = %s", got)
	}
}

func TestRewriteImageTagsReplacesTagsAndDigests(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "deployment.yaml")
	in := "image: reg.example/ip-alpha/core/auth-webapp:0.6.0\n" +
		"other: reg.example/ip-alpha/core/auth-webapp:0.5.0@sha256:abc123\n" +
		"keep: reg.example/ip-alpha/core/auth-api:1.0.0\n"
	if err := os.WriteFile(file, []byte(in), 0o644); err != nil {
		t.Fatal(err)
	}
	changed := RewriteImageTags(dir, "reg.example/ip-alpha/core/auth-webapp:", "0.7.0")
	if len(changed) != 1 || changed[0] != "deployment.yaml" {
		t.Fatalf("changed = %v", changed)
	}
	out, _ := os.ReadFile(file)
	want := "image: reg.example/ip-alpha/core/auth-webapp:0.7.0\n" +
		"other: reg.example/ip-alpha/core/auth-webapp:0.7.0\n" +
		"keep: reg.example/ip-alpha/core/auth-api:1.0.0\n"
	if string(out) != want {
		t.Fatalf("got:\n%s", out)
	}
	if again := RewriteImageTags(dir, "reg.example/ip-alpha/core/auth-webapp:", "0.7.0"); len(again) != 0 {
		t.Fatalf("second run changed %v", again)
	}
}

func TestAcampaImagesArePublishableAndPinnedInTheApp(t *testing.T) {
	w := &workspace.Workspace{Settings: &workspace.Settings{Registry: catalog.DefaultRegistry}}
	for _, c := range [][3]string{
		{"acampa-kids-backend", "registry.kevyn.com.br/ip-alpha/apps/acampa-kids/backend", "IpAlpha_AcampaKids_Backend_Build"},
		{"acampa-kids-frontend", "registry.kevyn.com.br/ip-alpha/apps/acampa-kids/frontend", "IpAlpha_AcampaKids_Frontend_Build"},
	} {
		r, ok := catalog.Get(c[0])
		if !ok || !r.Publishable() || !r.HasImage() {
			t.Fatalf("%s must be publishable: %+v", c[0], r)
		}
		if got := ImageName(w, r); got != c[1] {
			t.Fatalf("image = %s", got)
		}
		if got := AppBuildURL(r); !strings.HasSuffix(got, "/"+c[2]) {
			t.Fatalf("build url = %s", got)
		}
	}
	for _, n := range []string{"services-photo-api", "services-photo-webapp", "services-kids-api", "services-kids-webapp"} {
		r, _ := catalog.Get(n)
		want := "registry.kevyn.com.br/ip-alpha/apps/" + r.App + "/" + n
		if got := ImageName(w, r); got != want {
			t.Fatalf("%s image = %s", n, got)
		}
		if got := BuildContext("/w", r, "/w/"+r.Rel()); got != "/w/"+r.Rel() {
			t.Fatalf("%s context = %s", n, got)
		}
	}
	core, _ := catalog.Get("auth-api")
	if got := ImageName(w, core); got != catalog.DefaultRegistry+"/auth-api" {
		t.Fatalf("core image = %s", got)
	}
	// face-service stays a TeamCity-built core service, never a ./publish target.
	if f, _ := catalog.Get("face-service"); f.Publishable() || f.Rel() != "core/face-service" {
		t.Fatalf("face-service: %+v", f)
	}
}

func gitRepo(t *testing.T, commits ...string) string {
	t.Helper()
	dir := t.TempDir()
	run := func(args ...string) {
		cmd := exec.Command("git", append([]string{"-C", dir, "-c", "user.name=t", "-c", "user.email=t@t", "-c", "commit.gpgsign=false"}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init", "-q", "-b", "master")
	for i, m := range commits {
		if err := os.WriteFile(filepath.Join(dir, "f.txt"), []byte(strings.Repeat("x", i+1)), 0o644); err != nil {
			t.Fatal(err)
		}
		run("add", "-A")
		run("commit", "-q", "-m", m)
	}
	return dir
}

func TestBaselineWithoutTagsUsesReleaseCommit(t *testing.T) {
	dir := gitRepo(t, "one", "chore: release 0.4.1", "feat: a", "fix: b")
	ref, v := baseline(dir)
	if ref == "" || v != "0.4.1" {
		t.Fatalf("baseline = %q %q", ref, v)
	}
	_ = os.WriteFile(filepath.Join(dir, "package.json"), []byte(`{"version":"0.4.1"}`), 0o644)
	if !Dirty(dir) {
		t.Fatal("commits after the release commit must be dirty (version still 0.4.1 in git history only)")
	}
}

func TestDirtyAtReleaseCommitWithoutTags(t *testing.T) {
	dir := gitRepo(t, "one", "chore: release 0.4.1")
	if Dirty(dir) {
		t.Fatal("HEAD is the release commit: nothing to publish")
	}
}

func TestBaselinePrefersTag(t *testing.T) {
	dir := gitRepo(t, "chore: release 0.1.0", "feat: a")
	if out, err := exec.Command("git", "-C", dir, "tag", "v0.2.0").CombinedOutput(); err != nil {
		t.Fatal(string(out))
	}
	gitRepoAdd(t, dir, "fix: b")
	if ref, v := baseline(dir); ref != "v0.2.0" || v != "0.2.0" {
		t.Fatalf("baseline = %q %q", ref, v)
	}
}

func gitRepoAdd(t *testing.T, dir, msg string) {
	t.Helper()
	_ = os.WriteFile(filepath.Join(dir, "g.txt"), []byte(msg), 0o644)
	for _, a := range [][]string{{"add", "-A"}, {"-c", "user.name=t", "-c", "user.email=t@t", "commit", "-q", "-m", msg}} {
		if out, err := exec.Command("git", append([]string{"-C", dir}, a...)...).CombinedOutput(); err != nil {
			t.Fatal(string(out))
		}
	}
}

func TestBreakingMarkerAndCapMajor(t *testing.T) {
	plain := gitRepo(t, "chore: release 0.5.1", "dispatch-api: acampa core sync", "feat: more")
	if got := BreakingMarker(plain); got != "" {
		t.Fatalf("no marker expected, got %q", got)
	}
	if got := CapMajor(plain, "major"); got != "minor" {
		t.Fatalf("major without marker must cap to minor, got %s", got)
	}
	if got := CapMajor(plain, "patch"); got != "patch" {
		t.Fatalf("patch unchanged, got %s", got)
	}
	bang := gitRepo(t, "chore: release 0.5.1", "feat(api)!: drop v1")
	if BreakingMarker(bang) == "" || CapMajor(bang, "major") != "major" {
		t.Fatal("feat!: must keep major")
	}
	footer := gitRepo(t, "chore: release 0.5.1")
	gitRepoAdd(t, footer, "refactor: x\n\nBREAKING CHANGE: gone")
	if BreakingMarker(footer) == "" {
		t.Fatal("BREAKING CHANGE footer must be detected")
	}
}

func TestVersionFileForGoRepos(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "VERSION"), []byte("0.5.0\n"), 0o644)
	if got := CurrentVersion(dir); got != "0.5.0" {
		t.Fatal(got)
	}
	if err := SetVersion(dir, "0.5.1"); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(filepath.Join(dir, "VERSION")); string(data) != "0.5.1\n" {
		t.Fatalf("%q", data)
	}
	if _, err := os.Stat(filepath.Join(dir, "package.json")); err == nil {
		t.Fatal("package.json created")
	}
}
