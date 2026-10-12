package env

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ipalpha-dev/tooling/internal/catalog"
	"github.com/ipalpha-dev/tooling/internal/envfile"
)

func TestGatewayDefaultAndExistingValues(t *testing.T) {
	w := ws(t)
	r, _ := catalog.Get("ai-api")
	if err := os.MkdirAll(r.Path(w.Root), 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := Complete(w, r); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(r.Path(w.Root), ".env")
	values := envfile.Read(path)
	if values["AI_BASE_URL"] != "http://127.0.0.1:8317/v1" || values["AI_API_KEY"] != catalog.CPAMCSimulatorClientKey || values["CPAMC_MANAGEMENT_KEY"] != catalog.CPAMCSimulatorManagementKey {
		t.Fatal("fresh setup must be secret-free")
	}
	if _, ok := values["OPENROUTER_API_KEY"]; ok {
		t.Fatal("fresh setup must not require OpenRouter")
	}
	original := "AI_BASE_URL=https://custom.example/v1\nAI_API_KEY=existing-fixture\nCPAMC_MANAGEMENT_KEY=\nCUSTOM=keep\n"
	if err := os.WriteFile(path, []byte(original), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Complete(w, r); err != nil {
		t.Fatal(err)
	}
	values = envfile.Read(path)
	if values["AI_BASE_URL"] != "https://custom.example/v1" || values["AI_API_KEY"] != "existing-fixture" || values["CPAMC_MANAGEMENT_KEY"] != "" || values["CUSTOM"] != "keep" {
		t.Fatal("setup replaced custom gateway or inserted dummy key into real configuration")
	}
}
