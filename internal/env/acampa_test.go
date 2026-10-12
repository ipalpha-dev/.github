package env

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ipalpha-dev/tooling/internal/catalog"
	"github.com/ipalpha-dev/tooling/internal/envfile"
)

func TestAcampaBackendEnvUsesTheRunMongoAndKeepsDeveloperValues(t *testing.T) {
	w := ws(t)
	r, ok := catalog.Get("acampa-kids-backend")
	if !ok || r.Runtime != "bun" {
		t.Fatal("acampa-kids-backend must be a bun ./run process")
	}
	dir := r.Path(w.Root)
	os.MkdirAll(dir, 0o755)
	example := "PORT=3000\nCORS_ORIGIN=http://localhost:5173\nMONGODB_URI=mongodb://camping:camping-local-only@localhost:27019/camping?authSource=admin\nMONGODB_DB=camping\nSESSION_TOKEN_KEY=\nIPALPHA_PROJECT_ID=\nAI_API_URL=https://own.example.test/api/ai\n"
	os.WriteFile(filepath.Join(dir, ".env.example"), []byte(example), 0o644)

	if _, err := Prepare(w); err != nil {
		t.Fatal(err)
	}
	got := envfile.Read(filepath.Join(dir, ".env"))
	if got["PORT"] != "3020" || got["CORS_ORIGIN"] != "http://localhost:5117" {
		t.Fatalf("example standalone ports not converged: %v", got)
	}
	if got["MONGODB_URI"] != "mongodb://ipalpha:ipalpha@127.0.0.1:27017/camping?authSource=admin" || got["MONGODB_DB"] != "camping" {
		t.Fatalf("not the ./run Mongo: %q", got["MONGODB_URI"])
	}
	if len(got["SESSION_TOKEN_KEY"]) != 64 || got["WORKER_SECRET"] == "" {
		t.Fatalf("app-owned secrets not generated: %v", got)
	}
	if got["IPALPHA_PERSONS_API_URL"] != "http://127.0.0.1:3002" || got["IPALPHA_AUTH_ORIGIN"] != "http://localhost:5100" || got["IPALPHA_ENTRY_POINT"] != "login" {
		t.Fatalf("core URLs missing: %v", got)
	}
	if got["AI_API_URL"] != "https://own.example.test/api/ai" {
		t.Fatalf("developer value overwritten: %q", got["AI_API_URL"])
	}
	// Registration results are never invented: they stay blank until pasted.
	if got["IPALPHA_PROJECT_ID"] != "" || got["IPALPHA_CLIENT_ID"] != "" {
		t.Fatalf("registration ids must stay blank: %v", got)
	}

	// A second pass keeps the generated secrets and a developer's own change.
	key := got["SESSION_TOKEN_KEY"]
	f, _ := envfile.Load(filepath.Join(dir, ".env"))
	f.Set("PORT", "3999")
	f.Save()
	if _, err := Prepare(w); err != nil {
		t.Fatal(err)
	}
	again := envfile.Read(filepath.Join(dir, ".env"))
	if again["SESSION_TOKEN_KEY"] != key || again["PORT"] != "3999" {
		t.Fatalf("second pass changed values: %v", again)
	}
}

func TestAcampaBackendIsNotSeededIntoAuthWithoutAnApp(t *testing.T) {
	w := ws(t)
	r, _ := catalog.Get("acampa-kids-backend")
	os.MkdirAll(r.Path(w.Root), 0o755)
	os.WriteFile(filepath.Join(w.Repo("auth-api"), ".env"), []byte("SEED_CLIENTS_JSON=[]\n"), 0o600)
	if _, err := Prepare(w); err != nil {
		t.Fatal(err)
	}
	// auth-api refuses to boot on a client bound to an unknown app: Acampa must not be listed.
	if v := envfile.Read(filepath.Join(w.Repo("auth-api"), ".env"))["SEED_CLIENTS_JSON"]; strings.Contains(v, "acampa") {
		t.Fatalf("Acampa leaked into SEED_CLIENTS_JSON: %s", v)
	}
}
