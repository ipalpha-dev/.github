package catalog

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFaceServiceLivesInCore(t *testing.T) {
	r, ok := Get("face-service")
	if !ok || r.Kind != KindService || r.Rel() != "core/face-service" {
		t.Fatalf("face-service: %+v ok=%v rel=%q", r, ok, r.Rel())
	}
	if !r.SkippedWhenMissing() || r.Publishable() || r.Runs() || r.HasEnv() {
		t.Fatalf("face-service flags: %+v", r)
	}
	if _, ok := Get("acampa-kids-face-service"); ok {
		t.Fatal("the Acampa face-service repo is retired")
	}
	if FaceImage != "registry.kevyn.com.br/ip-alpha/core/face-service:0.1.0" {
		t.Fatalf("face image: %s", FaceImage)
	}
}

func TestDrivePortsAreUniqueAndCore(t *testing.T) {
	api, ok1 := Get("drive-api")
	web, ok2 := Get("drive-webapp")
	if !ok1 || !ok2 || api.Port != 3018 || web.Port != 5116 || api.Rel() != "core/drive-api" || web.Rel() != "core/drive-webapp" || WebBackend["drive-webapp"] != "drive-api" {
		t.Fatalf("drive: %+v %+v", api, web)
	}
	seen := map[int]string{}
	for _, r := range Repos {
		if r.Port == 0 {
			continue
		}
		if other, dup := seen[r.Port]; dup {
			t.Fatalf("port %d used by %s and %s", r.Port, other, r.Name)
		}
		seen[r.Port] = r.Name
	}
}

func TestVirtualAssistantIsACoreAPI(t *testing.T) {
	r, ok := Get("virtual-assistant")
	if !ok || r.Kind != KindAPI || r.Port != 3019 || r.Rel() != "core/virtual-assistant" || r.SkippedWhenMissing() {
		t.Fatalf("virtual-assistant: %+v", r)
	}
}

func TestDesktopReposAreCloneOnly(t *testing.T) {
	for name, rel := range map[string]string{
		"services-photo-macos":          "apps/services-photo/services-photo-macos",
		"services-photo-windows":        "apps/services-photo/services-photo-windows",
		"services-kids-printer-windows": "apps/services-kids/services-kids-printer-windows",
	} {
		r, ok := Get(name)
		if !ok || r.Kind != KindDesktop || r.Rel() != rel || r.Port != 0 {
			t.Fatalf("%s: %+v", name, r)
		}
		if r.Runs() || r.HasEnv() || r.HasImage() || r.Publishable() || !r.SkippedWhenMissing() {
			t.Fatalf("%s must be clone-only: %+v", name, r)
		}
	}
}

func TestAcampaKidsRunsAsABunBackendAndAViteFrontend(t *testing.T) {
	api, ok1 := Get("acampa-kids-backend")
	web, ok2 := Get("acampa-kids-frontend")
	if !ok1 || !ok2 || api.Runtime != "bun" || !api.Runs() || !api.IsBackend() || !api.HasEnv() || api.Label() != "Acampa Kids API" ||
		!web.Runs() || !web.IsWeb() || web.HasEnv() || web.Label() != "Acampa Kids Web" ||
		api.Rel() != "apps/acampa-kids/backend" || web.Rel() != "apps/acampa-kids/frontend" {
		t.Fatalf("acampa: %+v %+v", api, web)
	}
	// Still published (images) and skipped quietly when the developer cannot read them.
	if !api.IsAppImage() || !web.IsAppImage() || !api.SkippedWhenMissing() || !api.Publishable() {
		t.Fatalf("acampa image flags: %+v %+v", api, web)
	}
	keys := PortKeys()
	has := func(k string) bool {
		for _, v := range keys {
			if v == k {
				return true
			}
		}
		return false
	}
	if !has("acampa-kids-backend") || !has("acampa-kids-frontend") {
		t.Fatalf("port keys: %v", keys)
	}
	for _, w := range Webs() {
		if w.Name == "acampa-kids-frontend" {
			return
		}
	}
	t.Fatal("Webs() lacks the Acampa frontend")
}

func TestVirtualAssistantGoDetectionFollowsTheCheckout(t *testing.T) {
	r, _ := Get("virtual-assistant")
	if r.Lang != "go" || r.Runtime != "" || !r.HasEnv() {
		t.Fatalf("virtual-assistant: %+v", r)
	}
	dir := t.TempDir()
	if r.IsGo(dir) {
		t.Fatal("empty checkout is not Go")
	}
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module x\n"), 0o644)
	if !r.IsGo(dir) {
		t.Fatal("go.mod without package.json is Go")
	}
	os.WriteFile(filepath.Join(dir, "package.json"), []byte("{}"), 0o644)
	if r.IsGo(dir) {
		t.Fatal("package.json (Node master) must win")
	}
	other, _ := Get("persons-api")
	os.Remove(filepath.Join(dir, "package.json"))
	if other.IsGo(dir) {
		t.Fatal("repos without Lang are never Go")
	}
}

func TestCostsAPIIsACoreAPIOnPort3021(t *testing.T) {
	r, ok := Get("costs-api")
	if !ok || r.Port != 3021 || r.Kind != KindAPI || r.Rel() != "core/costs-api" {
		t.Fatalf("costs-api catalog entry: %+v ok=%v", r, ok)
	}
}
