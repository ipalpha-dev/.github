package run

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/ipalpha-dev/tooling/internal/catalog"
	"github.com/ipalpha-dev/tooling/internal/envfile"
	"github.com/ipalpha-dev/tooling/internal/ports"
	"github.com/ipalpha-dev/tooling/internal/workspace"
)

func TestFormsLoginUsesAssignedPortsAndKeepsLocalOverrides(t *testing.T) {
	root := t.TempDir()
	for _, path := range []string{"core/auth-api", "apps/forms/forms-webapp", ".ipalpha"} {
		if err := os.MkdirAll(filepath.Join(root, path), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "core/auth-api/.env"), []byte("BUILTIN_FORMS_RESPONDENT_LOGIN_CLIENT_ID=custom-auth-client\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	w, err := workspace.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	repo, ok := catalog.Get("forms-webapp")
	if !ok {
		t.Fatal("Forms missing from catalog")
	}
	plan := ports.Plan{Actual: map[string]int{"forms-webapp": 6106, "auth-api": 6005, "projects-api": 6001}, Preferred: map[string]int{"forms-webapp": 5106, "auth-api": 3005, "projects-api": 3001}}
	values, _ := webEnv(w, repo, plan, ports.NewRewriter(plan))
	parsed := envfile.Parse(joinLines(values))
	if parsed["VITE_RESPONDENT_LOGIN_CLIENT_ID"] != "custom-auth-client" || parsed["VITE_RESPONDENT_LOGIN_ENTRY_POINT"] != "respondent-login" || parsed["VITE_RESPONDENT_LOGIN_CALLBACK_URI"] != "http://localhost:6106/respond/login/callback" || parsed["PROJECTS_API_URL"] != "http://127.0.0.1:6001" {
		t.Fatalf("login settings or assigned ports incorrect: client=%s callback=%s projects=%s", parsed["VITE_RESPONDENT_LOGIN_CLIENT_ID"], parsed["VITE_RESPONDENT_LOGIN_CALLBACK_URI"], parsed["PROJECTS_API_URL"])
	}
	if err := os.WriteFile(filepath.Join(repo.Path(root), ".env.local"), []byte("VITE_RESPONDENT_LOGIN_CLIENT_ID=frontend-override\nVITE_RESPONDENT_LOGIN_CALLBACK_URI=http://localhost:7106/respond/login/callback\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	values, _ = webEnv(w, repo, plan, ports.NewRewriter(plan))
	parsed = envfile.Parse(joinLines(values))
	if parsed["VITE_RESPONDENT_LOGIN_CLIENT_ID"] != "frontend-override" || parsed["VITE_RESPONDENT_LOGIN_CALLBACK_URI"] != "http://localhost:7106/respond/login/callback" {
		t.Fatal("local frontend overrides replaced")
	}
}

func TestOikosOptionalProviderRegistryUsesAssignedPortsAndKeepsOverrides(t *testing.T) {
	root := t.TempDir()
	for _, path := range []string{"core/auth-api", "core/oikos-webapp", ".ipalpha"} {
		if err := os.MkdirAll(filepath.Join(root, path), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	w, err := workspace.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	repo, _ := catalog.Get("oikos-webapp")
	plan := ports.Plan{Actual: map[string]int{"oikos-webapp": 6110, "forms-webapp": 6106, "forms-api": 6006}, Preferred: map[string]int{"oikos-webapp": 5110, "forms-webapp": 5106, "forms-api": 3006}}
	values, _ := webEnv(w, repo, plan, ports.NewRewriter(plan))
	parsed := envfile.Parse(joinLines(values))
	var providers []struct {
		WebBase, Scope, ApiBase string
		Label                   map[string]string
	}
	if err := json.Unmarshal([]byte(parsed["VITE_PROJECT_CONTENT_PROVIDERS_JSON"]), &providers); err != nil || len(providers) != 1 || providers[0].WebBase != "http://localhost:6106" || providers[0].ApiBase != "/api/forms" || providers[0].Scope != "project-content:read" || len(providers[0].Label) != 5 {
		t.Fatal("provider registration, language labels or assigned web port incorrect")
	}
	if parsed["FORMS_API_URL"] != "http://127.0.0.1:6006" {
		t.Fatal("provider API port not mapped")
	}
	if err := os.WriteFile(filepath.Join(repo.Path(root), ".env.local"), []byte("VITE_PROJECT_CONTENT_PROVIDERS_JSON=[]\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	values, _ = webEnv(w, repo, plan, ports.NewRewriter(plan))
	parsed = envfile.Parse(joinLines(values))
	if parsed["VITE_PROJECT_CONTENT_PROVIDERS_JSON"] != "[]" {
		t.Fatal("operator-disabled providers overridden")
	}
}

func joinLines(lines []string) string {
	out := ""
	for _, line := range lines {
		out += line + "\n"
	}
	return out
}
