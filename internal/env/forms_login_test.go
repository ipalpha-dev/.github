package env

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/ipalpha-dev/tooling/internal/envfile"
)

func TestFormsRespondentLoginAddsMissingKeysWithoutReplacingDeveloperValues(t *testing.T) {
	w := ws(t)
	if err := os.MkdirAll(w.Repo("projects-api"), 0o755); err != nil {
		t.Fatal(err)
	}
	authPath := filepath.Join(w.Repo("auth-api"), ".env")
	if err := os.WriteFile(authPath, []byte("BUILTIN_FORMS_RESPONDENT_LOGIN_CLIENT_ID=operator-login\nBUILTIN_FORMS_RESPONDENT_LOGIN_ORIGINS=http://localhost:6206\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	formsPath := filepath.Join(w.Repo("forms-api"), ".env")
	if err := os.WriteFile(formsPath, []byte("PROJECTS_API_URL=http://localhost:6301\nFORMS_RESPONDENT_LOGIN_ENTRY_POINT_ID=operator-entry\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if _, err := Prepare(w); err != nil {
			t.Fatal(err)
		}
		auth := envfile.Read(authPath)
		if auth["BUILTIN_FORMS_RESPONDENT_LOGIN_CLIENT_ID"] != "operator-login" || auth["BUILTIN_FORMS_RESPONDENT_LOGIN_ORIGINS"] != "http://localhost:6206" || auth["BUILTIN_FORMS_RESPONDENT_LOGIN_REDIRECT_URIS"] != "http://localhost:5106/respond/login/callback" {
			t.Fatal("respondent login defaults missing or developer values replaced")
		}
		var rules []map[string]string
		if err := json.Unmarshal([]byte(auth["PROJECT_CONTENT_DELEGATES_JSON"]), &rules); err != nil || len(rules) != 1 || rules[0]["targetAppId"] != "app-ipalpha-forms" || rules[0]["scope"] != "project-content:read" {
			t.Fatal("metadata-only delegation default missing or invalid")
		}
		forms := envfile.Read(formsPath)
		if forms["PROJECTS_API_URL"] != "http://localhost:6301" || forms["FORMS_RESPONDENT_LOGIN_ENTRY_POINT_ID"] != "operator-entry" {
			t.Fatal("Forms developer values replaced")
		}
		if forms["PROJECT_CONTENT_ORIGINS"] != "http://localhost:5110,http://localhost:5106" {
			t.Fatal("provider websocket origins missing")
		}
		projects := envfile.Read(filepath.Join(w.Repo("projects-api"), ".env"))
		if projects["SELF_ONLY_ENTRY_POINT_IDS"] != "ep-ipalpha-forms-respondent-login" {
			t.Fatal("self-only permission boundary missing")
		}
	}
}
