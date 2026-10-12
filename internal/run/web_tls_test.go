package run

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ipalpha-dev/tooling/internal/catalog"
	"github.com/ipalpha-dev/tooling/internal/envfile"
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
