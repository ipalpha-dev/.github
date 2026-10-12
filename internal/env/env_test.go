package env

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ipalpha-dev/tooling/internal/catalog"
	"github.com/ipalpha-dev/tooling/internal/envfile"
	"github.com/ipalpha-dev/tooling/internal/workspace"
)

func ws(t *testing.T) *workspace.Workspace {
	t.Helper()
	root := t.TempDir()
	for _, d := range []string{"core/auth-api", "core/persons-api", "core/notifications-api", "core/forms-api", ".ipalpha"} {
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

func TestFedcmProviderOriginIsAddedWithoutReplacingDeveloperConfiguration(t *testing.T) {
	w := ws(t)
	r, _ := catalog.Get("auth-api")
	path := filepath.Join(w.Repo("auth-api"), ".env")
	if err := os.WriteFile(path, []byte("FEDCM_PROVIDER_ORIGIN=https://auth.ipalpha.test:5100\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Complete(w, r); err != nil {
		t.Fatal(err)
	}
	if got := envfile.Read(path)["FEDCM_PROVIDER_ORIGIN"]; got != "https://auth.ipalpha.test:5100" {
		t.Fatalf("developer origin overwritten: %q", got)
	}
	if err := os.WriteFile(path, []byte("CUSTOM_KEY=keep\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Complete(w, r); err != nil {
		t.Fatal(err)
	}
	values := envfile.Read(path)
	if value, exists := values["FEDCM_PROVIDER_ORIGIN"]; !exists || value != "" || values["CUSTOM_KEY"] != "keep" {
		t.Fatalf("missing disabled default or lost developer values: %v", values)
	}
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
	if strings.Join(byID["forms-api"].Scopes, ",") != "persons:read" {
		t.Fatalf("forms-api (core/forms-api) client: %+v", byID["forms-api"])
	}
	// Forms respondent: forms-api holds auth's confidential client id, secret and callback.
	f := envfile.Read(filepath.Join(w.Repo("forms-api"), ".env"))
	if f["FORMS_RESPONDENT_CLIENT_ID"] != auth["BUILTIN_FORMS_RESPONDENT_CLIENT_ID"] || f["FORMS_RESPONDENT_CLIENT_SECRET"] != auth["BUILTIN_FORMS_RESPONDENT_CLIENT_SECRET"] ||
		len(f["FORMS_RESPONDENT_CLIENT_SECRET"]) < 16 || f["FORMS_RESPONDENT_REDIRECT_URI"] != "http://localhost:5106/respond/callback" ||
		f["RABBITMQ_URL"] == "" || f["EXCHANGE_FORMS_RESPONSE_CREATED"] != "forms-api.response.created" || f["EXCHANGE_FORMS_PERSON_REQUESTED"] != "forms-api.person.requested" {
		t.Fatalf("forms-api respondent env: %v", f)
	}
	if auth["EXCHANGE_PERSON_DELETED"] != "persons-api.person.deleted" {
		t.Fatalf("auth-api env: %v", auth)
	}
}

// Forms and Sign became core tools: scopes system-clients.json dropped are removed from an existing
// SEED_CLIENTS_JSON (only those), and the old sign-api app binding disappears.
func TestRetiredScopesAndAppBindingDropped(t *testing.T) {
	w := ws(t)
	os.MkdirAll(filepath.Join(w.Root, "core", "sign-api"), 0o755)
	auth := filepath.Join(w.Repo("auth-api"), ".env")
	os.WriteFile(auth, []byte(`SEED_CLIENTS_JSON='[{"clientId":"forms-api","secret":"s","serviceId":"forms-api","scopes":["revocations:read","prefill:redeem","prefill:read","persons:read","persons:write","dispatch:events","extra:keep"]},{"clientId":"sign-api","secret":"s","serviceId":"sign-api","appId":"app-ipalpha-sign","scopes":["notifications:send-template","dispatch:app-channel"]}]'`+"\n"), 0o600)
	os.WriteFile(filepath.Join(w.Repo("forms-api"), ".env"), []byte("AUTH_CLIENT_ID=forms-api\nAUTH_CLIENT_SECRET=forms-secret-long\n"), 0o600)
	os.WriteFile(filepath.Join(w.Repo("sign-api"), ".env"), []byte("AUTH_CLIENT_ID=sign-api\nAUTH_CLIENT_SECRET=sign-secret-long\n"), 0o600)
	if err := SeedClients(w); err != nil {
		t.Fatal(err)
	}
	raw := envfile.Read(auth)["SEED_CLIENTS_JSON"]
	var clients []SeedClient
	json.Unmarshal([]byte(raw), &clients)
	byID := map[string]SeedClient{}
	for _, c := range clients {
		byID[c.ClientID] = c
	}
	if got := strings.Join(byID["forms-api"].Scopes, ","); got != "persons:read,extra:keep" {
		t.Fatalf("forms-api scopes: %s", got)
	}
	if got := strings.Join(byID["sign-api"].Scopes, ","); got != "notifications:send-template,revocations:read" || byID["sign-api"].AppID != "" || strings.Contains(raw, "app-ipalpha-sign") {
		t.Fatalf("sign-api client: %+v", byID["sign-api"])
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

func TestPhotosAPIGetsStoreKeysAndFaceOnlyWhenOn(t *testing.T) {
	w := ws(t)
	dir := w.Repo("photos-api")
	os.MkdirAll(dir, 0o755)
	os.WriteFile(filepath.Join(dir, ".env.example"), []byte("S3_ACCESS_KEY=\nS3_SECRET_KEY=\nS3_ENDPOINT=http://127.0.0.1:9000\nFACE_SERVICE_URL=\n"), 0o644)
	os.WriteFile(filepath.Join(w.Dir(), ".env"), []byte("RUSTFS_ACCESS_KEY=custom-key\nRUSTFS_SECRET_KEY=custom-secret-1\n"), 0o600)
	if _, err := Prepare(w); err != nil {
		t.Fatal(err)
	}
	e := envfile.Read(filepath.Join(dir, ".env"))
	if e["S3_ACCESS_KEY"] != "custom-key" || e["S3_SECRET_KEY"] != "custom-secret-1" || e["FACE_SERVICE_URL"] != "" {
		t.Fatalf("face is off by default and S3 keys follow the infra credentials: %v", e)
	}
	w.Settings.Face = true
	if _, err := Prepare(w); err != nil {
		t.Fatal(err)
	}
	if got := envfile.Read(filepath.Join(dir, ".env"))["FACE_SERVICE_URL"]; got != "http://127.0.0.1:8000" {
		t.Fatalf("face on: %q", got)
	}
}

func TestAuthAudiencesAndDocumentsEnvAreAddedToExistingFiles(t *testing.T) {
	w := ws(t)
	auth := w.Repo("auth-api")
	os.MkdirAll(auth, 0o755)
	os.WriteFile(filepath.Join(auth, ".env"), []byte("AUTH_RESOURCE_AUDIENCES_JSON=[\"ipalpha:auth\",\"ipalpha:places\"]\n"), 0o600)
	pers := w.Repo("persons-api")
	os.MkdirAll(pers, 0o755)
	os.WriteFile(filepath.Join(pers, ".env"), []byte("PORT=3002\nDOCUMENTS_API_URL=http://127.0.0.1:4000\n"), 0o600)
	if _, err := Prepare(w); err != nil {
		t.Fatal(err)
	}
	a := envfile.Read(filepath.Join(auth, ".env"))["AUTH_RESOURCE_AUDIENCES_JSON"]
	if !strings.Contains(a, "ipalpha:places") || !strings.Contains(a, "ipalpha:photos") || !strings.Contains(a, "ipalpha:documents") {
		t.Fatalf("audiences: %s", a)
	}
	p := envfile.Read(filepath.Join(pers, ".env"))
	if p["DOCUMENTS_API_URL"] != "http://127.0.0.1:4000" || p["MEMBERSHIP_TERM_DOCUMENT_SLUG"] == "" || p["EXCHANGE_PERSON_CONSENT_CHANGED"] == "" || p["DOCUMENTS_API_AUDIENCE"] != "ipalpha:documents" {
		t.Fatalf("persons env: %v", p)
	}
}

// IPAlpha Sign (core tool): sign-api's client is unbound (revocations:read + notifications:send-template, allowed
// by SEND_TEMPLATE_UNBOUND_CLIENT_IDS), documents-api gets its S3 keys and the dev seal, and an existing dispatch
// origin list gains the Sign web origin.
func TestSignClientsAndEnv(t *testing.T) {
	w := ws(t)
	for _, d := range []string{"core/sign-api", "core/documents-api", "core/dispatch-api"} {
		if err := os.MkdirAll(filepath.Join(w.Root, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	dispatch := filepath.Join(w.Repo("dispatch-api"), ".env")
	os.WriteFile(dispatch, []byte("SOCKET_ALLOWED_ORIGINS=http://localhost:5106,http://localhost:5110\n"), 0o600)
	auth := filepath.Join(w.Repo("auth-api"), ".env")
	os.WriteFile(auth, []byte(`AUTH_RESOURCE_AUDIENCES_JSON='["ipalpha:auth"]'`+"\nSEND_TEMPLATE_UNBOUND_CLIENT_IDS=schedule-api\n"), 0o600)
	if _, err := Prepare(w); err != nil {
		t.Fatal(err)
	}
	var clients []SeedClient
	if err := json.Unmarshal([]byte(envfile.Read(auth)["SEED_CLIENTS_JSON"]), &clients); err != nil {
		t.Fatal(err)
	}
	byID := map[string]SeedClient{}
	for _, c := range clients {
		byID[c.ClientID] = c
	}
	sign := byID["sign-api"]
	if sign.AppID != "" || strings.Join(sign.Scopes, ",") != "revocations:read,notifications:send-template" {
		t.Fatalf("sign-api client: %+v", sign)
	}
	if u := envfile.Read(auth)["SEND_TEMPLATE_UNBOUND_CLIENT_IDS"]; u != "schedule-api,sign-api" {
		t.Fatalf("unbound send-template clients: %q", u)
	}
	if byID["persons-api"].AppID != "" || strings.Contains(envfile.Read(auth)["SEED_CLIENTS_JSON"], `"appId":""`) {
		t.Fatal("unbound clients must not carry appId")
	}
	if a := envfile.Read(auth)["AUTH_RESOURCE_AUDIENCES_JSON"]; !strings.Contains(a, "ipalpha:sign") {
		t.Fatalf("audiences: %s", a)
	}
	s := envfile.Read(filepath.Join(w.Repo("sign-api"), ".env"))
	if s["PORT"] != "3014" || s["TOKEN_AUDIENCE"] != "ipalpha:sign" || s["SIGN_APP_ID"] != "app-ipalpha-sign" || s["AUTH_CLIENT_ID"] != "sign-api" ||
		s["RABBITMQ_URL"] == "" || s["AUTH_API_AUDIENCE"] != "ipalpha:auth" || s["EXCHANGE_SIGN_REQUEST_CHANGED"] != "sign-api.request.changed" ||
		s["EXCHANGE_SIGNED_DOCUMENTS_CHANGED"] != "documents-api.signed-document.changed" || s["EXCHANGE_PERSON_DELETED"] != "persons-api.person.deleted" ||
		s["EXCHANGE_AUTH_REVOCATIONS"] != "auth-api.token.revoked" {
		t.Fatalf("sign-api env: %v", s)
	}
	if _, ok := s["DISPATCH_API_URL"]; ok {
		t.Fatalf("sign-api no longer calls dispatch: %v", s)
	}
	if dv := envfile.Read(dispatch); dv["SIGN_API_URL"] != "http://127.0.0.1:3014" || dv["EXCHANGE_SIGN_REQUEST_CHANGED"] != "sign-api.request.changed" || dv["EXCHANGE_FORMS_RESPONSE_CREATED"] != "forms-api.response.created" {
		t.Fatalf("dispatch-api env: %v", dv)
	}
	d := envfile.Read(filepath.Join(w.Repo("documents-api"), ".env"))
	if d["S3_BUCKET"] != "documents" || d["S3_ACCESS_KEY"] == "" || d["SEAL_DEV_SELF_SIGNED"] != "true" || d["EXCHANGE_SIGNED_DOCUMENTS_CHANGED"] != "documents-api.signed-document.changed" {
		t.Fatalf("documents-api env: %v", d)
	}
	if o := envfile.Read(dispatch)["SOCKET_ALLOWED_ORIGINS"]; o != "http://localhost:5106,http://localhost:5110,http://localhost:5113,http://localhost:5116" {
		t.Fatalf("dispatch origins: %q", o)
	}
}

func TestServicesKidsAPIGetsStableVAPIDKeys(t *testing.T) {
	w := ws(t)
	dir := w.Repo("services-kids-api")
	os.MkdirAll(dir, 0o755)
	os.WriteFile(filepath.Join(dir, ".env.example"), []byte("SESSION_TOKEN_KEY=\nVAPID_PUBLIC_KEY=\nVAPID_PRIVATE_KEY=\nVAPID_SUBJECT=\n"), 0o644)
	if _, err := Prepare(w); err != nil {
		t.Fatal(err)
	}
	e := envfile.Read(filepath.Join(dir, ".env"))
	pub, err := base64.RawURLEncoding.DecodeString(e["VAPID_PUBLIC_KEY"])
	priv, err2 := base64.RawURLEncoding.DecodeString(e["VAPID_PRIVATE_KEY"])
	if err != nil || err2 != nil || len(pub) != 65 || pub[0] != 4 || len(priv) != 32 || len(e["SESSION_TOKEN_KEY"]) != 64 || e["VAPID_SUBJECT"] != "mailto:dev@example.test" {
		t.Fatalf("keys: %v", e)
	}
	if _, err := Prepare(w); err != nil {
		t.Fatal(err)
	}
	if again := envfile.Read(filepath.Join(dir, ".env")); again["VAPID_PUBLIC_KEY"] != e["VAPID_PUBLIC_KEY"] || again["VAPID_PRIVATE_KEY"] != e["VAPID_PRIVATE_KEY"] {
		t.Fatal("VAPID pair must be generated once and kept")
	}
}

func TestVirtualAssistantEnvAndPeerKeys(t *testing.T) {
	w := ws(t)
	for _, d := range []string{"core/virtual-assistant", "core/ai-api", "core/dispatch-api", "core/auth-api"} {
		if err := os.MkdirAll(filepath.Join(w.Root, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := Prepare(w); err != nil {
		t.Fatal(err)
	}
	v := envfile.Read(filepath.Join(w.Repo("virtual-assistant"), ".env"))
	if v["PORT"] != "3019" || v["TOKEN_AUDIENCE"] != "ipalpha:assistant" || v["S3_BUCKET"] != "virtual-assistant" || v["S3_ACCESS_KEY"] == "" ||
		v["EXCHANGE_ASSISTANT_CONVERSATION_CHANGED"] != "virtual-assistant.conversation.changed" || v["WHATSAPP_APP_SECRET"] == "" || v["TWILIO_AUTH_TOKEN"] == "" ||
		!strings.HasSuffix(v["MONGO_URI"], "/virtual-assistant?authSource=admin") {
		t.Fatalf("virtual-assistant env: %v", v)
	}
	ai := envfile.Read(filepath.Join(w.Repo("ai-api"), ".env"))
	if ai["ASSISTANT_CALLERS"] != "virtual-assistant" || ai["ASSISTANT_BILLING_PROJECT_ID"] == "" {
		t.Fatalf("ai-api env: %v", ai)
	}
	d := envfile.Read(filepath.Join(w.Repo("dispatch-api"), ".env"))
	if d["VIRTUAL_ASSISTANT_API_URL"] != "http://127.0.0.1:3019" || d["EXCHANGE_ASSISTANT_IDENTITY_REVIEW_CHANGED"] != "virtual-assistant.identity-review.changed" {
		t.Fatalf("dispatch-api env: %v", d)
	}
	if a := envfile.Read(filepath.Join(w.Repo("auth-api"), ".env"))["AUTH_RESOURCE_AUDIENCES_JSON"]; !strings.Contains(a, "ipalpha:assistant") {
		t.Fatalf("audiences: %s", a)
	}
}
