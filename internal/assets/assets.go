// Package assets embeds the files the tools write into a workspace (compose file, env fallbacks,
// system-client scopes, the Vite launcher). The binary is self-contained: nothing is copied from a
// clone of this repository at run time.
package assets

import (
	"embed"
	"io/fs"
	"os"
	"path/filepath"
)

//go:embed all:files
var files embed.FS

// Read returns an embedded file (path relative to files/).
func Read(name string) ([]byte, error) { return files.ReadFile("files/" + name) }

// MustRead panics when the file is missing (a build error, not a runtime condition).
func MustRead(name string) []byte {
	b, err := Read(name)
	if err != nil {
		panic(err)
	}
	return b
}

// EnvFallback returns the fallback env template of an API ("" when none).
func EnvFallback(repo string) string {
	b, err := Read("env-fallback/" + repo + ".env")
	if err != nil {
		return ""
	}
	return string(b)
}

// EnvFallbacks lists the APIs that have a fallback template.
func EnvFallbacks() []string {
	entries, _ := fs.ReadDir(files, "files/env-fallback")
	var out []string
	for _, e := range entries {
		if n := e.Name(); filepath.Ext(n) == ".env" {
			out = append(out, n[:len(n)-4])
		}
	}
	return out
}

// WriteTo copies an embedded file to dest when its content differs.
func WriteTo(name, dest string, mode os.FileMode) error {
	data := MustRead(name)
	if cur, err := os.ReadFile(dest); err == nil && string(cur) == string(data) {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	return os.WriteFile(dest, data, mode)
}
