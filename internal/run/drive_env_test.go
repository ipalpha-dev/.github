package run

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ipalpha-dev/tooling/internal/catalog"
	"github.com/ipalpha-dev/tooling/internal/envfile"
	"github.com/ipalpha-dev/tooling/internal/ports"
	"github.com/ipalpha-dev/tooling/internal/workspace"
)

func TestDriveWebappSignInEnvUsesAssignedPortAndKeepsOverrides(t *testing.T) {
	root := t.TempDir()
	for _, path := range []string{"core/auth-api", "core/drive-webapp", ".ipalpha"} {
		if err := os.MkdirAll(filepath.Join(root, path), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	w, err := workspace.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	repo, _ := catalog.Get("drive-webapp")
	plan := ports.Plan{Actual: map[string]int{"drive-webapp": 5216}, Preferred: map[string]int{"drive-webapp": 5116}}
	values, _ := webEnv(w, repo, plan, ports.NewRewriter(plan))
	p := envfile.Parse(joinLines(values))
	if p["VITE_DRIVE_CLIENT_ID"] != "drive-web" || p["VITE_DRIVE_ENTRY_POINT"] != "church" || p["VITE_DRIVE_CALLBACK_URI"] != "http://localhost:5216/auth/callback" {
		t.Fatalf("drive env: %v", p)
	}
	if err := os.WriteFile(filepath.Join(repo.Path(root), ".env.local"), []byte("VITE_DRIVE_CLIENT_ID=mine\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	values, _ = webEnv(w, repo, plan, ports.NewRewriter(plan))
	if envfile.Parse(joinLines(values))["VITE_DRIVE_CLIENT_ID"] != "mine" {
		t.Fatal("local override replaced")
	}
}
