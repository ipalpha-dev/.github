package env

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/ipalpha-dev/tooling/internal/envfile"
	"github.com/ipalpha-dev/tooling/internal/workspace"
)

func ws(t *testing.T) *workspace.Workspace {
	t.Helper()
	root := t.TempDir()
	for _, d := range []string{"core/auth-api", "core/persons-api", "core/notifications-api", "apps/forms/forms-api", ".ipalpha"} {
		if err := os.MkdirAll(filepath.Join(root, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	os.WriteFile(filepath.Join(root, ".ipalpha", "settings"), []byte("lang=en-US\n"), 0o600)
	w, err := workspace.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	return w
}

func TestPrepareKeepsDeveloperValuesAndFillsLocalOnes(t *testing.T) {
	w := ws(t)
	persons := filepath.Join(w.Repo("persons-api"), ".env")
	os.WriteFile(persons, []byte("AUTH_TOKEN_ISSUER=\"\"\nIMPORT_ROWS_KEY=\nAUTH_CLIENT_SECRET=operator-secret-long-enough\nCUSTOM_VALUE=keep-me\nAI_API_URL=http://localhost:3010\n"), 0o644)
	notif := filepath.Join(w.Repo("notifications-api"), ".env")
	os.WriteFile(notif, []byte("MAIL_PROVIDER=sendgrid\nSMS_PROVIDER=smsbarato\nSENDGRID_API_KEY=operator\nDEPLOYMENT_ENVIRONMENT=production\n"), 0o644)
	if _, err := Prepare(w); err != nil {
		t.Fatal(err)
	}
	p := envfile.Read(persons)
	if p["AUTH_TOKEN_ISSUER"] != "http://localhost:3005" || p["CUSTOM_VALUE"] != "keep-me" || p["AUTH_CLIENT_SECRET"] != "operator-secret-long-enough" {
		t.Fatalf("persons: %v", p)
	}
	if p["AI_API_URL"] != "http://127.0.0.1:3008" || p["OIKOS_APP_ID"] != "app-oikos" || p["INSTANCE_ID"] == "" {
		t.Fatalf("persons local values: %v", p)
	}
	key := p["IMPORT_ROWS_KEY"]
	if len(key) < 40 {
		t.Fatal("IMPORT_ROWS_KEY not generated")
	}
	n := envfile.Read(notif)
	if n["MAIL_PROVIDER"] != "mailpit" || n["SMS_PROVIDER"] != "mailpit" || n["DEPLOYMENT_ENVIRONMENT"] != "development" || n["SENDGRID_API_KEY"] != "operator" {
		t.Fatalf("notifications must always capture locally: %v", n)
	}
	if st, _ := os.Stat(persons); st.Mode().Perm() != 0o600 && os.PathSeparator == '/' {
		t.Fatalf("mode %v", st.Mode())
	}
	// Idempotent: a second run keeps generated secrets.
	if _, err := Prepare(w); err != nil {
		t.Fatal(err)
	}
	if envfile.Read(persons)["IMPORT_ROWS_KEY"] != key {
		t.Fatal("secret regenerated")
	}
	// Seed clients: one per API with approved scopes, operator clients kept.
	auth := envfile.Read(filepath.Join(w.Repo("auth-api"), ".env"))
	var clients []SeedClient
	if err := json.Unmarshal([]byte(auth["SEED_CLIENTS_JSON"]), &clients); err != nil {
		t.Fatal(err)
	}
	byID := map[string]SeedClient{}
	for _, c := range clients {
		byID[c.ClientID] = c
	}
	if byID["persons-api"].Secret != "operator-secret-long-enough" || len(byID["persons-api"].Scopes) == 0 {
		t.Fatalf("persons client: %+v", byID["persons-api"])
	}
	if len(byID["forms-api"].Scopes) == 0 {
		t.Fatal("forms-api (apps/forms) client missing")
	}
}

func TestOperatorClientsKept(t *testing.T) {
	w := ws(t)
	auth := filepath.Join(w.Repo("auth-api"), ".env")
	os.WriteFile(auth, []byte(`SEED_CLIENTS_JSON='[{"clientId":"other","secret":"x","serviceId":"other","scopes":["apps:read"]},{"clientId":"persons-api","secret":"old","serviceId":"persons-api","scopes":["extra:scope"]}]'`+"\n"), 0o600)
	os.WriteFile(filepath.Join(w.Repo("persons-api"), ".env"), []byte("AUTH_CLIENT_ID=persons-api\nAUTH_CLIENT_SECRET=new-secret-value\n"), 0o600)
	if err := SeedClients(w); err != nil {
		t.Fatal(err)
	}
	var list []map[string]any
	json.Unmarshal([]byte(envfile.Read(auth)["SEED_CLIENTS_JSON"]), &list)
	found := map[string]map[string]any{}
	for _, c := range list {
		found[c["clientId"].(string)] = c
	}
	if found["other"] == nil {
		t.Fatal("operator client dropped")
	}
	scopes := found["persons-api"]["scopes"].([]any)
	if scopes[0] != "extra:scope" || found["persons-api"]["secret"] != "new-secret-value" {
		t.Fatalf("persons client not merged: %v", found["persons-api"])
	}
}

func TestPhoneAndName(t *testing.T) {
	for _, in := range []string{"11987654321", "(11) 98765-4321", "+55 11 98765-4321", "011987654321"} {
		if p, err := NormalizePhone(in); err != nil || p != "+5511987654321" {
			t.Errorf("%q → %q %v", in, p, err)
		}
	}
	for _, bad := range []string{"", "987654321", "00987654321", "1188765432", "+1 11987654321"} {
		if _, err := NormalizePhone(bad); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
	if n, _ := NormalizeName("  Joao  Silva  "); n != "Joao Silva" {
		t.Fatal(n)
	}
	if _, err := NormalizeName("a\nSUPERUSER_PHONE=x"); err == nil {
		t.Fatal("control chars accepted")
	}
}

func TestSuperuserQuoted(t *testing.T) {
	w := ws(t)
	os.WriteFile(filepath.Join(w.Repo("auth-api"), ".env"), []byte("SUPERUSER_NAME=\nAUTH_CLIENT_SECRET=keep\n"), 0o600)
	if err := WriteSuperuser(w, "Joao $USER O'Neal", "+5511987654321"); err != nil {
		t.Fatal(err)
	}
	n, p := Superuser(w)
	if n != "Joao $USER O'Neal" || p != "+5511987654321" {
		t.Fatalf("%q %q", n, p)
	}
}
