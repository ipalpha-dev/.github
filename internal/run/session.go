// Package run prepares a workspace for ./run (infra, ports, env, deps, keys, local DB) and describes
// every process the panel or plain mode starts. Nothing waits on a peer: each service reports its
// own readiness on GET /ready.
package run

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ipalpha-dev/tooling/internal/catalog"
	"github.com/ipalpha-dev/tooling/internal/deps"
	"github.com/ipalpha-dev/tooling/internal/env"
	"github.com/ipalpha-dev/tooling/internal/envfile"
	"github.com/ipalpha-dev/tooling/internal/i18n"
	"github.com/ipalpha-dev/tooling/internal/infra"
	"github.com/ipalpha-dev/tooling/internal/localdb"
	"github.com/ipalpha-dev/tooling/internal/ports"
	"github.com/ipalpha-dev/tooling/internal/sys"
	"github.com/ipalpha-dev/tooling/internal/ui"
	"github.com/ipalpha-dev/tooling/internal/workspace"
)

// Kind of panel row.
type Kind string

const (
	KindInfra Kind = "infra"
	KindAPI   Kind = "api"
	KindWeb   Kind = "web"
)

// Spec describes one process (or one infra service) of the run.
type Spec struct {
	ID        string
	Name      string
	Kind      Kind
	Group     string // app outside core
	Dir       string
	Command   string
	Args      []string
	Env       []string // added to os.Environ()
	Port      int
	PortKey   string
	URL       string // page opened in a browser
	ReadyURL  string // GET → 200 when ready
	Refs      map[int]bool
	Autostart bool
	Service   string // infra compose service
	BrowserID string // key in settings browser_apps (defaults to ID)
	Missing   string // why it cannot start (not cloned, no package.json)
}

// Browser is the settings key that remembers this row's page.
func (s *Spec) Browser() string {
	if s.BrowserID != "" {
		return s.BrowserID
	}
	return s.ID
}

// Session is a prepared run.
type Session struct {
	W      *workspace.Workspace
	Engine *infra.Engine
	Plan   ports.Plan
	Specs  []*Spec
	Notes  []string // warnings shown in the panel header (ports moved, degraded integrations)
}

// Options of Prepare.
type Options struct {
	SkipInfra bool
	SkipDeps  bool
	Progress  func(string)
}

func (o Options) progress(s string) {
	ui.Raw("%s", s)
	if o.Progress != nil {
		o.Progress(s)
	}
}

// Prepare runs every step before the processes start. Each step fails with a Problem that says
// what to do; nothing here needs a peer service.
func Prepare(w *workspace.Workspace, opt Options) (*Session, error) {
	s := &Session{W: w, Engine: infra.New(w)}
	if err := os.MkdirAll(w.StateDir(), 0o755); err != nil {
		return nil, err
	}
	stopLeftovers(w, opt)

	if !opt.SkipInfra {
		opt.progress(i18n.T("run_step_runtime"))
		if err := s.Engine.EnsureRunning(); err != nil {
			return nil, err
		}
	}

	opt.progress(i18n.T("run_step_ports"))
	s.Plan = resolvePorts(w, s.Engine, opt.SkipInfra)
	if err := s.Plan.Save(w.StateDir()); err != nil {
		return nil, err
	}
	for _, m := range s.Plan.Moves {
		note := i18n.T("run_port_moved", m.Key, m.Preferred, m.Actual)
		if m.Holder != "" {
			note += " (" + i18n.T("run_port_holder", m.Holder) + ")"
		}
		s.Notes = append(s.Notes, note)
	}

	if !opt.SkipInfra {
		opt.progress(i18n.T("run_step_infra"))
		if err := s.Engine.Up(s.Plan); err != nil {
			return nil, err
		}
	}

	opt.progress(i18n.T("run_step_env"))
	if _, err := env.Prepare(w); err != nil {
		return nil, ui.Wrap(i18n.T("run_step_env"), err)
	}

	if !opt.SkipDeps {
		opt.progress(i18n.T("run_step_deps"))
		if err := deps.Run(w, opt.progress); err != nil {
			return nil, err
		}
	}

	rw := ports.NewRewriter(s.Plan)
	if !opt.SkipInfra && w.HasPackage("auth-api") {
		opt.progress(i18n.T("run_step_keys"))
		if err := bootstrapKeys(w, rw, s.Plan); err != nil {
			s.Notes = append(s.Notes, i18n.T("run_keys_failed")+": "+firstLine(err.Error()))
			ui.Raw("keys bootstrap: %v", err)
		}
		opt.progress(i18n.T("run_step_localdb"))
		if res, err := reconcileDB(w, rw, s.Plan); err != nil {
			s.Notes = append(s.Notes, i18n.T("run_localdb_failed")+": "+firstLine(err.Error()))
		} else {
			if len(res.Clients) > 0 {
				s.Notes = append(s.Notes, i18n.T("run_clients_synced", strings.Join(res.Clients, ", ")))
			}
			if len(res.Redirects) > 0 {
				s.Notes = append(s.Notes, i18n.T("run_redirects_synced", strings.Join(res.Redirects, ", ")))
			}
		}
	}

	s.Specs = buildSpecs(w, s.Engine, s.Plan, rw)
	return s, nil
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i > 0 {
		return s[:i]
	}
	return s
}

// resolvePorts keeps each preferred port unless something else holds it. Ports published by this
// workspace's own containers count as free (they are ours).
func resolvePorts(w *workspace.Workspace, eng *infra.Engine, skipInfra bool) ports.Plan {
	owned := map[int]bool{}
	current := map[string]int{}
	if !skipInfra {
		owned = eng.OwnedPorts()
		current = eng.CurrentPorts()
	}
	infraKeys := map[string]bool{}
	for _, s := range catalog.InfraServices {
		infraKeys[s.Key] = true
	}
	preferred := map[string]int{}
	for _, k := range catalog.PortKeys() {
		preferred[k] = w.Settings.Port(k)
	}
	busy := sys.PortBusy
	if os.Getenv("IPALPHA_TEST_NO_PORT_PROBE") == "1" {
		busy = func(int) bool { return false }
	}
	return ports.Resolve(preferred, catalog.PortKeys(), ports.Probe{
		Busy:    busy,
		Ours:    func(key string, p int) bool { return infraKeys[key] && owned[p] && current[key] == p },
		Current: func(key string) int { return current[key] },
		Holder: func(p int) string {
			for _, h := range sys.PortHolders(p) {
				if h.Command != "" {
					return fmt.Sprintf("%s, pid %d", h.Command, h.PID)
				}
				return fmt.Sprintf("pid %d", h.PID)
			}
			return ""
		},
	})
}

// pidFile records the processes of the running session, so the next run can stop orphans
// (a crashed terminal leaves node processes holding the ports).
func pidFile(w *workspace.Workspace) string { return filepath.Join(w.StateDir(), "pids.json") }

// RecordPIDs stores the live pids of this session.
func RecordPIDs(w *workspace.Workspace, pids map[string]int) {
	data, _ := json.Marshal(map[string]any{"owner": os.Getpid(), "pids": pids})
	_ = envfile.WriteAtomic(pidFile(w), data, 0o644)
}

// stopLeftovers stops processes recorded by a previous session whose panel is gone.
func stopLeftovers(w *workspace.Workspace, opt Options) {
	data, err := os.ReadFile(pidFile(w))
	if err != nil {
		return
	}
	var rec struct {
		Owner int            `json:"owner"`
		Pids  map[string]int `json:"pids"`
	}
	if json.Unmarshal(data, &rec) != nil {
		return
	}
	if rec.Owner != 0 && rec.Owner != os.Getpid() && sys.Alive(rec.Owner) {
		return // another ./run of this workspace is still open; its ports will be moved instead
	}
	// Each recorded pid leads its own process group (Detach), so the whole tree goes, even when
	// npm already exited and only tsc --watch / node remain.
	for name, pid := range rec.Pids {
		if sys.GroupAlive(pid) && looksOurs(pid) {
			opt.progress(i18n.T("run_stopping_leftover", name, pid))
			sys.KillGroup(pid)
		}
	}
	os.Remove(pidFile(w))
}

// looksOurs guards against pid reuse: only node/npm/docker/shell processes are stopped.
func looksOurs(pid int) bool {
	name := strings.ToLower(sys.ProcessName(pid))
	if name == "" {
		return !sys.Windows // Unix: the process group check above is enough
	}
	for _, n := range []string{"node", "npm", "docker", "bash", "sh", "cmd", "container", "tsc"} {
		if strings.Contains(filepath.Base(name), n) {
			return true
		}
	}
	return false
}

// RunningOwner returns the pid of another live ./run of this workspace (0 when none).
func RunningOwner(w *workspace.Workspace) int {
	data, err := os.ReadFile(pidFile(w))
	if err != nil {
		return 0
	}
	var rec struct {
		Owner int `json:"owner"`
	}
	if json.Unmarshal(data, &rec) == nil && rec.Owner != os.Getpid() && sys.Alive(rec.Owner) {
		return rec.Owner
	}
	return 0
}

func apiEnv(w *workspace.Workspace, r catalog.Repo, plan ports.Plan, rw ports.Rewriter) ([]string, map[int]bool) {
	vals := envfile.Read(filepath.Join(r.Path(w.Root), ".env"))
	refs := rw.Env(vals)
	vals["PORT"] = strconv.Itoa(plan.Port(r.Name))
	return toEnv(vals), refs
}

func toEnv(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]string, 0, len(keys))
	for _, k := range keys {
		out = append(out, k+"="+m[k])
	}
	return out
}

func urlOf(port int) string { return "http://127.0.0.1:" + strconv.Itoa(port) }

// webEnv computes what Vite and the app's proxy need: every API URL, its own port, and VITE_*
// values from the app's env files with their loopback ports moved to this run's ports.
func webEnv(w *workspace.Workspace, r catalog.Repo, plan ports.Plan, rw ports.Rewriter) ([]string, map[int]bool) {
	vals := map[string]string{}
	dir := r.Path(w.Root)
	for _, name := range []string{".env", ".env.local", ".env.development", ".env.development.local"} {
		for k, v := range envfile.Read(filepath.Join(dir, name)) {
			if strings.HasPrefix(k, "VITE_") {
				vals[k] = v
			}
		}
	}
	port := plan.Port(r.Name)
	self := "http://localhost:" + strconv.Itoa(port)
	auth := envfile.Read(filepath.Join(w.Repo("auth-api"), ".env"))
	def := func(k, v string) {
		if strings.TrimSpace(vals[k]) == "" {
			vals[k] = v
		}
	}
	def("VITE_AUTH_WEBAPP_URL", "http://localhost:5100")
	def("VITE_AUTH_API_URL", "http://localhost:3005")
	switch r.Name {
	case "oikos-webapp":
		def("VITE_APP_CLIENT_ID", firstNonEmpty(auth["BUILTIN_OIKOS_CLIENT_ID"], "oikos-webapp"))
		def("VITE_ENTRY_POINT_KEY", "church")
		def("VITE_AUTH_CALLBACK_URI", "http://localhost:5110/auth/callback")
		def("VITE_FORMS_URL", "http://localhost:5106")
	case "developers-webapp":
		def("VITE_APP_CLIENT_ID", firstNonEmpty(auth["BUILTIN_DEVELOPERS_CLIENT_ID"], "developers-web"))
		def("VITE_ENTRY_POINT_KEY", "portal")
		def("VITE_AUTH_CALLBACK_URI", "http://localhost:5111/auth/callback")
	case "forms-webapp":
		def("VITE_CREATOR_CLIENT_ID", firstNonEmpty(auth["BUILTIN_FORMS_CREATOR_CLIENT_ID"], "forms-creator-web"))
		def("VITE_RESPONDENT_CLIENT_ID", firstNonEmpty(auth["BUILTIN_FORMS_RESPONDENT_CLIENT_ID"], "forms-respondent-web"))
		def("VITE_CREATOR_ENTRY_POINT", "creator")
		def("VITE_RESPONDENT_ENTRY_POINT", "respondent")
		def("VITE_CREATOR_CALLBACK_URI", "http://localhost:5106/auth/callback")
		def("VITE_RESPONDENT_CALLBACK_URI", "http://localhost:5106/respond/callback")
	}
	refs := rw.Env(vals)
	for _, api := range catalog.APIs() {
		key := strings.ToUpper(strings.ReplaceAll(strings.TrimSuffix(api.Name, "-api"), "-", "_")) + "_API_URL"
		vals[key] = urlOf(plan.Port(api.Name))
		refs[plan.Port(api.Name)] = true
	}
	vals["IPALPHA_WEB_PORT"] = strconv.Itoa(port)
	vals["IPALPHA_WEB_URL"] = self
	vals["IPALPHA_VITE_CACHE"] = filepath.Join(w.Dir(), ".vite", r.Name)
	return toEnv(vals), refs
}

func firstNonEmpty(v ...string) string {
	for _, s := range v {
		if strings.TrimSpace(s) != "" {
			return s
		}
	}
	return ""
}

// startScript picks the npm script that runs a service in development.
func startScript(dir string) string {
	data, err := os.ReadFile(filepath.Join(dir, "package.json"))
	if err != nil {
		return "start"
	}
	var pkg struct {
		Scripts map[string]string `json:"scripts"`
	}
	_ = json.Unmarshal(data, &pkg)
	for _, s := range []string{"start:dev", "dev", "start"} {
		if pkg.Scripts[s] != "" {
			return s
		}
	}
	return "start"
}

func buildSpecs(w *workspace.Workspace, eng *infra.Engine, plan ports.Plan, rw ports.Rewriter) []*Spec {
	var specs []*Spec
	for _, svc := range []string{"mongo", "redis", "rabbitmq", "mailpit"} {
		var display, url string
		var port int
		for _, i := range catalog.InfraServices {
			if i.Service == svc && display == "" {
				display, port = i.Display, plan.Port(i.Key)
			}
			if i.Service == svc && i.Browser != "" {
				url = fmt.Sprintf("http://127.0.0.1:%d%s", plan.Port(i.Key), i.Browser)
			}
		}
		name, args, envs := eng.LogsCmd(svc)
		sp := &Spec{ID: svc, Name: display, Kind: KindInfra, Dir: w.Dir(), Command: name, Args: args, Env: envs,
			Port: port, PortKey: svc, URL: url, Service: svc, Autostart: true}
		if svc == "rabbitmq" {
			sp.BrowserID = "rabbitmq-mgmt" // its management UI opens with o
		}
		if svc == "mailpit" {
			sp.ReadyURL = url + "readyz"
		}
		specs = append(specs, sp)
	}
	webBin := filepath.Join(w.Dir(), "bin", "web-dev.mjs")
	for _, r := range catalog.Repos {
		if !r.Runs() {
			continue
		}
		dir := r.Path(w.Root)
		sp := &Spec{ID: r.Name, Name: r.Label(), Group: r.App, Dir: dir, Port: plan.Port(r.Name), PortKey: r.Name}
		if r.Kind == catalog.KindAPI {
			sp.Kind = KindAPI
			sp.Env, sp.Refs = apiEnv(w, r, plan, rw)
			sp.Env = append(sp.Env, sys.NodeEnv()...)
			sp.Command, sp.Args = sys.Npm(), []string{"run", "--silent", startScript(dir)}
			sp.ReadyURL = urlOf(sp.Port) + "/ready"
			sp.Autostart = true
		} else {
			sp.Kind = KindWeb
			sp.Env, sp.Refs = webEnv(w, r, plan, rw)
			sp.Env = append(sp.Env, sys.NodeEnv()...)
			sp.Command, sp.Args = "node", []string{webBin}
			sp.URL = "http://localhost:" + strconv.Itoa(sp.Port) + "/"
			sp.ReadyURL = sp.URL
			sp.Autostart = w.Settings.BrowserSelected(r.Name)
		}
		switch {
		case !exists(dir):
			sp.Missing = i18n.T("run_missing_repo", r.Rel())
			sp.Autostart = false
		case !exists(filepath.Join(dir, "package.json")):
			sp.Missing = i18n.T("run_missing_package", r.Rel())
			sp.Autostart = false
		}
		specs = append(specs, sp)
	}
	return specs
}

func exists(p string) bool { _, err := os.Stat(p); return err == nil }

// bootstrapKeys creates auth-api's signing keys (insert-if-absent in the DATABASE, so a fresh
// Mongo volume also works). Compiles only when the sources are newer than the build.
func bootstrapKeys(w *workspace.Workspace, rw ports.Rewriter, plan ports.Plan) error {
	dir := w.Repo("auth-api")
	data, err := os.ReadFile(filepath.Join(dir, "package.json"))
	if err != nil || !strings.Contains(string(data), "\"keys:bootstrap\"") {
		return nil
	}
	envs, _ := apiEnv(w, catalog.Repo{Name: "auth-api", Kind: catalog.KindAPI}, plan, rw)
	envs = append(envs, sys.NodeEnv()...)
	main := filepath.Join(dir, "dist", "key-maintenance.main.js")
	if !exists(main) || deps.NeedsBuildSince(filepath.Join(dir, "src"), main) {
		tsc := filepath.Join(dir, "node_modules", "typescript", "bin", "tsc")
		res := sys.Cmd{Dir: dir, Env: envs, Name: "node", Args: []string{tsc, "-p", "tsconfig.build.json"}, Log: ui.LogWriter()}.Run()
		if res.Err != nil {
			return fmt.Errorf("tsc: %s", strings.Join(ui.Tail(res.Output, 5), " | "))
		}
	}
	res := sys.Cmd{Dir: dir, Env: envs, Name: sys.Npm(), Args: []string{"run", "--silent", "keys:bootstrap"}, Log: ui.LogWriter()}.Run()
	if res.Err != nil {
		return fmt.Errorf("%s", strings.Join(ui.Tail(res.Output, 5), " | "))
	}
	return nil
}

var builtinEntries = []string{"OIKOS", "FORMS_CREATOR", "FORMS_RESPONDENT", "DEVELOPERS"}

// reconcileDB syncs drifted local client secrets and the loopback addresses of built-in apps.
func reconcileDB(w *workspace.Workspace, rw ports.Rewriter, plan ports.Plan) (localdb.Result, error) {
	auth := envfile.Read(filepath.Join(w.Repo("auth-api"), ".env"))
	rw.Env(auth)
	uri := auth["MONGO_URI"]
	if uri == "" {
		return localdb.Result{}, nil
	}
	var entries []localdb.EntryURLs
	for _, name := range builtinEntries {
		id := auth["BUILTIN_"+name+"_CLIENT_ID"]
		if id == "" {
			continue
		}
		entries = append(entries, localdb.EntryURLs{
			ClientID:  id,
			Origins:   loopbackOnly(auth["BUILTIN_"+name+"_ORIGINS"]),
			Redirects: loopbackOnly(auth["BUILTIN_"+name+"_REDIRECT_URIS"]),
		})
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	return localdb.Reconcile(ctx, uri, env.SeedClientList(w), entries)
}

func loopbackOnly(list string) []string {
	var out []string
	for _, v := range strings.Split(list, ",") {
		v = strings.TrimSpace(v)
		if strings.HasPrefix(v, "http://localhost:") || strings.HasPrefix(v, "http://127.0.0.1:") {
			out = append(out, v)
		}
	}
	return out
}
