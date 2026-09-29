package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDiscoverEnvDepsIgnoresWebAppsAndAllowLists(t *testing.T) {
	root := t.TempDir()
	write := func(rel, body string) {
		p := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("core/auth-api/.env", "PORT=3005\nPROJECTS_API_URL=http://localhost:3001\nAUTH_FRONTEND_ORIGINS=http://localhost:5100\nCORE_FRONTEND_ORIGINS=http://localhost:5001\n")
	write("core/projects-api/.env", "PORT=3001\nAUTH_API_URL=http://127.0.0.1:3005\n")
	specs := []projectSpec{
		{Name: "auth-api", Kind: "service", Path: "core/auth-api", Port: "3005"},
		{Name: "projects-api", Kind: "service", Path: "core/projects-api", Port: "3001"},
		{Name: "auth-webapp", Kind: "app", Path: "core/auth-webapp", Port: "5100"},
		{Name: "projects-api-web", Kind: "attached", Parent: "projects-api", Path: "core/projects-api/frontend", Port: "5001"},
	}
	deps := discoverEnvDeps(root, specs)
	if got := deps["auth-api"]; len(got) != 1 || got[0] != "projects-api" {
		t.Fatalf("auth-api deps = %v, want [projects-api] (web apps and *_ORIGINS must not count)", got)
	}
	if got := deps["projects-api"]; len(got) != 1 || got[0] != "auth-api" {
		t.Fatalf("projects-api deps = %v, want [auth-api]", got)
	}
	if _, ok := deps["auth-webapp"]; ok {
		t.Fatalf("auth-webapp has no env, must have no deps")
	}
}
