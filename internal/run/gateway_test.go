package run

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ipalpha-dev/tooling/internal/catalog"
	"github.com/ipalpha-dev/tooling/internal/envfile"
	"github.com/ipalpha-dev/tooling/internal/infra"
	"github.com/ipalpha-dev/tooling/internal/ports"
	"github.com/ipalpha-dev/tooling/internal/workspace"
)

func TestGatewayRuntimeOverlay(t *testing.T) {
	root := t.TempDir()
	w := &workspace.Workspace{Root: root, Settings: workspace.NewSettings()}
	repo, _ := catalog.Get("ai-api")
	if err := os.MkdirAll(repo.Path(root), 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(repo.Path(root), ".env")
	original := "AI_BASE_URL=https://real-gateway.example/v1\nAI_API_KEY=real-client-fixture\nCPAMC_MANAGEMENT_KEY=real-management-fixture\nMONGO_URI=mongodb://127.0.0.1:27017/ai\n"
	if err := os.WriteFile(path, []byte(original), 0600); err != nil {
		t.Fatal(err)
	}
	plan := ports.Plan{Actual: map[string]int{"cpamc": 8420, "ai-api": 3108, "mongo": 27117}}
	rw := ports.NewRewriter(plan)
	env, refs := apiEnv(w, repo, plan, rw)
	values := envfile.Parse(strings.Join(env, "\n"))
	if values["AI_BASE_URL"] != "http://127.0.0.1:8420/v1" || values["AI_API_KEY"] != catalog.CPAMCSimulatorClientKey || values["CPAMC_MANAGEMENT_KEY"] != catalog.CPAMCSimulatorManagementKey || !refs[8420] {
		t.Fatal("simulator overlay did not use the resolved port and dummy keys")
	}
	for _, value := range env {
		if strings.Contains(value, "real-client-fixture") || strings.Contains(value, "real-management-fixture") {
			t.Fatal("real key sent to simulator")
		}
	}
	w.Settings.GatewayMode = "real"
	env, _ = apiEnv(w, repo, plan, rw)
	values = envfile.Parse(strings.Join(env, "\n"))
	if values["AI_BASE_URL"] != "https://real-gateway.example/v1" || values["AI_API_KEY"] != "real-client-fixture" {
		t.Fatal("real settings changed")
	}
	data, _ := os.ReadFile(path)
	if string(data) != original {
		t.Fatal("runtime changed env file")
	}
	specs := buildSpecs(w, &infra.Engine{W: w, Runtime: infra.Docker, Name: "test"}, plan, rw)
	found := false
	for _, sp := range specs {
		if sp.ID == "cpamc" {
			found = true
			if sp.ReadyURL != "http://127.0.0.1:8420/ready" || sp.Kind != KindInfra {
				t.Fatal("missing simulator health spec")
			}
		}
	}
	if !found {
		t.Fatal("simulator not in run panel")
	}
}

func TestGoCoreServiceSpec(t *testing.T) {
	root := t.TempDir()
	w := &workspace.Workspace{Root: root, Settings: workspace.NewSettings()}
	repo, _ := catalog.Get("virtual-assistant")
	os.MkdirAll(repo.Path(root), 0o700)
	os.WriteFile(filepath.Join(repo.Path(root), "go.mod"), []byte("module x\n"), 0o600)
	plan := ports.Plan{Actual: map[string]int{"virtual-assistant": 3019, "mongo": 27017}}
	find := func() *Spec {
		for _, sp := range buildSpecs(w, &infra.Engine{W: w, Runtime: infra.Docker, Name: "test"}, plan, ports.NewRewriter(plan)) {
			if sp.ID == "virtual-assistant" {
				return sp
			}
		}
		t.Fatal("no spec")
		return nil
	}
	sp := find()
	if sp.Command != "go" || strings.Join(sp.Args, " ") != "run ./cmd/virtual-assistant" || sp.ReadyURL != "http://127.0.0.1:3019/ready" {
		t.Fatalf("%+v", sp)
	}
	for _, e := range sp.Env {
		if strings.HasPrefix(e, "NODE_") {
			t.Fatal("node env on a Go service:", e)
		}
	}
	// Node master (package.json present) keeps the npm command.
	os.WriteFile(filepath.Join(repo.Path(root), "package.json"), []byte(`{"scripts":{"start":"x"}}`), 0o600)
	if sp := find(); sp.Command == "go" || sp.Missing != "" {
		t.Fatalf("node checkout: %+v", sp)
	}
	// Neither go.mod nor package.json: reported missing.
	os.Remove(filepath.Join(repo.Path(root), "package.json"))
	os.Remove(filepath.Join(repo.Path(root), "go.mod"))
	if sp := find(); sp.Missing == "" || sp.Autostart {
		t.Fatalf("empty checkout: %+v", sp)
	}
}
