package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// Browser preferences are workspace settings, not application/member data.
func browserSelection(ipalphaDir string) map[string]bool {
	ids := "auth-webapp forms-webapp mordomia-webapp developers-webapp mailpit"
	if data, err := os.ReadFile(filepath.Join(ipalphaDir, "settings")); err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			if strings.HasPrefix(line, "browser_apps=") {
				ids = strings.TrimSpace(strings.TrimPrefix(line, "browser_apps="))
				break
			}
		}
	}
	selected := map[string]bool{}
	for _, id := range strings.Fields(ids) {
		selected[id] = true
	}
	return selected
}

type browserActionMsg struct {
	id        string
	action    string
	enabled   bool
	err       error
	runAction string
}

func browserAction(ipalphaDir, id, action string, runAction ...string) tea.Cmd {
	return func() tea.Msg {
		cmd := exec.Command("node", filepath.Join(ipalphaDir, "bin", "browser-dev.mjs"), ipalphaDir, action, id)
		output, err := cmd.Output()
		msg := browserActionMsg{id: id, action: action, enabled: action == "open" || strings.TrimSpace(string(output)) == "true", err: err}
		if len(runAction) > 0 {
			msg.runAction = runAction[0]
		}
		return msg
	}
}
