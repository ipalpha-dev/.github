package env

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/ipalpha-dev/tooling/internal/assets"
	"github.com/ipalpha-dev/tooling/internal/catalog"
	"github.com/ipalpha-dev/tooling/internal/envfile"
)

func TestAuthScheduleAudienceRepairsOldLocalEnvironment(t *testing.T) {
	w := ws(t)
	repo, _ := catalog.Get("auth-api")
	// Reproduce the previous template, not an unrelated partial audience list.
	current := envfile.Parse(assets.EnvFallback("auth-api"))["AUTH_RESOURCE_AUDIENCES_JSON"]
	var audiences []string
	if err := json.Unmarshal([]byte(current), &audiences); err != nil {
		t.Fatal(err)
	}
	old := slices.DeleteFunc(audiences, func(a string) bool { return a == "ipalpha:schedule" })
	old = append(old, "operator:custom")
	data, _ := json.Marshal(old)
	path := filepath.Join(repo.Path(w.Root), ".env")
	original := "AUTH_RESOURCE_AUDIENCES_JSON='" + string(data) + "'\nAUTH_CLIENT_SECRET=existing-fixture\nCUSTOM_VALUE=keep\n"
	if err := os.WriteFile(path, []byte(original), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Complete(w, repo); err != nil {
		t.Fatal(err)
	}
	values := envfile.Read(path)
	var got []string
	if err := json.Unmarshal([]byte(values["AUTH_RESOURCE_AUDIENCES_JSON"]), &got); err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(got, "ipalpha:schedule") || !slices.Contains(got, "operator:custom") || len(got) != len(old)+1 {
		t.Fatal("missing schedule or modified custom audiences", got)
	}
	for _, audience := range old {
		if !slices.Contains(got, audience) {
			t.Fatal("existing audience removed", audience)
		}
	}
	if values["AUTH_CLIENT_SECRET"] != "existing-fixture" || values["CUSTOM_VALUE"] != "keep" {
		t.Fatal("unrelated local values changed")
	}
	before, _ := os.ReadFile(path)
	if _, err := Complete(w, repo); err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(path)
	if string(before) != string(after) {
		t.Fatal("audience repair is not idempotent")
	}
}

func TestFreshAuthFallbackRegistersSchedule(t *testing.T) {
	w := ws(t)
	repo, _ := catalog.Get("auth-api")
	if _, err := Complete(w, repo); err != nil {
		t.Fatal(err)
	}
	var got []string
	if err := json.Unmarshal([]byte(envfile.Read(filepath.Join(repo.Path(w.Root), ".env"))["AUTH_RESOURCE_AUDIENCES_JSON"]), &got); err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(got, "ipalpha:schedule") || !slices.Contains(got, "ipalpha:sign") || !slices.Contains(got, "ipalpha:apps") {
		t.Fatal("new setup is missing built-in audiences", got)
	}
}
