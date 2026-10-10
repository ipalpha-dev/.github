// Package tools checks the developer tools (git, Node.js, Docker, gh…) and installs missing ones
// with the platform's package manager after the developer confirms.
package tools

import (
	"fmt"
	"os/exec"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/ipalpha-dev/tooling/internal/i18n"
	"github.com/ipalpha-dev/tooling/internal/sys"
)

// Tool is one requirement.
type Tool struct {
	ID       string
	Name     string
	Required bool
	Why      string // i18n key
	URL      string
	Check    func() (string, error) // version or error
	// Package ids per manager.
	Brew, BrewCask, Winget, Apt, Dnf, Pacman string
	Manual                                   string // i18n key with manual instructions
}

var reVersion = regexp.MustCompile(`\d+\.\d+(\.\d+)?`)

func version(name string, args ...string) (string, error) {
	out, errOut, err := sys.RunTimeout(20*time.Second, "", "", name, args...)
	if err != nil {
		return "", fmt.Errorf("%s", strings.TrimSpace(errOut+" "+err.Error()))
	}
	if v := reVersion.FindString(out + errOut); v != "" {
		return v, nil
	}
	return strings.TrimSpace(out), nil
}

// MinNode is the lowest Node.js major the services support.
const MinNode = 20

// All tools, in check order.
var All = []Tool{
	{ID: "git", Name: "Git", Required: true, Why: "tool_why_git", URL: "https://git-scm.com/downloads",
		Check: func() (string, error) { return version("git", "--version") },
		Brew:  "git", Winget: "Git.Git", Apt: "git", Dnf: "git", Pacman: "git"},
	{ID: "node", Name: "Node.js", Required: true, Why: "tool_why_node", URL: "https://nodejs.org/en/download",
		Check: func() (string, error) {
			v, err := version("node", "--version")
			if err != nil {
				return "", err
			}
			major, _ := strconv.Atoi(strings.SplitN(v, ".", 2)[0])
			if major < MinNode {
				return v, fmt.Errorf("%s", i18n.T("tool_node_old", v, MinNode))
			}
			return v, nil
		},
		Brew: "node@22", Winget: "OpenJS.NodeJS.LTS", Apt: "nodejs", Dnf: "nodejs", Pacman: "nodejs npm", Manual: "tool_manual_node"},
	{ID: "docker", Name: "Docker", Required: true, Why: "tool_why_docker", URL: "https://www.docker.com/products/docker-desktop/",
		Check: func() (string, error) {
			if runtime.GOOS == "darwin" && !sys.Has("docker") && sys.Has("container") {
				return version("container", "--version")
			}
			v, err := version("docker", "--version")
			if err != nil {
				return "", err
			}
			if _, err := version("docker", "compose", "version"); err != nil {
				return v, fmt.Errorf("%s", i18n.T("tool_compose_missing"))
			}
			return v, nil
		},
		BrewCask: "docker", Winget: "Docker.DockerDesktop", Apt: "docker.io docker-compose-v2", Dnf: "docker docker-compose", Pacman: "docker docker-compose", Manual: "tool_manual_docker"},
	{ID: "gh", Name: "GitHub CLI", Required: false, Why: "tool_why_gh", URL: "https://cli.github.com",
		Check: func() (string, error) { return version("gh", "--version") },
		Brew:  "gh", Winget: "GitHub.cli", Apt: "gh", Dnf: "gh", Pacman: "github-cli"},
	{ID: "kubectl", Name: "kubectl", Required: false, Why: "tool_why_kubectl", URL: "https://kubernetes.io/docs/tasks/tools/",
		Check: func() (string, error) { return version("kubectl", "version", "--client") },
		Brew:  "kubectl", Winget: "Kubernetes.kubectl", Apt: "", Dnf: "kubectl", Pacman: "kubectl"},
}

// Get a tool by id.
func Get(id string) Tool {
	for _, t := range All {
		if t.ID == id {
			return t
		}
	}
	return Tool{}
}

// Manager is a package manager available on this machine.
type Manager struct {
	ID   string
	Name string
}

// DetectManager returns the package manager to install with (empty when none).
func DetectManager() Manager {
	switch {
	case runtime.GOOS == "darwin" && sys.Has("brew"):
		return Manager{"brew", "Homebrew"}
	case sys.Windows && sys.Has("winget"):
		return Manager{"winget", "winget"}
	case runtime.GOOS == "linux" && sys.Has("apt-get"):
		return Manager{"apt", "apt"}
	case runtime.GOOS == "linux" && sys.Has("dnf"):
		return Manager{"dnf", "dnf"}
	case runtime.GOOS == "linux" && sys.Has("pacman"):
		return Manager{"pacman", "pacman"}
	case runtime.GOOS == "linux" && sys.Has("brew"):
		return Manager{"brew", "Homebrew"}
	}
	return Manager{}
}

// InstallCommand returns the argv that installs t (nil when the manager has no package for it).
func InstallCommand(m Manager, t Tool) []string {
	sudo := func(args ...string) []string {
		if sys.Has("sudo") {
			return append([]string{"sudo"}, args...)
		}
		return args
	}
	switch m.ID {
	case "brew":
		if t.BrewCask != "" {
			return []string{"brew", "install", "--cask", t.BrewCask}
		}
		if t.Brew != "" {
			return []string{"brew", "install", t.Brew}
		}
	case "winget":
		if t.Winget != "" {
			return []string{"winget", "install", "--id", t.Winget, "-e", "--accept-source-agreements", "--accept-package-agreements"}
		}
	case "apt":
		if t.ID == "node" {
			return nil // distro Node.js is often too old: use the NodeSource or nvm instructions
		}
		if t.Apt != "" {
			return sudo(append([]string{"apt-get", "install", "-y"}, strings.Fields(t.Apt)...)...)
		}
	case "dnf":
		if t.Dnf != "" {
			return sudo(append([]string{"dnf", "install", "-y"}, strings.Fields(t.Dnf)...)...)
		}
	case "pacman":
		if t.Pacman != "" {
			return sudo(append([]string{"pacman", "-S", "--noconfirm"}, strings.Fields(t.Pacman)...)...)
		}
	}
	return nil
}

// Install runs the install command attached to the terminal (sudo/winget may ask questions).
func Install(argv []string) error {
	c := exec.Command(argv[0], argv[1:]...)
	c.Stdin, c.Stdout, c.Stderr = stdIO()
	err := c.Run()
	sys.AddToPath()
	return err
}

// Status of one tool.
type Status struct {
	Tool    Tool
	Version string
	Err     error
}

// CheckAll checks every tool.
func CheckAll() []Status {
	sys.AddToPath()
	out := make([]Status, 0, len(All))
	for _, t := range All {
		v, err := t.Check()
		out = append(out, Status{Tool: t, Version: v, Err: err})
	}
	return out
}
