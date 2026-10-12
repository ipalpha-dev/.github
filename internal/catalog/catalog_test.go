package catalog

import "testing"

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
