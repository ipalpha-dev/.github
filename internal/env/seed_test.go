package env

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ipalpha-dev/tooling/internal/catalog"
	"github.com/ipalpha-dev/tooling/internal/envfile"
)

func TestSeedChoiceOwnsSeedKeys(t *testing.T) {
	w := ws(t)
	acampa, _ := catalog.Get("acampa-kids-backend")
	os.MkdirAll(acampa.Path(w.Root), 0o755)
	auth := filepath.Join(w.Repo("auth-api"), ".env")
	persons := filepath.Join(w.Repo("persons-api"), ".env")
	forms := filepath.Join(w.Repo("forms-api"), ".env")
	os.MkdirAll(w.Repo("services-kids-api"), 0o755)
	sk := filepath.Join(w.Repo("services-kids-api"), ".env")
	notif := filepath.Join(w.Repo("notifications-api"), ".env")
	app := filepath.Join(acampa.Path(w.Root), ".env")
	os.MkdirAll(w.Repo("developers-api"), 0o755)
	dev := filepath.Join(w.Repo("developers-api"), ".env")
	prep := func() {
		t.Helper()
		if _, err := Prepare(w); err != nil {
			t.Fatal(err)
		}
	}
	reload := func(on string) {
		w.Settings.Seed = on
	}

	// Not asked yet: nothing is written, the keys stay blank (fallback) and retired keys are removed.
	os.WriteFile(forms, []byte("SEED_OWNER_PERSON_ID=abc\nSEED_PERSON_IDS_JSON={}\nKEEP=1\n"), 0o600)
	prep()
	if v := envfile.Read(auth); v["SEED_DATA"] != "" || v["SEED_KEY"] != "" || v["IPALPHA_ENV"] != "" {
		t.Fatalf("unasked workspace got seed keys: %v", v)
	}
	if f := envfile.Read(forms); f["KEEP"] != "1" {
		t.Fatal("developer value lost")
	} else if _, ok := f["SEED_OWNER_PERSON_ID"]; ok {
		t.Fatal("SEED_OWNER_PERSON_ID not retired")
	}
	if _, ok := envfile.Read(forms)["SEED_PERSON_IDS_JSON"]; ok {
		t.Fatal("SEED_PERSON_IDS_JSON not retired")
	}

	// On: SEED_DATA/IPALPHA_ENV in the seeding services, SEED_KEY (same value) in every service that takes it (persons-api no longer does).
	reload("on")
	os.WriteFile(auth, append(mustRead(t, auth), []byte("SEED_DATA=false\nIPALPHA_ENV=production\n")...), 0o600)
	prep()
	a, p, f, n, ap := envfile.Read(auth), envfile.Read(persons), envfile.Read(forms), envfile.Read(notif), envfile.Read(app)
	for name, v := range map[string]map[string]string{"auth": a, "persons": p, "forms": f, "acampa": ap, "kids": envfile.Read(sk)} {
		if v["SEED_DATA"] != "true" || v["IPALPHA_ENV"] != "dev" {
			t.Fatalf("%s: SEED_DATA/IPALPHA_ENV not owned by the tool: %v %v", name, v["SEED_DATA"], v["IPALPHA_ENV"])
		}
	}
	if d := envfile.Read(dev); d["SEED_DATA"] != "true" || d["IPALPHA_ENV"] != "dev" || d["SEED_KEY"] != "" {
		t.Fatalf("developers-api seed mode (and no SEED_KEY): %v", d)
	}
	if len(a["SEED_KEY"]) != 64 || a["SEED_KEY"] != ap["SEED_KEY"] {
		t.Fatalf("SEED_KEY not shared: %q %q", a["SEED_KEY"], ap["SEED_KEY"])
	}
	if k := envfile.Read(sk); len(k["SEED_KEY"]) != 64 || k["SEED_KEY"] != a["SEED_KEY"] {
		t.Fatalf("services-kids-api SEED_KEY not shared: %q", k["SEED_KEY"])
	}
	if _, ok := p["SEED_KEY"]; ok {
		t.Fatal("persons-api must not get SEED_KEY (and a stale one is removed)")
	}
	if f["SEED_KEY"] != a["SEED_KEY"] {
		t.Fatalf("forms-api must share SEED_KEY: %q", f["SEED_KEY"])
	}
	// Every service that looks the sample people up through persons-api gets the shared key and the URLs it needs.
	for _, name := range []string{"organizations-api", "projects-api", "schedule-api", "drive-api", "photos-api", "sign-api"} {
		os.MkdirAll(w.Repo(name), 0o755)
	}
	prep()
	for _, name := range []string{"organizations-api", "projects-api", "schedule-api", "drive-api", "photos-api", "sign-api", "forms-api"} {
		v := envfile.Read(filepath.Join(w.Repo(name), ".env"))
		if v["SEED_KEY"] != a["SEED_KEY"] || v["AUTH_API_URL"] == "" || v["PERSONS_API_URL"] == "" {
			t.Fatalf("%s: SEED_KEY/AUTH_API_URL/PERSONS_API_URL: %q %q %q", name, v["SEED_KEY"], v["AUTH_API_URL"], v["PERSONS_API_URL"])
		}
	}
	for _, name := range []string{"documents-api", "places-api"} {
		if v := envfile.Read(filepath.Join(w.Repo(name), ".env")); v["SEED_KEY"] != "" {
			t.Fatalf("%s must not get SEED_KEY", name)
		}
	}
	if n["SEED_DATA"] != "" {
		t.Fatal("notifications-api does not seed")
	}
	if !strings.HasPrefix(ap["IPALPHA_CLIENT_ID"], "acampa-kids-") || ap["IPALPHA_CLIENT_ID"] == "acampa-kids-" {
		t.Fatalf("IPALPHA_CLIENT_ID not chosen by setup: %q", ap["IPALPHA_CLIENT_ID"])
	}
	if len(ap["IPALPHA_CLIENT_SECRET"]) < 32 || len(ap["IPALPHA_SYSTEM_CLIENT_SECRET"]) < 32 || ap["IPALPHA_CLIENT_SECRET"] == ap["IPALPHA_SYSTEM_CLIENT_SECRET"] ||
		ap["IPALPHA_SYSTEM_CLIENT_ID"] != "acampa-kids" || ap["SUPER_ADMIN_PERSON_IDS"] != "" ||
		ap["IPALPHA_DEVELOPERS_API_URL"] != "http://127.0.0.1:3009" || ap["IPALPHA_ENTRY_POINT"] != "login" {
		t.Fatalf("acampa seed registration values: %v", ap)
	}
	key, secret, cid := a["SEED_KEY"], ap["IPALPHA_CLIENT_SECRET"], ap["IPALPHA_CLIENT_ID"]

	// Generated once: a second pass keeps the same values.
	prep()
	if envfile.Read(auth)["SEED_KEY"] != key || envfile.Read(app)["IPALPHA_CLIENT_SECRET"] != secret || envfile.Read(app)["IPALPHA_CLIENT_ID"] != cid {
		t.Fatal("seed secrets regenerated")
	}

	// Off: the tool blanks them again; the registration values the developer may use stay.
	reload("off")
	prep()
	a, p, ap = envfile.Read(auth), envfile.Read(persons), envfile.Read(app)
	if a["SEED_DATA"] != "" || a["IPALPHA_ENV"] != "" || a["SEED_KEY"] != "" || ap["SEED_DATA"] != "" || ap["SEED_KEY"] != "" {
		t.Fatalf("seed keys not blanked: %v %v %v", a, p, ap)
	}
	if k := envfile.Read(sk); k["SEED_DATA"] != "" || k["IPALPHA_ENV"] != "" || k["SEED_KEY"] != "" {
		t.Fatalf("services-kids-api seed keys not blanked: %v", k)
	}
	if ap["IPALPHA_CLIENT_SECRET"] != secret {
		t.Fatal("off must not touch the app's registration secrets")
	}

	// On again reuses the workspace key.
	reload("on")
	prep()
	if envfile.Read(auth)["SEED_KEY"] != key {
		t.Fatal("SEED_KEY must be generated once per workspace")
	}
}

func mustRead(t *testing.T, p string) []byte {
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
