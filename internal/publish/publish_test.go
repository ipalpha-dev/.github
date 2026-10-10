package publish

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
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
