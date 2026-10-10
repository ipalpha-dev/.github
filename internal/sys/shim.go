package sys

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

// npm's Windows shim ends with: "%_prog%"  "%dp0%\node_modules\pkg\bin\cli.js" %*
var shimTarget = regexp.MustCompile(`"%dp0%\\([^"]+\.(?:js|mjs|cjs))"`)

// UnwrapShim turns an npm .cmd shim into `node <script> args…` on Windows (no cmd.exe re-parsing
// of arguments). Elsewhere, or when the command is not a shim, it returns its input.
func UnwrapShim(name string, args []string) (string, []string) {
	if !Windows {
		return name, args
	}
	path := name
	if !filepath.IsAbs(path) {
		p, err := exec.LookPath(name)
		if err != nil {
			return name, args
		}
		path = p
	}
	return unwrap(path, args)
}

func unwrap(path string, args []string) (string, []string) {
	ext := strings.ToLower(filepath.Ext(path))
	if ext != ".cmd" && ext != ".bat" {
		return path, args
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return path, args
	}
	m := shimTarget.FindStringSubmatch(string(data))
	if m == nil {
		return path, args
	}
	script := filepath.Join(filepath.Dir(path), filepath.FromSlash(strings.ReplaceAll(m[1], `\`, "/")))
	if _, err := os.Stat(script); err != nil {
		return path, args
	}
	node := "node"
	if local := filepath.Join(filepath.Dir(path), "node.exe"); fileExists(local) {
		node = local
	}
	return node, append([]string{script}, args...)
}

func fileExists(p string) bool { _, err := os.Stat(p); return err == nil }
