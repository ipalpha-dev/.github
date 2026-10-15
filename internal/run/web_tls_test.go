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

func TestWebEnvPreservesOptionalLocalHTTPSAndNormalDefaults(t *testing.T) {
	for _, tls := range []bool{false, true} {
		w := &workspace.Workspace{Root: t.TempDir(), Settings: workspace.NewSettings()}
		r, _ := catalog.Get("auth-webapp")
		if err := os.MkdirAll(r.Path(w.Root), 0o755); err != nil {
			t.Fatal(err)
		}
		if tls {
			contents := "IPALPHA_WEB_TLS_CERT=/local/private/certificate.crt\nIPALPHA_WEB_TLS_KEY=/local/private/key.pem\nIPALPHA_WEB_ORIGIN=https://auth.ipalpha.test:5100\n"
			if err := os.WriteFile(filepath.Join(r.Path(w.Root), ".env"), []byte(contents), 0o600); err != nil {
				t.Fatal(err)
			}
		}
		plan := ports.Plan{Actual: catalog.DefaultPorts(), Preferred: catalog.DefaultPorts()}
		values, _ := webEnv(w, r, plan, ports.NewRewriter(plan))
		got := envfile.Parse(strings.Join(values, "\n"))
		if tls {
			for key, want := range map[string]string{"IPALPHA_WEB_TLS_CERT": "/local/private/certificate.crt", "IPALPHA_WEB_TLS_KEY": "/local/private/key.pem", "IPALPHA_WEB_ORIGIN": "https://auth.ipalpha.test:5100", "IPALPHA_WEB_URL": "https://auth.ipalpha.test:5100"} {
				if got[key] != want {
					t.Errorf("%s = %q; want %q", key, got[key], want)
				}
			}
		} else if got["IPALPHA_WEB_URL"] != "http://localhost:5100" || got["IPALPHA_WEB_TLS_KEY"] != "" {
			t.Errorf("normal HTTP defaults changed: %v", got)
		}
		if got["AUTH_API_URL"] != "http://127.0.0.1:3005" {
			t.Errorf("API proxy changed: %q", got["AUTH_API_URL"])
		}
	}
}

func TestAcampaFrontendProxiesToThisRunsBackendAndBackendRunsWithBun(t *testing.T) {
	w := &workspace.Workspace{Root: t.TempDir(), Settings: workspace.NewSettings()}
	web, _ := catalog.Get("acampa-kids-frontend")
	plan := ports.Plan{Actual: catalog.DefaultPorts(), Preferred: catalog.DefaultPorts()}
	plan.Actual["acampa-kids-backend"] = 3999
	values, refs := webEnv(w, web, plan, ports.NewRewriter(plan))
	got := envfile.Parse(strings.Join(values, "\n"))
	if got["IPALPHA_WEB_PROXY_API"] != "http://127.0.0.1:3999" || got["IPALPHA_WEB_PORT"] != "5117" || !refs[3999] {
		t.Fatalf("frontend env: %v", got)
	}

	api, _ := catalog.Get("acampa-kids-backend")
	dir := api.Path(w.Root)
	os.MkdirAll(dir, 0o755)
	os.WriteFile(filepath.Join(dir, ".env"), []byte("PORT=3000\nCORS_ORIGIN=http://localhost:5117\nMONGODB_URI=mongodb://ipalpha:ipalpha@127.0.0.1:27017/camping\n"), 0o600)
	plan.Actual["mongo"] = 28000
	plan.Actual["acampa-kids-frontend"] = 5999
	plan.Preferred = catalog.DefaultPorts()
	env, _ := apiEnv(w, api, plan, ports.NewRewriter(plan))
	vals := envfile.Parse(strings.Join(env, "\n"))
	if vals["PORT"] != "3999" || vals["CORS_ORIGIN"] != "http://localhost:5999" || !strings.Contains(vals["MONGODB_URI"], ":28000/") {
		t.Fatalf("backend env follows the run's ports: %v", vals)
	}
	var backend, frontend *Spec
	for _, sp := range buildSpecs(w, infra.New(w), plan, ports.NewRewriter(plan)) {
		switch sp.ID {
		case "acampa-kids-backend":
			backend = sp
		case "acampa-kids-frontend":
			frontend = sp
		}
	}
	if backend == nil || frontend == nil {
		t.Fatal("./run must list both Acampa Kids processes")
	}
	if backend.Kind != KindAPI || backend.Command != "bun" || strings.Join(backend.Args, " ") != "run dev" || backend.Group != "acampa-kids" ||
		backend.Port != 3999 || backend.ReadyURL != "http://127.0.0.1:3999/ready" || backend.Name != "Acampa Kids API" {
		t.Fatalf("backend spec: %+v", backend)
	}
	if frontend.Kind != KindWeb || frontend.Command != "node" || frontend.Group != "acampa-kids" || frontend.Port != 5999 || frontend.Name != "Acampa Kids Web" {
		t.Fatalf("frontend spec: %+v", frontend)
	}
	// Not cloned here: both rows explain why instead of starting.
	if backend.Missing == "" && !exists(filepath.Join(dir, "package.json")) {
		t.Fatal("a backend without package.json must say why it cannot start")
	}
}
