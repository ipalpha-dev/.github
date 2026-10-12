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
