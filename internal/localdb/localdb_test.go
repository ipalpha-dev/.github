package localdb

import (
	"os/exec"
	"path/filepath"
	"testing"
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
