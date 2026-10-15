package env

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/ipalpha-dev/tooling/internal/catalog"
	"github.com/ipalpha-dev/tooling/internal/envfile"
	"github.com/ipalpha-dev/tooling/internal/workspace"
)

// Sample data (seed=on|off in .ipalpha/settings, asked once in setup, ./feature new and preview requests).
//
// The choice owns three keys in the .env of the services below. This is the one explicit exception to "never
// overwrite a developer's value": they are rewritten whenever the choice changes (on = set, off = blanked).
// Nothing is written until the choice exists, and production-bound files never get these keys (.env is local).

const (
	// SeedEnvDev is the IPALPHA_ENV value local workspaces get (previews use preview:<slug> in their configmaps).
	SeedEnvDev = "dev"
)

// seedServices read SEED_DATA / IPALPHA_ENV and seed themselves at boot.
var seedServices = map[string]bool{
	"auth-api": true, "persons-api": true, "organizations-api": true, "places-api": true, "projects-api": true,
	"schedule-api": true, "drive-api": true, "photos-api": true, "documents-api": true, "sign-api": true,
	"forms-api": true, "acampa-kids-backend": true, "services-kids-api": true, "developers-api": true,
}

// seedKeyServices also get SEED_KEY (the same value everywhere) while seeding is on: auth-api honors it, the others
// use it to get a seed-actor token and find the sample people by their sample phone through persons-api (they also
// need AUTH_API_URL and PERSONS_API_URL, which the env-fallback files carry). persons-api seeds in-process and takes
// no key (a stale one is removed); documents-api and places-api seed no people and take none either.
var seedKeyServices = map[string]bool{
	"auth-api": true, "organizations-api": true, "projects-api": true, "schedule-api": true, "drive-api": true,
	"photos-api": true, "sign-api": true, "forms-api": true, "acampa-kids-backend": true, "services-kids-api": true,
}

// retiredKeys are no longer read by any service. Removed from existing .env files.
var retiredKeys = []string{"SEED_PERSON_IDS_JSON", "SEED_OWNER_PERSON_ID"}

// applySeed writes the tool-owned seed keys of one service's .env. Returns the keys it changed.
func applySeed(w *workspace.Workspace, r catalog.Repo, f *envfile.File) []string {
	var changed []string
	for _, k := range retiredKeys {
		if f.Delete(k) {
			changed = append(changed, k)
		}
	}
	if !seedKeyServices[r.Name] && f.Delete("SEED_KEY") {
		changed = append(changed, "SEED_KEY")
	}
	if !seedServices[r.Name] || !w.Settings.SeedAsked() {
		return changed
	}
	if !w.Settings.SeedOn() {
		for _, k := range []string{"SEED_DATA", "IPALPHA_ENV", "SEED_KEY"} {
			if cur, ok := f.Get(k); ok && strings.TrimSpace(cur) != "" {
				f.Set(k, "")
				changed = append(changed, k)
			}
		}
		return changed
	}
	own := func(k, v string) {
		if cur, ok := f.Get(k); !ok || envfile.Unquote(strings.TrimSpace(cur)) != v {
			f.Set(k, v)
			changed = append(changed, k)
		}
	}
	own("SEED_DATA", "true")
	own("IPALPHA_ENV", SeedEnvDev)
	secrets := seedSecrets(w)
	if seedKeyServices[r.Name] {
		own("SEED_KEY", secrets["SEED_KEY"])
	}
	if r.Runtime != "" { // the Acampa Kids backend: its registration secrets are generated once, same value in the seed
		fill := func(k, v string) {
			if f.SetIfEmpty(k, envfile.Quote(v)) {
				changed = append(changed, k)
			}
		}
		fill("IPALPHA_CLIENT_ID", secrets["IPALPHA_CLIENT_ID"])
		fill("IPALPHA_CLIENT_SECRET", secrets["IPALPHA_CLIENT_SECRET"])
		fill("IPALPHA_SYSTEM_CLIENT_SECRET", secrets["IPALPHA_SYSTEM_CLIENT_SECRET"])
		fill("IPALPHA_SYSTEM_CLIENT_ID", "acampa-kids")
	}
	return changed
}

var seedSecretKeys = []string{"SEED_KEY", "IPALPHA_CLIENT_ID", "IPALPHA_CLIENT_SECRET", "IPALPHA_SYSTEM_CLIENT_SECRET"}

// seedSecrets returns the per-workspace values that must be equal in several .env files, generated once and
// kept in .ipalpha/seed.env (0600). A feature workspace has its own.
func seedSecrets(w *workspace.Workspace) map[string]string {
	path := filepath.Join(w.Dir(), "seed.env")
	cur := envfile.Read(path)
	dirty := false
	for _, k := range seedSecretKeys {
		if cur[k] == "" {
			cur[k] = randomHex(32) // 64 hex chars: SEED_KEY is 32 random bytes, the client secrets need >= 32
			if k == "IPALPHA_CLIENT_ID" {
				cur[k] = "acampa-kids-" + randomHex(6) // a setup-chosen public client id, accepted by auth-api in seed mode
			}
			dirty = true
		}
	}
	if dirty {
		var b strings.Builder
		for _, k := range seedSecretKeys {
			b.WriteString(k + "=" + cur[k] + "\n")
		}
		_ = os.MkdirAll(w.Dir(), 0o755)
		_ = envfile.WriteAtomic(path, []byte(b.String()), 0o600)
	}
	return cur
}
