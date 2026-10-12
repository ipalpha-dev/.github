package localdb

import (
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/ipalpha-dev/tooling/internal/env"
)

func TestHashRoundTrip(t *testing.T) {
	h := HashSecret("local-secret-123")
	if !VerifySecret(h, "local-secret-123") || VerifySecret(h, "other") {
		t.Fatal("round trip failed")
	}
}

// A hash made by node-argon2 (auth-api's library) verifies here.
func TestVerifyNodeArgon2(t *testing.T) {
	const node = "$argon2id$v=19$m=65536,t=3,p=4$CgAnA+pMX4/vlIxer9W1mw$GHoFFrHDWZ/sK3zjo3RzWP3UO7QtesEoTjFJ1ebY3Cc"
	if !VerifySecret(node, "local-secret-123") {
		t.Fatal("node-argon2 hash not verified")
	}
}

// Our hash verifies in node-argon2 when the workspace has auth-api installed (skipped otherwise).
func TestNodeVerifiesOurHash(t *testing.T) {
	auth := filepath.Join("..", "..", "..", "core", "auth-api")
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("no node")
	}
	h := HashSecret("abc-123")
	c := exec.Command("node", "-e", `let a; try { a = require("argon2") } catch { process.exit(4) }
a.verify(process.argv[1], "abc-123").then(ok => process.exit(ok ? 0 : 3))`, h)
	c.Dir = auth
	out, err := c.CombinedOutput()
	if ee, ok := err.(*exec.ExitError); ok && ee.ExitCode() == 3 {
		t.Fatalf("node-argon2 rejected our hash %s", h)
	}
	if err != nil {
		t.Skipf("argon2 not available: %s", out)
	}
}

func TestLoopbackOnly(t *testing.T) {
	for uri, want := range map[string]bool{
		"mongodb://u:p@127.0.0.1:27017/auth?authSource=admin": true,
		"mongodb://localhost/auth":                            true,
		"mongodb://[::1]:27017/auth":                          true,
		"mongodb://db.example.org:27017/auth":                 false,
		"mongodb+srv://cluster.example.org/auth":              false,
		"mongodb://127.0.0.1,db.example.org/auth":             false,
	} {
		if IsLoopbackURI(uri) != want {
			t.Errorf("%s → %v", uri, !want)
		}
	}
}

func TestMergeKeepsNonLoopback(t *testing.T) {
	out, changed := merge([]string{"https://oikos.example.org", "http://localhost:5110"}, []string{"http://localhost:5112"})
	if !changed || len(out) != 2 || out[0] != "https://oikos.example.org" || out[1] != "http://localhost:5112" {
		t.Fatalf("%v %v", out, changed)
	}
	if _, changed := merge([]string{"http://localhost:5110"}, []string{"http://localhost:5110"}); changed {
		t.Fatal("same value reported as change")
	}
}

// Seeding is insert-only, so an existing local client must gain new scopes (e.g. documents-api's IPAlpha Sign
// grants) and an app-bound client its appId; a client that already matches is left alone.
func TestClientUpdateSyncsScopesAndApp(t *testing.T) {
	now := time.Now()
	hash := HashSecret("local-secret-123")
	seed := env.SeedClient{ClientID: "documents-api", Secret: "local-secret-123", ServiceID: "documents-api",
		Scopes: []string{"revocations:read", "persons:signing-fields", "persons:access-log", "projects:read"}}
	doc := clientDoc{SecretHash: hash, AllowedScopes: []string{"revocations:read"}}
	u := clientUpdate(doc, seed, now)
	set, _ := u["$set"].(bson.M)
	if u == nil || set == nil || u["$inc"] != nil || set["secretHash"] != nil {
		t.Fatalf("scope drift only: %v", u)
	}
	if got := set["allowedScopes"].([]string); len(got) != 4 || got[1] != "persons:signing-fields" {
		t.Fatalf("allowedScopes: %v", got)
	}
	if _, ok := set["appId"]; ok {
		t.Fatal("unbound client got an appId")
	}
	// Same scopes in another order, same secret: nothing to do.
	doc.AllowedScopes = []string{"projects:read", "persons:access-log", "revocations:read", "persons:signing-fields"}
	if u := clientUpdate(doc, seed, now); u != nil {
		t.Fatalf("matching client updated: %v", u)
	}
	// App-bound client: appId set; a drifted secret is re-hashed and its version bumped.
	bound := env.SeedClient{ClientID: "sign-api", Secret: "new-secret-456", ServiceID: "sign-api", AppID: "app-ipalpha-sign",
		Scopes: []string{"notifications:send-template", "dispatch:app-channel"}}
	u = clientUpdate(clientDoc{SecretHash: hash, AllowedScopes: bound.Scopes}, bound, now)
	set = u["$set"].(bson.M)
	if set["appId"] != "app-ipalpha-sign" || set["secretHash"] == nil || u["$inc"] == nil || set["allowedScopes"] != nil {
		t.Fatalf("bound client: %v", u)
	}
	if !VerifySecret(set["secretHash"].(string), "new-secret-456") {
		t.Fatal("new secret hash does not verify")
	}
	// Scopes removed from the seed are removed locally too (exact match), never nil.
	u = clientUpdate(clientDoc{SecretHash: hash, AllowedScopes: []string{"old:scope"}}, env.SeedClient{ClientID: "x", Secret: "local-secret-123"}, now)
	if got := u["$set"].(bson.M)["allowedScopes"].([]string); got == nil || len(got) != 0 {
		t.Fatalf("empty scopes: %#v", got)
	}
}
