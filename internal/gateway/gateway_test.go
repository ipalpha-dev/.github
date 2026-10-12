package gateway

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ipalpha-dev/tooling/internal/catalog"
	"github.com/ipalpha-dev/tooling/internal/envfile"
	"github.com/ipalpha-dev/tooling/internal/workspace"
)

func TestNormalizeURL(t *testing.T) {
	for input, want := range map[string]string{"https://gateway.example": "https://gateway.example/v1", "https://gateway.example/v1/": "https://gateway.example/v1", "http://127.0.0.1:8317": "http://127.0.0.1:8317/v1", "http://[::1]:8317/v1": "http://[::1]:8317/v1"} {
		got, err := NormalizeURL(input)
		if err != nil || got != want {
			t.Fatal(input, got, err)
		}
	}
	for _, input := range []string{"", "ftp://gateway.example", "http://gateway.example", "https://key@gateway.example", "https://gateway.example/prefix", "https://gateway.example?key=x", "https://gateway.example#key"} {
		if _, err := NormalizeURL(input); err == nil {
			t.Fatal("accepted", input)
		}
	}
}
func TestConfigure(t *testing.T) {
	root := t.TempDir()
	w := &workspace.Workspace{Root: root, Settings: workspace.NewSettings()}
	dir := w.Repo("ai-api")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, ".env")
	if err := os.WriteFile(path, []byte("MONGO_URI=keep-me\nOPENROUTER_API_KEY=old-value\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := Configure(w, "real", "https://gateway.example", "client-fixture", "management-fixture"); err != nil {
		t.Fatal(err)
	}
	got := envfile.Read(path)
	if got["AI_BASE_URL"] != "https://gateway.example/v1" || got["AI_API_KEY"] != "client-fixture" || got["MONGO_URI"] != "keep-me" || got["OPENROUTER_API_KEY"] != "old-value" {
		t.Fatal("gateway config or unrelated values changed")
	}
	stat, _ := os.Stat(path)
	if stat.Mode().Perm() != 0600 {
		t.Fatal("env must be private")
	}
	s, err := workspace.LoadSettings(w.SettingsPath())
	if err != nil || s.Gateway() != "real" {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(w.SettingsPath())
	if strings.Contains(string(data), "fixture") {
		t.Fatal("secret in settings")
	}
	if err := Configure(w, "simulator", "ignored", "ignored", "ignored"); err != nil {
		t.Fatal(err)
	}
	got = envfile.Read(path)
	if got["AI_API_KEY"] != catalog.CPAMCSimulatorClientKey || got["CPAMC_MANAGEMENT_KEY"] != catalog.CPAMCSimulatorManagementKey {
		t.Fatal("simulator not configured")
	}
	for _, keys := range [][2]string{{"", ""}, {catalog.CPAMCSimulatorClientKey, ""}, {"client", catalog.CPAMCSimulatorManagementKey}, {"same", "same"}, {"key\nEVIL=x", ""}} {
		if err := Configure(w, "real", "https://gateway.example", keys[0], keys[1]); err == nil {
			t.Fatal("accepted invalid credentials")
		}
	}
}
