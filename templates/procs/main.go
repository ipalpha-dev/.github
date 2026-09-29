package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

type projectJSON struct {
	Root     string        `json:"root"`
	Lang     string        `json:"lang"`
	Infra    infraSpec     `json:"infra"`
	Projects []projectSpec `json:"projects"`
}

type infraSpec struct {
	Start string `json:"start"`
	Logs  string `json:"logs"`
	Stop  string `json:"stop"`
}

type projectSpec struct {
	Name      string `json:"name"`
	Kind      string `json:"kind"`
	Path      string `json:"path"`
	Display   string `json:"display"`
	Port      string `json:"port"`
	Autostart bool   `json:"autostart"`
	Cmd       string `json:"cmd"`
	Frontend  string `json:"frontend"`
	Parent    string `json:"parent"`
}

func main() {
	ipalphaDir := os.Getenv("IPALPHA_DIR")
	if ipalphaDir == "" {
		if len(os.Args) > 1 {
			ipalphaDir = os.Args[1]
		} else {
			exe, _ := os.Executable()
			ipalphaDir = filepath.Clean(filepath.Join(filepath.Dir(exe), ".."))
		}
	}
	ipalphaDir, err := filepath.Abs(ipalphaDir)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	root := filepath.Dir(ipalphaDir)
	projectsPath := filepath.Join(ipalphaDir, "projects.json")
	data, err := os.ReadFile(projectsPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ipalpha-procs: read %s: %v\n", projectsPath, err)
		os.Exit(1)
	}
	var cfg projectJSON
	if err := json.Unmarshal(data, &cfg); err != nil {
		fmt.Fprintf(os.Stderr, "ipalpha-procs: parse projects.json: %v\n", err)
		os.Exit(1)
	}
	if cfg.Root != "" {
		root = cfg.Root
	}
	setLang(cfg.Lang)
	os.Setenv("IPALPHA_PANEL", "1")

	procs := buildProcs(ipalphaDir, root, cfg)
	m := newModel(ipalphaDir, root, procs)
	p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseCellMotion())
	final, err := p.Run()
	if fm, ok := final.(model); ok {
		fm.stopAll()
	} else {
		m.stopAll()
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func buildProcs(ipalphaDir, root string, cfg projectJSON) []*proc {
	specs := cfg.Projects
	depsMap := discoverEnvDeps(root, specs)

	names := make([]string, 0, len(specs))
	for _, s := range specs {
		names = append(names, s.Name)
	}
	scc := sccID(names, depsMap)
	inCycle := cycleMembers(names, depsMap)

	out := make([]*proc, 0, len(specs)+1)
	infraCmd := cfg.Infra.Logs
	if cfg.Infra.Start != "" {
		infraCmd = cfg.Infra.Start + " >/dev/null && " + cfg.Infra.Logs
	}
	out = append(out, &proc{
		id:        "infrastructure",
		name:      tr("dependencies"),
		kind:      "infrastructure",
		cwd:       ipalphaDir,
		shell:     true,
		cmd:       infraCmd,
		stopCmd:   cfg.Infra.Stop,
		autostart: true,
	})

	for _, s := range specs {
		kind := s.Kind
		if kind == "" {
			kind = "service"
		}
		cwd := filepath.Join(root, s.Path)
		port := strings.TrimSpace(s.Port)
		if port == "" {
			port = readPortFromEnvFiles(cwd)
		}
		wait, soft := waitableDeps(s.Name, depsMap[s.Name], scc, inCycle)
		p := &proc{
			id:        s.Name,
			name:      formatDisplayName(s.Display, s.Name),
			kind:      kind,
			cwd:       cwd,
			port:      port,
			frontend:  s.Frontend,
			parent:    s.Parent,
			deps:      wait,
			softDeps:  soft,
			autostart: s.Autostart,
		}
		if s.Cmd != "" {
			p.shell = true
			p.cmd = s.Cmd
		} else {
			p.args = []string{filepath.Join(ipalphaDir, "bin", "node-dev"), s.Name}
		}
		out = append(out, p)
	}
	return groupAttached(out)
}

func groupAttached(procs []*proc) []*proc {
	children := map[string][]*proc{}
	for _, p := range procs {
		if p.kind == "attached" && p.parent != "" {
			children[p.parent] = append(children[p.parent], p)
		}
	}
	out := make([]*proc, 0, len(procs))
	var apps []*proc
	for _, p := range procs {
		switch {
		case p.kind == "attached" && p.parent != "":
		case p.kind == "app":
			apps = append(apps, p)
		default:
			out = append(out, p)
			out = append(out, children[p.id]...)
		}
	}
	return append(out, apps...)
}

func formatDisplayName(display, name string) string {
	s := strings.TrimSpace(display)
	if s == "" {
		s = name
	}
	return s
}
