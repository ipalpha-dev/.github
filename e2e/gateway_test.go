package e2e

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/ipalpha-dev/tooling/internal/catalog"
	"github.com/ipalpha-dev/tooling/internal/envfile"
)

func TestGatewaySetupAndPull(t *testing.T) {
	e := newEnv(t)
	e.setup()
	envPath := filepath.Join(e.root, "core", "ai-api", ".env")
	e.must(e.root, "ai", "gateway", "--mode", "simulator")
	got := envfile.Read(envPath)
	if got["AI_API_KEY"] != catalog.CPAMCSimulatorClientKey || got["CPAMC_MANAGEMENT_KEY"] != catalog.CPAMCSimulatorManagementKey {
		t.Fatal("simulator setup did not supply dummy credentials")
	}
	if _, err := e.run(e.root, "ai", "gateway", "--mode", "real", "--url", "https://gateway.example/v1"); err == nil {
		t.Fatal("real setup without key must fail")
	}
	e.vars = append(e.vars, "CPAMC_SETUP_CLIENT_KEY=client-e2e-fixture", "CPAMC_SETUP_MANAGEMENT_KEY=management-e2e-fixture")
	output := e.must(e.root, "ai", "gateway", "--mode", "real", "--url", "https://gateway.example")
	if strings.Contains(output, "e2e-fixture") {
		t.Fatal("secret in command output")
	}
	got = envfile.Read(envPath)
	if got["AI_BASE_URL"] != "https://gateway.example/v1" || got["AI_API_KEY"] != "client-e2e-fixture" {
		t.Fatal("real gateway not saved")
	}
	s := envfile.Read(filepath.Join(e.root, ".ipalpha", "settings"))
	if s["ai_gateway"] != "real" || s["ai_cli"] != "none" {
		t.Fatal("coding CLI was changed or gateway mode not saved")
	}
	e.must(e.root, "pull")
	got = envfile.Read(envPath)
	if got["AI_API_KEY"] != "client-e2e-fixture" || got["CPAMC_MANAGEMENT_KEY"] != "management-e2e-fixture" {
		t.Fatal("pull replaced explicit credentials")
	}
	if _, err := e.run(e.root, "ai", "gateway", "--mode", "real", "--url", "http://insecure.example"); err == nil {
		t.Fatal("insecure gateway accepted")
	}
}
