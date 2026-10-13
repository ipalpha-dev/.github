// Package catalog is the single list of IPAlpha repositories, their kinds, folders and default ports.
// Adding a service = one entry here plus its env fallback in internal/assets/files/env-fallback.
package catalog

import (
	"path/filepath"
	"sort"
	"strings"
)

// Org is the GitHub organization that owns every repository.
const Org = "ipalpha-dev"

// ToolingRepo is this repository (org profile + developer tools). It is never cloned into a workspace.
const ToolingRepo = ".github"

// DefaultRegistry is where ./publish pushes images when it builds them locally.
const DefaultRegistry = "registry.kevyn.com.br/ip-alpha/core"

// Public, version-pinned development infrastructure; never a production core service.
const CPAMCSimulatorImage = "docker.io/kevynsax/cpamc-simulator:0.1.2"
const CPAMCSimulatorClientKey = "simulator-client-key"
const CPAMCSimulatorManagementKey = "simulator-management-key"

// Kind of repository.
type Kind string

const (
	KindAPI        Kind = "api"        // NestJS backend run with node (has .env, GET /live + /ready)
	KindWeb        Kind = "web"        // standalone Vite web app
	KindLibrary    Kind = "library"    // shared-js
	KindDeployment Kind = "deployment" // k8s manifests
	KindOptional   Kind = "optional"   // apps outside core with their own registry/TeamCity (Acampa Kids)
	KindDesktop    Kind = "desktop"    // native desktop app (Xcode / Visual Studio): cloned and given feature worktrees only; no port, no .env, not run, not published
	KindService    Kind = "service"    // core service that is not a Node app: own image built by Core (face-service, Python); not a ./run process
)

// Repo describes one repository of the workspace.
type Repo struct {
	Name    string
	Kind    Kind
	App     string // apps/<App>/… when not empty (consumers outside core)
	Port    int    // default local port (APIs and web apps)
	Display string // panel label
	// Image is the image name of an optional app repo (registry …/apps/<app>/<image>).
	Image string
	// Runtime is how ./run starts a repo that is not a Node/NestJS core API: "bun" (Acampa Kids backend,
	// `bun run dev`). Empty for everything else.
	Runtime string
	// Frontend marks an optional app repo that ./run serves with Vite like a web app.
	Frontend bool
}

// Repos in run order: nothing waits for a peer (GET /ready), the order only reads peers-first.
var Repos = []Repo{
	{Name: "deployment", Kind: KindDeployment},
	{Name: "shared-js", Kind: KindLibrary},
	{Name: "projects-api", Kind: KindAPI, Port: 3001, Display: "Projects"},
	{Name: "persons-api", Kind: KindAPI, Port: 3002, Display: "Persons"},
	{Name: "organizations-api", Kind: KindAPI, Port: 3003, Display: "Organizations"},
	{Name: "places-api", Kind: KindAPI, Port: 3011, Display: "Places"},
	{Name: "photos-api", Kind: KindAPI, Port: 3010, Display: "Photos"},
	{Name: "documents-api", Kind: KindAPI, Port: 3012, Display: "Documents"},
	{Name: "schedule-api", Kind: KindAPI, Port: 3013, Display: "Schedule"},
	{Name: "apps-api", Kind: KindAPI, Port: 3015, Display: "Apps"},
	{Name: "notifications-api", Kind: KindAPI, Port: 3004, Display: "Notifications"},
	{Name: "auth-api", Kind: KindAPI, Port: 3005, Display: "Auth"},
	{Name: "forms-api", Kind: KindAPI, Port: 3006, Display: "Forms API"},
	{Name: "sign-api", Kind: KindAPI, Port: 3014, Display: "Sign API"},
	// IPAlpha Drive (core tool).
	{Name: "drive-api", Kind: KindAPI, Port: 3018, Display: "Drive API"},
	{Name: "ai-api", Kind: KindAPI, Port: 3008, Display: "AI"},
	{Name: "developers-api", Kind: KindAPI, Port: 3009, Display: "Developers"},
	{Name: "services-photo-api", Kind: KindAPI, App: "services-photo", Port: 3016, Display: "Services Photo API"},
	// Services Photo desktop apps (private repos): ./pull and ./feature handle them, ./run and ./publish never do.
	{Name: "services-photo-macos", Kind: KindDesktop, App: "services-photo", Display: "Services Photo macOS"},
	{Name: "services-photo-windows", Kind: KindDesktop, App: "services-photo", Display: "Services Photo Windows"},
	// Services Kids (app outside core).
	{Name: "services-kids-api", Kind: KindAPI, App: "services-kids", Port: 3017, Display: "Services Kids API"},
	// Virtual assistant (core). Repo name has no "-api" suffix.
	{Name: "virtual-assistant", Kind: KindAPI, Port: 3019, Display: "Virtual Assistant"},
	{Name: "dispatch-api", Kind: KindAPI, Port: 3007, Display: "Dispatch"},
	{Name: "auth-webapp", Kind: KindWeb, Port: 5100, Display: "Auth Web"},
	{Name: "forms-webapp", Kind: KindWeb, Port: 5106, Display: "Forms Web"},
	{Name: "sign-webapp", Kind: KindWeb, Port: 5113, Display: "Sign Web"},
	{Name: "drive-webapp", Kind: KindWeb, Port: 5116, Display: "Drive Web"},
	{Name: "oikos-webapp", Kind: KindWeb, Port: 5110, Display: "Oikos Web"},
	{Name: "developers-webapp", Kind: KindWeb, Port: 5111, Display: "Developers Web"},
	{Name: "services-photo-webapp", Kind: KindWeb, App: "services-photo", Port: 5114, Display: "Services Photo Web"},
	{Name: "services-kids-webapp", Kind: KindWeb, App: "services-kids", Port: 5115, Display: "Services Kids Web"},
	// Acampa Kids (app outside core, own registry/TeamCity): also ./run processes. The backend is Bun (Hono), the
	// frontend Vite; ports follow the catalog like every other app.
	{Name: "acampa-kids-backend", Kind: KindOptional, App: "acampa-kids", Image: "backend", Runtime: "bun", Port: 3020, Display: "Acampa Kids API"},
	{Name: "acampa-kids-frontend", Kind: KindOptional, App: "acampa-kids", Image: "frontend", Frontend: true, Port: 5117, Display: "Acampa Kids Web"},
	// Moved from Acampa Kids (ipalpha-dev/acampa-kids-face-service): only photos-api calls it. ./run starts its
	// image as the optional "face" infra container (settings face=on); the repo is only cloned for its code.
	{Name: "face-service", Kind: KindService, Display: "Face service"},
}

// FaceImage is the face service container ./run starts when face=on (built by Core's TeamCity from
// ipalpha-dev/face-service; the same text is in internal/assets/files/compose.yaml).
const FaceImage = "registry.kevyn.com.br/ip-alpha/core/face-service:0.1.0"

// Infra service with a host port published by the container runtime.
type Infra struct {
	Key       string // settings / ports.env key prefix
	Service   string // compose service name
	Port      int
	Container int // port inside the container
	Display   string
	EnvKey    string // ports.env variable
	Browser   string // path opened in a browser, when it has a UI
	Optional  bool   // off by default: only runs when the workspace turns it on (settings face=on)
}

// InfraServices are the containers every workspace runs.
var InfraServices = []Infra{
	{Key: "cpamc", Service: "cpamc", Port: 8317, Container: 8317, Display: "cpamc simulator", EnvKey: "CPAMC_HOST_PORT"},
	{Key: "mongo", Service: "mongo", Port: 27017, Container: 27017, Display: "MongoDB", EnvKey: "MONGO_HOST_PORT"},
	{Key: "redis", Service: "redis", Port: 6379, Container: 6379, Display: "Redis", EnvKey: "REDIS_HOST_PORT"},
	{Key: "rabbitmq", Service: "rabbitmq", Port: 5672, Container: 5672, Display: "RabbitMQ", EnvKey: "RABBITMQ_HOST_PORT"},
	{Key: "rabbitmq-mgmt", Service: "rabbitmq", Port: 15672, Container: 15672, Display: "RabbitMQ UI", EnvKey: "RABBITMQ_MGMT_HOST_PORT", Browser: "/"},
	{Key: "mailpit", Service: "mailpit", Port: 8025, Container: 8025, Display: "Mailpit", EnvKey: "MAILPIT_HOST_PORT", Browser: "/"},
	{Key: "rustfs", Service: "rustfs", Port: 9000, Container: 9000, Display: "RustFS (S3)", EnvKey: "RUSTFS_HOST_PORT"},
	{Key: "face", Service: "face", Port: 8000, Container: 8000, Display: "Face service", EnvKey: "FACE_HOST_PORT", Optional: true},
}

// DefaultBrowserApps start (web apps) and open (pages) on ./run until the developer changes them.
var DefaultBrowserApps = []string{"auth-webapp", "oikos-webapp", "sign-webapp", "mailpit"}

// NeverAutoOpen pages start but never get their own tab (auth is a popup the other apps open).
var NeverAutoOpen = map[string]bool{"auth-webapp": true}

// AlwaysAutoOpen pages open on every ./run that opens pages, whatever browser_apps holds (Mailpit has the sign-in
// codes). NeverAutoOpen still wins.
var AlwaysAutoOpen = map[string]bool{"mailpit": true}

// WebBackend is the API a web app's Vite proxy reaches first (others are mounted at /api/<name>).
var WebBackend = map[string]string{
	"auth-webapp":           "auth-api",
	"forms-webapp":          "forms-api",
	"sign-webapp":           "sign-api",
	"drive-webapp":          "drive-api",
	"oikos-webapp":          "persons-api",
	"developers-webapp":     "developers-api",
	"services-photo-webapp": "services-photo-api",
	"services-kids-webapp":  "services-kids-api",
	"acampa-kids-frontend":  "acampa-kids-backend",
}

// SystemClientScopes is the fallback of deployment/base/core/auth-api/system-clients.json (approved scopes only).
// The workspace's deployment copy wins when present.

// Get returns a repository by name.
func Get(name string) (Repo, bool) {
	for _, r := range Repos {
		if r.Name == name {
			return r, true
		}
	}
	return Repo{}, false
}

// Rel is the workspace-relative folder of a repository (slash separated).
func (r Repo) Rel() string {
	switch {
	case r.Kind == KindDeployment:
		return r.Name
	case r.Kind == KindOptional && r.App != "":
		return "apps/" + r.App + "/" + strings.TrimPrefix(r.Name, r.App+"-")
	case r.App != "":
		return "apps/" + r.App + "/" + r.Name
	default:
		return "core/" + r.Name
	}
}

// Path is the absolute folder of a repository inside root.
func (r Repo) Path(root string) string { return filepath.Join(root, filepath.FromSlash(r.Rel())) }

// HasEnv: APIs (and the Bun backend of an app) get a .env completed from .env.example / fallback templates.
func (r Repo) HasEnv() bool { return r.Kind == KindAPI || r.Runtime != "" }

// Runs: APIs and web apps are processes of ./run, and so are the optional app repos that have a port
// (Acampa Kids backend + frontend).
func (r Repo) Runs() bool {
	return r.Kind == KindAPI || r.Kind == KindWeb || (r.Kind == KindOptional && r.Port != 0)
}

// IsWeb: served with Vite (web apps and an app's frontend).
func (r Repo) IsWeb() bool { return r.Kind == KindWeb || r.Frontend }

// IsBackend: a ./run process that is an API (core APIs and an app's Bun backend).
func (r Repo) IsBackend() bool { return r.Runs() && !r.IsWeb() }

// Image: everything but libraries and deployment ships a container image.
func (r Repo) HasImage() bool {
	return r.Kind == KindAPI || r.Kind == KindWeb || r.Kind == KindOptional || r.Kind == KindService
}

// SkippedWhenMissing: repositories a developer may not have (no access, or not created yet) are skipped
// quietly by ./pull, ./doctor and ./feature instead of failing them.
func (r Repo) SkippedWhenMissing() bool {
	return r.Kind == KindOptional || r.Kind == KindService || r.Kind == KindDesktop
}

// Publishable by ./publish (release mode): core services, web apps, libraries and the images of
// apps outside core (Acampa Kids backend/frontend). face-service (KindService) is built by TeamCity only.
func (r Repo) Publishable() bool {
	return r.Kind == KindAPI || r.Kind == KindWeb || r.Kind == KindLibrary || r.IsAppImage()
}

// IsAppImage: an optional app repository that ships its own image under apps/<app>/<image>.
func (r Repo) IsAppImage() bool { return r.Kind == KindOptional && r.App != "" && r.Image != "" }

// TagsLocally: libraries are tagged when published; image repos are tagged by CI when they reach production.
func (r Repo) TagsLocally() bool { return r.Kind == KindLibrary }

// Label for the panel.
func (r Repo) Label() string {
	if r.Display != "" {
		return r.Display
	}
	return r.Name
}

// ByKind lists repositories of the given kinds, in catalog order.
func ByKind(kinds ...Kind) []Repo {
	var out []Repo
	for _, r := range Repos {
		for _, k := range kinds {
			if r.Kind == k {
				out = append(out, r)
				break
			}
		}
	}
	return out
}

// APIs in run order.
func APIs() []Repo { return ByKind(KindAPI) }

// Webs in catalog order: web apps and the frontend of apps outside core (Acampa Kids).
func Webs() []Repo {
	var out []Repo
	for _, r := range Repos {
		if r.IsWeb() {
			out = append(out, r)
		}
	}
	return out
}

// AppGroupTitle turns "acampa-kids" into "Acampa Kids".
func AppGroupTitle(app string) string {
	words := strings.Split(app, "-")
	for i, w := range words {
		if w != "" {
			words[i] = strings.ToUpper(w[:1]) + w[1:]
		}
	}
	return strings.Join(words, " ")
}

// DefaultPorts maps every port key (repo name or infra key) to its default.
func DefaultPorts() map[string]int {
	out := map[string]int{}
	for _, r := range Repos {
		if r.Port != 0 {
			out[r.Name] = r.Port
		}
	}
	for _, i := range InfraServices {
		out[i.Key] = i.Port
	}
	return out
}

// PortKeys in a stable order: infra first (peers read them), then APIs, then web apps.
func PortKeys() []string {
	var out []string
	for _, i := range InfraServices {
		out = append(out, i.Key)
	}
	for _, r := range Repos {
		if r.Port != 0 && r.IsBackend() {
			out = append(out, r.Name)
		}
	}
	for _, r := range Repos {
		if r.Port != 0 && r.IsWeb() {
			out = append(out, r.Name)
		}
	}
	return out
}

// restrictedPorts are the ports Firefox and Chrome refuse to connect to (WHATWG Fetch "bad ports"). A service that
// lands on one is unreachable from the browser (6000 broke the sign-in popup), so no port is ever assigned there.
var restrictedPorts = func() map[int]bool {
	m := map[int]bool{}
	for _, p := range []int{1, 7, 9, 11, 13, 15, 17, 19, 20, 21, 22, 23, 25, 37, 42, 43, 53, 69, 77, 79, 87, 95,
		101, 102, 103, 104, 109, 110, 111, 113, 115, 117, 119, 123, 135, 137, 139, 143, 161, 179, 389, 427, 465,
		512, 513, 514, 515, 526, 530, 531, 532, 540, 548, 554, 556, 563, 587, 601, 636, 989, 990, 993, 995, 1719,
		1720, 1723, 2049, 3659, 4045, 4190, 5060, 5061, 6000, 6566, 6665, 6666, 6667, 6668, 6669, 6679, 6697, 10080} {
		m[p] = true
	}
	return m
}()

// BrowserRestrictedPort reports whether browsers refuse to connect to p.
func BrowserRestrictedPort(p int) bool { return restrictedPorts[p] }

// IsDefaultPort reports whether p is any catalog default (replacement ports avoid them).
func IsDefaultPort(p int) bool {
	for _, v := range DefaultPorts() {
		if v == p {
			return true
		}
	}
	return false
}

// Names of a repo list.
func Names(repos []Repo) []string {
	out := make([]string, 0, len(repos))
	for _, r := range repos {
		out = append(out, r.Name)
	}
	return out
}

// SortedKeys returns the keys of a map sorted.
func SortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
