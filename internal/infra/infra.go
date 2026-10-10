// Package infra runs the shared containers (MongoDB, Redis, RabbitMQ, Mailpit) with Docker Compose
// on every OS, or Apple's `container` runtime on macOS when Docker is not available.
package infra

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/ipalpha-dev/tooling/internal/assets"
	"github.com/ipalpha-dev/tooling/internal/catalog"
	"github.com/ipalpha-dev/tooling/internal/envfile"
	"github.com/ipalpha-dev/tooling/internal/i18n"
	"github.com/ipalpha-dev/tooling/internal/ports"
	"github.com/ipalpha-dev/tooling/internal/sys"
	"github.com/ipalpha-dev/tooling/internal/ui"
	"github.com/ipalpha-dev/tooling/internal/workspace"
)

// Runtime names.
const (
	Docker = "docker"
	Apple  = "container"
)

// Engine runs the infra of one workspace.
type Engine struct {
	W       *workspace.Workspace
	Runtime string
	Name    string // compose project / container prefix
}

// Credentials of the local containers (local only, never real secrets).
func credentials(w *workspace.Workspace) map[string]string {
	path := filepath.Join(w.Dir(), ".env")
	vals := envfile.Read(path)
	defaults := map[string]string{"MONGO_USERNAME": "ipalpha", "MONGO_PASSWORD": "ipalpha", "RABBITMQ_USERNAME": "ipalpha", "RABBITMQ_PASSWORD": "ipalpha"}
	for k, v := range defaults {
		if vals[k] == "" {
			vals[k] = v
		}
	}
	return vals
}

// WriteFiles writes compose.yaml and the credentials file into .ipalpha/.
func WriteFiles(w *workspace.Workspace) error {
	if err := assets.WriteTo("compose.yaml", filepath.Join(w.Dir(), "compose.yaml"), 0o644); err != nil {
		return err
	}
	credPath := filepath.Join(w.Dir(), ".env")
	if _, err := os.Stat(credPath); os.IsNotExist(err) {
		return envfile.WriteAtomic(credPath, []byte("MONGO_USERNAME=ipalpha\nMONGO_PASSWORD=ipalpha\nRABBITMQ_USERNAME=ipalpha\nRABBITMQ_PASSWORD=ipalpha\n"), 0o600)
	}
	return nil
}

// Detect picks the runtime: the setting when usable, else Docker, else Apple container (macOS).
func Detect(preferred string) string {
	hasDocker := sys.Has("docker")
	hasApple := runtime.GOOS == "darwin" && sys.Has("container")
	switch {
	case preferred == Apple && hasApple:
		return Apple
	case preferred == Docker && hasDocker:
		return Docker
	case hasDocker:
		return Docker
	case hasApple:
		return Apple
	}
	return ""
}

// New returns the engine for a workspace.
func New(w *workspace.Workspace) *Engine {
	return &Engine{W: w, Runtime: Detect(w.Settings.Runtime), Name: w.Settings.InfraName}
}

// Problem when no runtime is installed.
func noRuntime() *ui.Problem {
	fix := []string{i18n.T("infra_fix_install_docker")}
	if runtime.GOOS == "darwin" {
		fix = append(fix, i18n.T("infra_fix_apple_container"))
	}
	return ui.NewProblem(i18n.T("infra_step"), i18n.T("infra_no_runtime"), fix...)
}

// EnsureRunning makes sure the runtime answers, starting Docker Desktop / Apple container when off.
func (e *Engine) EnsureRunning() error {
	switch e.Runtime {
	case "":
		return noRuntime()
	case Apple:
		if _, err := sys.Output("", "container", "system", "status"); err != nil {
			ui.Info(i18n.T("infra_starting_runtime", "Apple container"))
			if out, err := sys.Output("", "container", "system", "start", "--enable-kernel-install"); err != nil {
				return &ui.Problem{Step: i18n.T("infra_step"), Cause: i18n.T("infra_apple_failed"), Tail: ui.Tail(out+"\n"+err.Error(), 8),
					Fix: []string{"container system start", i18n.T("infra_fix_use_docker")}}
			}
		}
		return nil
	}
	if dockerUp() {
		return composeCheck()
	}
	ui.Info(i18n.T("infra_starting_runtime", "Docker"))
	startDocker()
	deadline := time.Now().Add(120 * time.Second)
	for time.Now().Before(deadline) {
		if dockerUp() {
			return composeCheck()
		}
		time.Sleep(2 * time.Second)
	}
	return ui.NewProblem(i18n.T("infra_step"), i18n.T("infra_docker_down"), dockerFix()...)
}

func dockerUp() bool {
	_, _, err := sys.RunTimeout(15*time.Second, "", "", "docker", "info", "--format", "{{.ServerVersion}}")
	return err == nil
}

func composeCheck() error {
	if _, err := sys.Output("", "docker", "compose", "version"); err != nil {
		return ui.NewProblem(i18n.T("infra_step"), i18n.T("infra_no_compose"), i18n.T("infra_fix_compose"))
	}
	return nil
}

func dockerFix() []string {
	switch {
	case runtime.GOOS == "darwin":
		return []string{i18n.T("infra_fix_open_docker_mac")}
	case sys.Windows:
		return []string{i18n.T("infra_fix_open_docker_win")}
	case sys.IsWSL():
		return []string{i18n.T("infra_fix_wsl_integration")}
	}
	return []string{"sudo systemctl start docker", i18n.T("infra_fix_docker_group")}
}

func startDocker() {
	switch {
	case runtime.GOOS == "darwin":
		_ = exec.Command("open", "-g", "-a", "Docker").Run()
	case sys.Windows:
		exe := filepath.Join(os.Getenv("ProgramFiles"), "Docker", "Docker", "Docker Desktop.exe")
		if _, err := os.Stat(exe); err == nil {
			c := exec.Command(exe)
			sys.Detach(c)
			_ = c.Start()
		}
	case sys.IsWSL():
		_ = exec.Command("cmd.exe", "/c", "start", "", "Docker Desktop").Run()
	default:
		if exec.Command("systemctl", "--user", "start", "docker-desktop").Run() != nil {
			_ = exec.Command("systemctl", "start", "docker").Run() // works when the user may manage it
		}
	}
}

// Env returns the variables compose.yaml reads, for this port plan.
func (e *Engine) Env(plan ports.Plan) []string {
	env := []string{"IPALPHA_INFRA_NAME=" + e.Name}
	for _, s := range catalog.InfraServices {
		env = append(env, fmt.Sprintf("%s=%d", s.EnvKey, plan.Port(s.Key)))
	}
	for k, v := range credentials(e.W) {
		env = append(env, k+"="+v)
	}
	return env
}

// WritePortsEnv records the ports of this run in .ipalpha/ports.env (manual docker compose use).
func (e *Engine) WritePortsEnv(plan ports.Plan) error {
	var b strings.Builder
	for _, s := range catalog.InfraServices {
		fmt.Fprintf(&b, "%s=%d\n", s.EnvKey, plan.Port(s.Key))
	}
	fmt.Fprintf(&b, "IPALPHA_INFRA_NAME=%s\n", e.Name)
	return envfile.WriteAtomic(filepath.Join(e.W.Dir(), "ports.env"), []byte(b.String()), 0o644)
}

func (e *Engine) compose(plan *ports.Plan, args ...string) sys.Cmd {
	full := append([]string{"compose", "-p", e.Name, "-f", filepath.Join(e.W.Dir(), "compose.yaml")}, args...)
	var env []string
	if plan != nil {
		env = e.Env(*plan)
	} else {
		p, _ := ports.Load(e.W.StateDir())
		env = e.Env(p)
	}
	return sys.Cmd{Dir: e.W.Dir(), Env: env, Name: "docker", Args: full, Log: ui.LogWriter()}
}

// OwnedPorts returns the host ports currently published by this workspace's containers.
func (e *Engine) OwnedPorts() map[int]bool {
	owned := map[int]bool{}
	switch e.Runtime {
	case Docker:
		out, err := sys.Output("", "docker", "ps", "--filter", "label=com.docker.compose.project="+e.Name, "--format", "{{.Ports}}")
		if err != nil {
			return owned
		}
		for _, m := range hostPort.FindAllStringSubmatch(out, -1) {
			if p, err := strconv.Atoi(m[1]); err == nil {
				owned[p] = true
			}
		}
	case Apple:
		for _, c := range appleList() {
			if !strings.HasPrefix(c.Configuration.ID, e.Name+"-") {
				continue
			}
			for _, p := range c.Configuration.PublishedPorts {
				owned[p.HostPort] = true
			}
		}
	}
	return owned
}

var hostPort = regexp.MustCompile(`:(\d+)->`)
var mapping = regexp.MustCompile(`:(\d+)->(\d+)/tcp`)

// CurrentPorts maps each infra key to the host port its running container publishes now.
func (e *Engine) CurrentPorts() map[string]int {
	out := map[string]int{}
	byService := map[string]map[int]int{} // service → container port → host port
	switch e.Runtime {
	case Docker:
		raw, err := sys.Output("", "docker", "ps", "--filter", "label=com.docker.compose.project="+e.Name,
			"--format", `{{.Label "com.docker.compose.service"}}|{{.Ports}}`)
		if err != nil {
			return out
		}
		for _, line := range strings.Split(raw, "\n") {
			svc, ports, ok := strings.Cut(line, "|")
			if !ok {
				continue
			}
			for _, m := range mapping.FindAllStringSubmatch(ports, -1) {
				h, _ := strconv.Atoi(m[1])
				c, _ := strconv.Atoi(m[2])
				if byService[svc] == nil {
					byService[svc] = map[int]int{}
				}
				byService[svc][c] = h
			}
		}
	case Apple:
		for _, c := range appleList() {
			if c.Status != "running" || !strings.HasPrefix(c.Configuration.ID, e.Name+"-") {
				continue
			}
			svc := strings.TrimPrefix(c.Configuration.ID, e.Name+"-")
			for _, p := range c.Configuration.PublishedPorts {
				if byService[svc] == nil {
					byService[svc] = map[int]int{}
				}
				byService[svc][p.ContainerPort] = p.HostPort
			}
		}
	}
	for _, s := range catalog.InfraServices {
		if h := byService[s.Service][s.Container]; h > 0 {
			out[s.Key] = h
		}
	}
	return out
}

type appleContainer struct {
	Status        string `json:"status"`
	Configuration struct {
		ID             string `json:"id"`
		PublishedPorts []struct {
			HostPort      int `json:"hostPort"`
			ContainerPort int `json:"containerPort"`
		} `json:"publishedPorts"`
	} `json:"configuration"`
}

func appleList() []appleContainer {
	out, err := sys.Output("", "container", "ls", "--all", "--format", "json")
	if err != nil {
		return nil
	}
	var list []appleContainer
	_ = json.Unmarshal([]byte(out), &list)
	return list
}

// Up starts (or reuses) the containers and waits until each one is healthy.
func (e *Engine) Up(plan ports.Plan) error {
	if err := WriteFiles(e.W); err != nil {
		return err
	}
	_ = e.WritePortsEnv(plan)
	if e.Runtime == Apple {
		return e.appleUp(plan)
	}
	res := e.compose(&plan, "up", "--detach", "--wait", "--wait-timeout", "180", "--remove-orphans").Run()
	if res.Err != nil {
		return e.upProblem(res.Output)
	}
	return nil
}

func (e *Engine) upProblem(output string) *ui.Problem {
	p := &ui.Problem{Step: i18n.T("infra_step"), Cause: i18n.T("infra_up_failed"), Tail: ui.Tail(output, 12)}
	low := strings.ToLower(output)
	switch {
	case strings.Contains(low, "port is already allocated") || strings.Contains(low, "address already in use") || strings.Contains(low, "bind:"):
		p.Cause = i18n.T("infra_port_taken")
		p.Fix = []string{i18n.T("infra_fix_rerun")}
	case strings.Contains(low, "unhealthy"):
		p.Fix = []string{i18n.T("infra_fix_logs"), i18n.T("infra_fix_reset")}
	case strings.Contains(low, "pull access denied") || strings.Contains(low, "tls handshake") || strings.Contains(low, "no such host"):
		p.Cause = i18n.T("infra_pull_failed")
		p.Fix = []string{i18n.T("infra_fix_network")}
	case strings.Contains(low, "no space left"):
		p.Cause = i18n.T("infra_no_space")
		p.Fix = []string{"docker system prune"}
	default:
		p.Fix = []string{i18n.T("infra_fix_logs")}
	}
	return p
}

// Stop stops the containers (kept for a fast next start). purge removes them; volumes also wipes data.
func (e *Engine) Stop(purge, volumes bool) error {
	switch e.Runtime {
	case Apple:
		for _, s := range services() {
			name := e.Name + "-" + s
			_, _ = sys.Output("", "container", "stop", name)
			if purge {
				_, _ = sys.Output("", "container", "delete", "--force", name)
				os.Remove(filepath.Join(e.W.StateDir(), name+".spec"))
			}
		}
		if purge && volumes {
			for _, v := range []string{"mongo-data", "redis-data", "rabbitmq-data"} {
				_, _ = sys.Output("", "container", "volume", "delete", e.Name+"-"+v)
			}
		}
		return nil
	case Docker:
		args := []string{"stop"}
		if purge {
			args = []string{"down"}
			if volumes {
				args = append(args, "--volumes")
			}
		}
		if res := e.compose(nil, args...).Run(); res.Err != nil {
			return &ui.Problem{Step: i18n.T("infra_stop_step"), Cause: res.Err.Error(), Tail: ui.Tail(res.Output, 8)}
		}
	}
	return nil
}

func services() []string { return []string{"mongo", "redis", "rabbitmq", "mailpit"} }

// Health of each service: running+healthy / starting / stopped / unhealthy.
func (e *Engine) Health() map[string]string {
	out := map[string]string{}
	switch e.Runtime {
	case Docker:
		raw, err := sys.Output("", "docker", "ps", "--all", "--filter", "label=com.docker.compose.project="+e.Name,
			"--format", `{{.Label "com.docker.compose.service"}} {{.State}} {{.Status}}`)
		if err != nil {
			return out
		}
		for _, line := range strings.Split(raw, "\n") {
			f := strings.Fields(line)
			if len(f) < 2 {
				continue
			}
			state := f[1]
			if state == "running" {
				switch {
				case strings.Contains(line, "(healthy)"):
					state = "healthy"
				case strings.Contains(line, "(health: starting)"):
					state = "starting"
				case strings.Contains(line, "(unhealthy)"):
					state = "unhealthy"
				}
			}
			out[f[0]] = state
		}
	case Apple:
		for _, c := range appleList() {
			for _, s := range services() {
				if c.Configuration.ID == e.Name+"-"+s {
					out[s] = c.Status
				}
			}
		}
	}
	return out
}

// LogsCmd returns the command that follows one service's logs.
func (e *Engine) LogsCmd(service string) (string, []string, []string) {
	if e.Runtime == Apple {
		return "container", []string{"logs", "--follow", "-n", "200", e.Name + "-" + service}, nil
	}
	c := e.compose(nil, "logs", "--follow", "--tail", "200", "--no-log-prefix", service)
	return c.Name, c.Args, c.Env
}

// Restart restarts one service.
func (e *Engine) Restart(service string) error {
	if e.Runtime == Apple {
		_, err := sys.Output("", "container", "stop", e.Name+"-"+service)
		if err == nil {
			_, err = sys.Output("", "container", "start", e.Name+"-"+service)
		}
		return err
	}
	res := e.compose(nil, "restart", service).Run()
	if res.Err != nil {
		return errors.New(strings.TrimSpace(res.Output))
	}
	return nil
}

// MongoURI is the root connection string of this run (tools use it for local reconciles).
func (e *Engine) MongoURI(plan ports.Plan) string {
	c := credentials(e.W)
	return fmt.Sprintf("mongodb://%s:%s@127.0.0.1:%d/?authSource=admin", c["MONGO_USERNAME"], c["MONGO_PASSWORD"], plan.Port("mongo"))
}
