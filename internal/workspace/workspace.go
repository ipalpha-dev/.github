// Package workspace finds the IPAlpha workspace (the folder with core/, apps/, deployment and
// .ipalpha/) and reads/writes its settings. Settings stay a plain key=value file people can edit.
package workspace

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/ipalpha-dev/tooling/internal/catalog"
	"github.com/ipalpha-dev/tooling/internal/envfile"
)

// DirName is the per-workspace tooling folder.
const DirName = ".ipalpha"

// Settings of one workspace (.ipalpha/settings).
type Settings struct {
	Lang        string // empty = follow the OS
	Org         string
	Runtime     string // docker | container (Apple) | "" = auto
	AICLI       string // preset id, "custom" or "none"
	AIModel     string
	AICommand   string // custom command template ({prompt} / {prompt_file} / {model})
	GatewayMode string // simulator (default) | real; gateway secrets live only in ai-api/.env
	BrowserApps []string
	browserSet  bool
	InfraName   string
	Registry    string
	Face        bool           // run the optional face service (settings face=on); off = photos-api skips face work
	Seed        string         // seed=on|off: sample data in this workspace's services (empty = not asked yet; default yes)
	Tmux        bool           // settings tmux=on: ./run starts inside the workspace tmux session (off by default)
	Ports       map[string]int // preferred ports (port key → port)
	extra       []string       // unknown lines, kept as written
}

// Workspace is an opened workspace.
type Workspace struct {
	Root     string
	Settings *Settings
	// Feature workspace data (features/<slug>), empty for the main workspace.
	FeatureSlug string
	MainRoot    string
}

// Dir is <root>/.ipalpha.
func (w *Workspace) Dir() string { return filepath.Join(w.Root, DirName) }

// StateDir holds runtime state (pids, actual ports, caches).
func (w *Workspace) StateDir() string { return filepath.Join(w.Dir(), ".state") }

// LogsDir holds one log per command and per process.
func (w *Workspace) LogsDir() string { return filepath.Join(w.Dir(), "logs") }

// SettingsPath is .ipalpha/settings.
func (w *Workspace) SettingsPath() string { return filepath.Join(w.Dir(), "settings") }

// IsFeature reports a features/<slug> workspace.
func (w *Workspace) IsFeature() bool { return w.FeatureSlug != "" }

// Main returns the main workspace root (itself for the main workspace).
func (w *Workspace) Main() string {
	if w.MainRoot != "" {
		return w.MainRoot
	}
	return w.Root
}

// ErrNotFound: no workspace around the current folder.
var ErrNotFound = errors.New("workspace not found")

// Find walks up from start until it finds a folder with .ipalpha/settings.
func Find(start string) (string, error) {
	dir, err := filepath.Abs(start)
	if err != nil {
		return "", err
	}
	for {
		if st, err := os.Stat(filepath.Join(dir, DirName, "settings")); err == nil && !st.IsDir() {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", ErrNotFound
		}
		dir = parent
	}
}

// Open loads a workspace rooted at root.
func Open(root string) (*Workspace, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	w := &Workspace{Root: root}
	s, err := LoadSettings(w.SettingsPath())
	if err != nil {
		return nil, err
	}
	w.Settings = s
	fe := envfile.Read(filepath.Join(w.Dir(), "feature.env"))
	w.FeatureSlug = fe["slug"]
	w.MainRoot = fe["main_root"]
	return w, nil
}

// NewSettings returns defaults.
func NewSettings() *Settings {
	return &Settings{Org: catalog.Org, AICLI: "", GatewayMode: "simulator", InfraName: "ipalpha", Registry: catalog.DefaultRegistry, Ports: map[string]int{}}
}

// LoadSettings reads a settings file; missing → defaults.
func LoadSettings(path string) (*Settings, error) {
	s := NewSettings()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return s, nil
		}
		return nil, err
	}
	for _, line := range strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n") {
		t := strings.TrimSpace(line)
		if t == "" || strings.HasPrefix(t, "#") {
			continue
		}
		k, v, ok := strings.Cut(t, "=")
		if !ok {
			s.extra = append(s.extra, line)
			continue
		}
		k, v = strings.TrimSpace(k), strings.TrimSpace(v)
		switch k {
		case "lang":
			s.Lang = v
		case "org":
			if v != "" {
				s.Org = v
			}
		case "runtime":
			s.Runtime = v
		case "ai_cli":
			s.AICLI = v
		case "ai_model":
			s.AIModel = v
		case "ai_command":
			s.AICommand = v
		case "ai_gateway":
			if v != "simulator" && v != "real" {
				return nil, errors.New("ai_gateway must be simulator or real")
			}
			s.GatewayMode = v
		case "browser_apps":
			s.BrowserApps = strings.Fields(v)
			s.browserSet = true
		case "infra_name":
			if v != "" {
				s.InfraName = v
			}
		case "registry":
			if v != "" {
				s.Registry = v
			}
		case "face":
			s.Face = v == "on" || v == "true" || v == "1"
		case "seed":
			s.Seed = NormalizeSeed(v)
		case "tmux":
			s.Tmux = v == "on" || v == "true" || v == "1"
		case "runner":
			// Removed (the panel and --plain replace mprocs/background runners).
		default:
			if strings.HasSuffix(k, "_port") {
				if p, err := strconv.Atoi(v); err == nil && p > 0 && p < 65536 {
					s.Ports[portKey(strings.TrimSuffix(k, "_port"))] = p
				}
				continue
			}
			s.extra = append(s.extra, line)
		}
	}
	return s, nil
}

// Settings keys used dashes for repos (projects-api_port) and underscores for infra (rabbitmq_mgmt_port).
func portKey(k string) string {
	if k == "rabbitmq_mgmt" {
		return "rabbitmq-mgmt"
	}
	return k
}

func settingsPortKey(k string) string {
	if k == "rabbitmq-mgmt" {
		return "rabbitmq_mgmt"
	}
	return k
}

// NormalizeSeed maps on/true/yes/1 to "on", off/false/no/0 to "off" and anything else to "" (not chosen).
func NormalizeSeed(v string) string {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "on", "true", "yes", "1":
		return "on"
	case "off", "false", "no", "0":
		return "off"
	}
	return ""
}

// SeedAsked reports whether the developer already chose to seed or not.
func (s *Settings) SeedAsked() bool { return s.Seed != "" }

// SeedOn is the choice with its default: sample data unless the developer said no.
func (s *Settings) SeedOn() bool { return s.Seed != "off" }

// SetSeed stores the choice (seed=on|off).
func (s *Settings) SetSeed(on bool) {
	if on {
		s.Seed = "on"
	} else {
		s.Seed = "off"
	}
}

// Gateway defaults old workspaces to the secret-free simulator without editing their env files.
func (s *Settings) Gateway() string {
	if s.GatewayMode == "real" {
		return "real"
	}
	return "simulator"
}

// Port returns the preferred port of a key (settings, else catalog default).
func (s *Settings) Port(key string) int {
	if p := s.Ports[key]; p > 0 {
		return p
	}
	return catalog.DefaultPorts()[key]
}

// Browsers returns the remembered web apps/pages (defaults until the developer changes them).
func (s *Settings) Browsers() []string {
	if s.browserSet {
		return append([]string(nil), s.BrowserApps...)
	}
	return append([]string(nil), catalog.DefaultBrowserApps...)
}

// SetBrowsers remembers a selection (an empty one included).
func (s *Settings) SetBrowsers(ids []string) {
	seen := map[string]bool{}
	s.BrowserApps = nil
	for _, id := range ids {
		if id != "" && !seen[id] {
			seen[id] = true
			s.BrowserApps = append(s.BrowserApps, id)
		}
	}
	s.browserSet = true
}

// BrowserSelected reports whether an id is remembered.
func (s *Settings) BrowserSelected(id string) bool {
	for _, b := range s.Browsers() {
		if b == id {
			return true
		}
	}
	return false
}

// Render writes the settings file content.
func (s *Settings) Render() string {
	var b strings.Builder
	b.WriteString("# IPAlpha workspace settings. Edit with `ipalpha config` / `ipalpha ai`, or by hand.\n")
	b.WriteString("# lang= empty follows the computer's language. Ports are preferences: ./run moves a busy one.\n")
	fmt.Fprintf(&b, "lang=%s\n", s.Lang)
	fmt.Fprintf(&b, "org=%s\n", s.Org)
	fmt.Fprintf(&b, "runtime=%s\n", s.Runtime)
	fmt.Fprintf(&b, "ai_cli=%s\n", s.AICLI)
	fmt.Fprintf(&b, "ai_model=%s\n", s.AIModel)
	fmt.Fprintf(&b, "ai_command=%s\n", s.AICommand)
	fmt.Fprintf(&b, "ai_gateway=%s\n", s.Gateway())
	fmt.Fprintf(&b, "browser_apps=%s\n", strings.Join(s.Browsers(), " "))
	fmt.Fprintf(&b, "infra_name=%s\n", s.InfraName)
	if s.Registry != catalog.DefaultRegistry {
		fmt.Fprintf(&b, "registry=%s\n", s.Registry)
	}
	if s.Face {
		b.WriteString("face=on\n")
	}
	if s.Seed != "" {
		fmt.Fprintf(&b, "seed=%s\n", s.Seed)
	}
	if s.Tmux {
		b.WriteString("tmux=on\n")
	}
	for _, k := range catalog.PortKeys() {
		fmt.Fprintf(&b, "%s_port=%d\n", settingsPortKey(k), s.Port(k))
	}
	extraKeys := catalog.SortedKeys(s.Ports)
	for _, k := range extraKeys {
		if _, known := catalog.DefaultPorts()[k]; !known {
			fmt.Fprintf(&b, "%s_port=%d\n", settingsPortKey(k), s.Ports[k])
		}
	}
	for _, l := range s.extra {
		b.WriteString(l + "\n")
	}
	return b.String()
}

// Save writes .ipalpha/settings.
func (w *Workspace) Save() error {
	return envfile.WriteAtomic(w.SettingsPath(), []byte(w.Settings.Render()), 0o600)
}

var infraSlug = regexp.MustCompile(`[^a-z0-9]+`)

// InfraNameFor derives the container prefix from the workspace folder (IpAlpha → ipalpha,
// ipalpha-2 → ipalpha-2), so two workspaces never share a database.
func InfraNameFor(root string) string {
	slug := strings.Trim(infraSlug.ReplaceAllString(strings.ToLower(filepath.Base(root)), "-"), "-")
	if !strings.HasPrefix(slug, "ipalpha") {
		if slug == "" {
			slug = "local"
		}
		slug = "ipalpha-" + slug
	}
	if len(slug) > 40 {
		slug = strings.TrimRight(slug[:40], "-")
	}
	return slug
}

// Repo returns the folder of a repository in this workspace.
func (w *Workspace) Repo(name string) string {
	r, ok := catalog.Get(name)
	if !ok {
		return filepath.Join(w.Root, "core", name)
	}
	return r.Path(w.Root)
}

// Present lists catalog repos whose folder exists, in catalog order.
func (w *Workspace) Present(kinds ...catalog.Kind) []catalog.Repo {
	var out []catalog.Repo
	for _, r := range catalog.ByKind(kinds...) {
		if st, err := os.Stat(r.Path(w.Root)); err == nil && st.IsDir() {
			out = append(out, r)
		}
	}
	return out
}

// PresentRuns lists the ./run processes whose folder exists: core APIs and web apps, plus the apps outside core
// that ./run starts (Acampa Kids), in catalog order.
func (w *Workspace) PresentRuns() []catalog.Repo {
	var out []catalog.Repo
	for _, r := range w.Present(catalog.KindAPI, catalog.KindWeb, catalog.KindOptional) {
		if r.Runs() {
			out = append(out, r)
		}
	}
	return out
}

// HasPackage reports whether a repo folder has a package.json.
func (w *Workspace) HasPackage(name string) bool {
	_, err := os.Stat(filepath.Join(w.Repo(name), "package.json"))
	return err == nil
}

// UsedPorts lists the preferred ports recorded by this workspace and its features (for feature offsets).
func UsedPorts(main string) map[int]bool {
	used := map[int]bool{}
	files := []string{filepath.Join(main, DirName, "settings")}
	if matches, _ := filepath.Glob(filepath.Join(main, "features", "*", DirName, "settings")); len(matches) > 0 {
		files = append(files, matches...)
	}
	for _, f := range files {
		s, err := LoadSettings(f)
		if err != nil {
			continue
		}
		for _, k := range catalog.PortKeys() {
			used[s.Port(k)] = true
		}
	}
	return used
}

// SortedPortKeys of a map.
func SortedPortKeys(m map[string]int) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
