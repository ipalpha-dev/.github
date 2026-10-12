// Package env creates and completes each API's .env: copy .env.example (or the embedded fallback),
// add keys that appeared later, fill local-only values. It never overwrites a developer's value,
// except the few local safety rules below (notifications always go to Mailpit).
package env

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/ipalpha-dev/tooling/internal/assets"
	"github.com/ipalpha-dev/tooling/internal/catalog"
	"github.com/ipalpha-dev/tooling/internal/envfile"
	"github.com/ipalpha-dev/tooling/internal/sys"
	"github.com/ipalpha-dev/tooling/internal/workspace"
)

// Change reports what happened to one .env.
type Change struct {
	Repo     string
	Created  bool
	Added    []string // keys copied from the template
	Filled   []string // local values filled
	Fallback bool     // the repo has no .env.example yet
	Skipped  bool     // no template at all
}

// Prepare completes the .env of every API present in the workspace.
func Prepare(w *workspace.Workspace) ([]Change, error) {
	var out []Change
	for _, r := range w.Present(catalog.KindAPI) {
		ch, err := Complete(w, r)
		if err != nil {
			return out, fmt.Errorf("%s/.env: %w", r.Rel(), err)
		}
		out = append(out, ch)
	}
	if err := SeedClients(w); err != nil {
		return out, err
	}
	return out, nil
}

// Template returns the env template of an API: its .env.example, else the embedded fallback.
func Template(dir, repo string) (string, bool) {
	if data, err := os.ReadFile(filepath.Join(dir, ".env.example")); err == nil {
		return string(data), false
	}
	return assets.EnvFallback(repo), true
}

// Complete creates/updates one API .env.
func Complete(w *workspace.Workspace, r catalog.Repo) (Change, error) {
	ch := Change{Repo: r.Name}
	dir := r.Path(w.Root)
	tmpl, fallback := Template(dir, r.Name)
	ch.Fallback = fallback
	if tmpl == "" {
		ch.Skipped = true
		return ch, nil
	}
	ensureGitignore(dir, ".env")
	path := filepath.Join(dir, ".env")
	f, err := envfile.Load(path)
	if err != nil {
		return ch, err
	}
	if len(f.Lines()) == 0 {
		ch.Created = true
		for _, l := range strings.Split(strings.TrimRight(strings.ReplaceAll(tmpl, "\r\n", "\n"), "\n"), "\n") {
			f.AppendLine(l)
		}
	} else {
		for _, l := range envfile.MissingLines(f, tmpl) {
			f.AppendLine(l)
			k, _, _ := strings.Cut(l, "=")
			ch.Added = append(ch.Added, strings.TrimSpace(k))
		}
	}
	ch.Filled = fillLocal(w, r, f)
	if ch.Created || len(ch.Added) > 0 || len(ch.Filled) > 0 {
		if err := f.Save(); err != nil {
			return ch, err
		}
	} else {
		_ = os.Chmod(path, 0o600)
	}
	return ch, nil
}

var peers = []string{"projects", "persons", "organizations", "notifications", "forms", "ai", "developers", "dispatch", "places", "photos", "documents", "schedule", "sign", "apps"}

// Stable ids owned by auth-api (src/builtin-apps.ts).
var builtinIDs = [][2]string{
	{"FORMS_APP_ID", "app-ipalpha-forms"},
	{"FORMS_CREATOR_ENTRY_POINT_ID", "ep-ipalpha-forms-creator"},
	{"FORMS_RESPONDENT_ENTRY_POINT_ID", "ep-ipalpha-forms-respondent"},
	{"FORMS_RESPONDENT_LOGIN_ENTRY_POINT_ID", "ep-ipalpha-forms-respondent-login"},
	{"OIKOS_APP_ID", "app-oikos"},
}

// fillLocal fills blanks with local values (ports stay the catalog defaults: ./run maps them).
func fillLocal(w *workspace.Workspace, r catalog.Repo, f *envfile.File) []string {
	var changed []string
	set := func(k, v string) {
		f.Set(k, envfile.Quote(v))
		changed = append(changed, k)
	}
	fill := func(k, v string) {
		if v == "" {
			return
		}
		if cur, ok := f.Get(k); !ok || strings.TrimSpace(cur) == "" {
			set(k, v)
		}
	}
	if r.Name == "forms-api" {
		// The respondent entry point is a confidential client seeded by auth-api (BUILTIN_FORMS_RESPONDENT_*):
		// forms-api must hold the same id, secret and registered callback. Blanks only, never a chosen value.
		auth := envfile.Read(filepath.Join(w.Repo("auth-api"), ".env"))
		fill("FORMS_RESPONDENT_CLIENT_ID", auth["BUILTIN_FORMS_RESPONDENT_CLIENT_ID"])
		fill("FORMS_RESPONDENT_CLIENT_SECRET", auth["BUILTIN_FORMS_RESPONDENT_CLIENT_SECRET"])
		if first, _, _ := strings.Cut(auth["BUILTIN_FORMS_RESPONDENT_REDIRECT_URIS"], ","); strings.TrimSpace(first) != "" {
			fill("FORMS_RESPONDENT_REDIRECT_URI", strings.TrimSpace(first))
		}
	}
	for k, v := range envfile.Parse(assets.EnvFallback(r.Name)) {
		fill(k, v)
	}
	if r.Name == "ai-api" && w.Settings.Gateway() == "simulator" {
		if base, _ := f.Get("AI_BASE_URL"); base == "http://127.0.0.1:8317/v1" {
			fill("AI_API_KEY", catalog.CPAMCSimulatorClientKey)
			fill("CPAMC_MANAGEMENT_KEY", catalog.CPAMCSimulatorManagementKey)
		}
	}
	fill("SERVICE_NAME", r.Name)
	sum := sha256.Sum256([]byte(w.Root))
	host, _ := os.Hostname()
	fill("INSTANCE_ID", fmt.Sprintf("%s-%s", host, hex.EncodeToString(sum[:])[:8]))
	issuer := "http://localhost:3005"
	if auth := envfile.Read(filepath.Join(w.Repo("auth-api"), ".env")); auth["AUTH_TOKEN_ISSUER"] != "" {
		issuer = auth["AUTH_TOKEN_ISSUER"]
	}
	fill("AUTH_TOKEN_ISSUER", issuer)
	if r.Name != "auth-api" {
		// Only the known local defaults (localhost vs 127.0.0.1, auth's default port); a port the
		// developer chose is kept. ./run maps ports at start-up anyway.
		if cur, _ := f.Get("AUTH_TOKEN_ISSUER"); cur != issuer && localIssuer.MatchString(cur) && strings.HasSuffix(cur, ":3005") {
			set("AUTH_TOKEN_ISSUER", issuer) // JWT iss matching is exact; converge the local defaults only
		}
	}
	fill("TOKEN_AUDIENCE", "ipalpha:"+strings.TrimSuffix(r.Name, "-api"))
	fill("AUTH_API_AUDIENCE", "ipalpha:auth")
	for _, p := range peers {
		fill(strings.ToUpper(p)+"_API_AUDIENCE", "ipalpha:"+p)
	}
	for _, fix := range [][3]string{{"FORMS_API_URL", "3007", "3006"}, {"AI_API_URL", "3010", "3008"}} {
		if cur, _ := f.Get(fix[0]); regexp.MustCompile(`^http://(localhost|127\.0\.0\.1):` + fix[1] + `/?$`).MatchString(cur) {
			set(fix[0], "http://127.0.0.1:"+fix[2])
		}
	}
	if cur, _ := f.Get("REDIS_URL"); redisWithAuth.MatchString(cur) {
		set("REDIS_URL", strings.Replace(cur, "ipalpha:ipalpha@", "", 1)) // local Redis has no ACL
	}
	if r.Name == "persons-api" || r.Name == "forms-api" || r.Name == "dispatch-api" {
		for _, id := range builtinIDs {
			fill(id[0], id[1])
		}
	}
	if r.Name == "notifications-api" {
		// Local setup always captures notifications, even when vendor credentials exist (no paid sends).
		for _, k := range []string{"SMS_PROVIDER", "MAIL_PROVIDER"} {
			if cur, _ := f.Get(k); cur != "mailpit" {
				set(k, "mailpit")
			}
		}
		fill("MAILPIT_URL", "http://127.0.0.1:8025")
		if cur, _ := f.Get("DEPLOYMENT_ENVIRONMENT"); cur != "development" && cur != "test" {
			set("DEPLOYMENT_ENVIRONMENT", "development")
		}
	}
	if r.Name == "notifications-api" {
		fill("CHURCH_EMAIL_UNSUBSCRIBE_KEY", randomB64(32))
		fill("CONTACT_LOOKUP_KEY", randomB64(32))
	}
	if r.Name == "photos-api" || r.Name == "notifications-api" || r.Name == "documents-api" {
		// Same keys the RustFS container starts with (.ipalpha/.env), else the local defaults.
		store := envfile.Read(filepath.Join(w.Dir(), ".env"))
		fill("S3_ACCESS_KEY", firstNonEmpty(store["RUSTFS_ACCESS_KEY"], "ipalpha"))
		fill("S3_SECRET_KEY", firstNonEmpty(store["RUSTFS_SECRET_KEY"], "ipalpha-secret"))
		if r.Name == "photos-api" && w.Settings.Face {
			fill("FACE_SERVICE_URL", "http://127.0.0.1:8000")
		}
	}
	if r.Name == "persons-api" {
		fill("IMPORT_ROWS_KEY", randomB64(32))
	}
	if r.Name == "photo-services-api" {
		fill("SESSION_TOKEN_KEY", randomHex(32)) // seals the per-role core tokens of a session: 64 hex characters
	}
	if r.Name == "dispatch-api" {
		// An existing list is kept; only the local origins of newer web apps are appended (IPAlpha Sign).
		if cur, ok := f.Get("SOCKET_ALLOWED_ORIGINS"); ok {
			if merged, changedList := addCSV(cur, "http://localhost:5113"); changedList {
				set("SOCKET_ALLOWED_ORIGINS", merged)
			}
		}
	}
	if r.Name == "auth-api" {
		fill("WEBHOOK_SECRET_KEY", randomB64(32))
		// sign-api is an unbound core client that sends e-mail templates (IPAlpha Sign is a core tool).
		if cur, ok := f.Get("SEND_TEMPLATE_UNBOUND_CLIENT_IDS"); ok {
			if merged, changedList := addCSV(cur, "sign-api"); changedList {
				set("SEND_TEMPLATE_UNBOUND_CLIENT_IDS", merged)
			}
		}
		// An existing list is kept; only the audiences new services need are appended.
		if cur, ok := f.Get("AUTH_RESOURCE_AUDIENCES_JSON"); ok {
			if merged, changedList := addAudiences(cur, "ipalpha:photos", "ipalpha:documents", "ipalpha:schedule", "ipalpha:sign"); changedList {
				set("AUTH_RESOURCE_AUDIENCES_JSON", merged)
			}
		}
	}
	// Machine credentials of this service, registered in auth-api through SEED_CLIENTS_JSON.
	fill("AUTH_CLIENT_ID", r.Name)
	fill("AUTH_CLIENT_SECRET", randomHex(24))
	return changed
}

var (
	localIssuer   = regexp.MustCompile(`^http://(localhost|127\.0\.0\.1):\d+$`)
	redisWithAuth = regexp.MustCompile(`^redis://ipalpha:ipalpha@(localhost|127\.0\.0\.1):\d+/?$`)
)

func randomB64(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return base64.StdEncoding.EncodeToString(b)
}

func randomHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// ensureGitignore keeps entry out of Git through the repository's info/exclude (shared by every
// worktree), never by editing a tracked .gitignore. Outside Git it does nothing.
func ensureGitignore(dir, entry string) {
	common, err := sys.Git(dir, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil || common == "" {
		return
	}
	path := filepath.Join(common, "info", "exclude")
	data, _ := os.ReadFile(path)
	for _, l := range strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n") {
		if strings.TrimSpace(l) == entry {
			return
		}
	}
	text := string(data)
	if text != "" && !strings.HasSuffix(text, "\n") {
		text += "\n"
	}
	_ = os.MkdirAll(filepath.Dir(path), 0o755)
	_ = os.WriteFile(path, []byte(text+entry+"\n"), 0o644)
}

// retiredScopes are grants an internal client no longer has (system-clients.json dropped them). SeedClients
// otherwise only ever adds scopes, so a developer's SEED_CLIENTS_JSON would keep them forever. Forms and Sign
// became core tools: forms-api lost the prefill + webhook-delivery scopes and runs without revocation sync;
// sign-api is no longer bound to its app, so its app-bound-only dispatch:app-channel is gone.
var retiredScopes = map[string][]string{
	"forms-api": {"prefill:purpose", "prefill:redeem", "prefill:read", "persons:write", "dispatch:events", "revocations:read"},
	"sign-api":  {"dispatch:app-channel"},
}

// SeedClient is one entry of auth-api's SEED_CLIENTS_JSON. AppID binds the client to an app
// (system-clients.json `appBound`; empty today: every internal client is unbound) for app-bound-only scopes.
type SeedClient struct {
	ClientID  string   `json:"clientId"`
	Secret    string   `json:"secret"`
	ServiceID string   `json:"serviceId"`
	AppID     string   `json:"appId,omitempty"`
	Scopes    []string `json:"scopes"`
}

// SeedClients writes every local service's client id/secret into auth-api's SEED_CLIENTS_JSON with
// the approved scopes (deployment/base/core/auth-api/system-clients.json, else the embedded copy).
// Operator clients already listed are kept; scopes are only ever added.
func SeedClients(w *workspace.Workspace) error {
	authPath := filepath.Join(w.Repo("auth-api"), ".env")
	auth, err := envfile.Load(authPath)
	if err != nil || len(auth.Lines()) == 0 {
		return nil
	}
	approved, bound, err := approvedScopes(w)
	if err != nil {
		return err
	}
	var ours []SeedClient
	for _, r := range w.Present(catalog.KindAPI) {
		e := envfile.Read(filepath.Join(r.Path(w.Root), ".env"))
		if e["AUTH_CLIENT_ID"] == "" || e["AUTH_CLIENT_SECRET"] == "" {
			continue
		}
		scopes := approved[r.Name]
		if scopes == nil {
			scopes = []string{}
		}
		ours = append(ours, SeedClient{ClientID: e["AUTH_CLIENT_ID"], Secret: e["AUTH_CLIENT_SECRET"], ServiceID: r.Name, AppID: bound[r.Name], Scopes: scopes})
	}
	var current []map[string]any
	if raw, _ := auth.Get("SEED_CLIENTS_JSON"); strings.TrimSpace(raw) != "" {
		if err := json.Unmarshal([]byte(raw), &current); err != nil {
			return errors.New("core/auth-api/.env: SEED_CLIENTS_JSON is not a JSON array; fix or empty it (local clients were not changed)")
		}
	}
	ids := map[string]bool{}
	for _, c := range ours {
		ids[c.ClientID] = true
	}
	var merged []any
	for _, c := range current {
		if id, _ := c["clientId"].(string); !ids[id] {
			merged = append(merged, c)
		}
	}
	for _, c := range ours {
		set := map[string]bool{}
		for _, retired := range retiredScopes[c.ServiceID] {
			if !contains(c.Scopes, retired) {
				set[retired] = true // skipped below, unless system-clients.json grants it again
			}
		}
		var scopes []string
		for _, prev := range current {
			if id, _ := prev["clientId"].(string); id == c.ClientID {
				if list, ok := prev["scopes"].([]any); ok {
					for _, s := range list {
						if str, ok := s.(string); ok && !set[str] {
							set[str] = true
							scopes = append(scopes, str)
						}
					}
				}
			}
		}
		for _, s := range c.Scopes {
			if !set[s] {
				set[s] = true
				scopes = append(scopes, s)
			}
		}
		if scopes == nil {
			scopes = []string{}
		}
		c.Scopes = scopes
		merged = append(merged, c)
	}
	data, _ := json.Marshal(merged)
	line := "'" + string(data) + "'"
	if cur, _ := auth.Get("SEED_CLIENTS_JSON"); cur == string(data) {
		return nil
	}
	auth.Set("SEED_CLIENTS_JSON", line)
	return auth.Save()
}

// approvedScopes returns the scopes per internal client and the app each app-bound client belongs to.
func approvedScopes(w *workspace.Workspace) (map[string][]string, map[string]string, error) {
	var raw []byte
	if data, err := os.ReadFile(filepath.Join(w.Repo("deployment"), "base", "core", "auth-api", "system-clients.json")); err == nil {
		raw = data
	} else {
		raw = assets.MustRead("system-clients.json")
	}
	var doc struct {
		Clients  map[string][]string `json:"clients"`
		AppBound map[string]string   `json:"appBound"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, nil, fmt.Errorf("system-clients.json: %w", err)
	}
	if doc.AppBound == nil {
		doc.AppBound = map[string]string{}
	}
	return doc.Clients, doc.AppBound, nil
}

// SeedClientList returns the clients auth-api will seed (for the DB reconcile step).
func SeedClientList(w *workspace.Workspace) []SeedClient {
	auth := envfile.Read(filepath.Join(w.Repo("auth-api"), ".env"))
	var list []SeedClient
	_ = json.Unmarshal([]byte(auth["SEED_CLIENTS_JSON"]), &list)
	sort.Slice(list, func(i, j int) bool { return list[i].ClientID < list[j].ClientID })
	return list
}

var (
	phoneDigits = regexp.MustCompile(`\D`)
	brMobile    = regexp.MustCompile(`^[1-9]{2}9\d{8}$`)
	controlChar = regexp.MustCompile(`[\x00-\x1f\x7f]`)
	spaces      = regexp.MustCompile(`[ \t]+`)
)

// NormalizePhone accepts Brazilian mobile numbers with or without +55 and returns E.164.
func NormalizePhone(raw string) (string, error) {
	d := phoneDigits.ReplaceAllString(raw, "")
	if len(d) > 11 && strings.HasPrefix(d, "55") {
		d = d[2:]
	}
	if len(d) == 12 && strings.HasPrefix(d, "0") {
		d = d[1:]
	}
	if !brMobile.MatchString(d) {
		return "", errors.New("invalidPhone")
	}
	return "+55" + d, nil
}

// NormalizeName trims and validates a display name.
func NormalizeName(raw string) (string, error) {
	n := spaces.ReplaceAllString(strings.TrimSpace(raw), " ")
	if n == "" || len([]rune(n)) > 200 || controlChar.MatchString(n) {
		return "", errors.New("invalidName")
	}
	return n, nil
}

// WriteSuperuser stores the first sign-in account in auth-api's .env.
func WriteSuperuser(w *workspace.Workspace, name, phone string) error {
	path := filepath.Join(w.Repo("auth-api"), ".env")
	f, err := envfile.Load(path)
	if err != nil {
		return err
	}
	f.Set("SUPERUSER_NAME", envfile.Quote(name))
	f.Set("SUPERUSER_PHONE", phone)
	return f.Save()
}

// Superuser returns the stored first account.
func Superuser(w *workspace.Workspace) (string, string) {
	e := envfile.Read(filepath.Join(w.Repo("auth-api"), ".env"))
	return e["SUPERUSER_NAME"], e["SUPERUSER_PHONE"]
}

func contains(list []string, v string) bool {
	for _, s := range list {
		if s == v {
			return true
		}
	}
	return false
}

func firstNonEmpty(v ...string) string {
	for _, s := range v {
		if strings.TrimSpace(s) != "" {
			return s
		}
	}
	return ""
}

// addCSV appends missing entries to a comma-separated value (quotes around it are tolerated). An empty
// value is left alone (the fallback fills it).
func addCSV(raw string, add ...string) (string, bool) {
	raw = strings.TrimSpace(raw)
	if len(raw) >= 2 && (raw[0] == '\'' || raw[0] == '"') && raw[len(raw)-1] == raw[0] {
		raw = raw[1 : len(raw)-1]
	}
	if strings.TrimSpace(raw) == "" {
		return "", false
	}
	have := map[string]bool{}
	var list []string
	for _, v := range strings.Split(raw, ",") {
		if v = strings.TrimSpace(v); v != "" {
			have[v] = true
			list = append(list, v)
		}
	}
	changed := false
	for _, a := range add {
		if !have[a] {
			list = append(list, a)
			changed = true
		}
	}
	return strings.Join(list, ","), changed
}

// addAudiences appends missing audiences to a JSON array value (quotes around it are tolerated).
// A value that is not a JSON string array is left alone.
func addAudiences(raw string, add ...string) (string, bool) {
	raw = strings.TrimSpace(raw)
	if len(raw) >= 2 && (raw[0] == '\'' || raw[0] == '"') && raw[len(raw)-1] == raw[0] {
		raw = raw[1 : len(raw)-1]
	}
	var list []string
	if json.Unmarshal([]byte(raw), &list) != nil || len(list) == 0 {
		return "", false
	}
	have := map[string]bool{}
	for _, a := range list {
		have[a] = true
	}
	changed := false
	for _, a := range add {
		if !have[a] {
			list = append(list, a)
			changed = true
		}
	}
	data, _ := json.Marshal(list)
	return string(data), changed
}
