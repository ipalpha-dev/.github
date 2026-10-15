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

// The webapp proxies /sync only when the private sync is cloned; without it no INTEGRATIONS_URL is set.
func TestServicesKidsWebappIntegrationsURLOnlyWithTheSync(t *testing.T) {
	root := t.TempDir()
	for _, path := range []string{"core/auth-api", "apps/services-kids/services-kids-webapp", ".ipalpha"} {
		if err := os.MkdirAll(filepath.Join(root, path), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	w, err := workspace.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	repo, _ := catalog.Get("services-kids-webapp")
	plan := ports.Plan{Actual: map[string]int{"services-kids-sync-api": 3124}, Preferred: map[string]int{"services-kids-sync-api": 3024}}
	values, _ := webEnv(w, repo, plan, ports.NewRewriter(plan))
	if v, ok := envfile.Parse(joinLines(values))["INTEGRATIONS_URL"]; ok {
		t.Fatalf("no sync cloned, got INTEGRATIONS_URL=%q", v)
	}
	if err := os.MkdirAll(filepath.Join(root, "apps/services-kids/services-kids-sync-api"), 0o755); err != nil {
		t.Fatal(err)
	}
	values, _ = webEnv(w, repo, plan, ports.NewRewriter(plan))
	if got := envfile.Parse(joinLines(values))["INTEGRATIONS_URL"]; got != "http://127.0.0.1:3124" {
		t.Fatalf("INTEGRATIONS_URL = %q", got)
	}
}
