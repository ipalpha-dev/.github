// Package envfile reads and edits dotenv files as data (never shell code), keeping comments, order
// and every value the developer set. Writes are atomic and private (0600).
package envfile

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var keyLine = regexp.MustCompile(`^\s*([A-Za-z_][A-Za-z0-9_.-]*)\s*=(.*)$`)

// File is an editable dotenv document.
type File struct {
	Path  string
	lines []string
}

// Load reads a file; a missing file is an empty document.
func Load(path string) (*File, error) {
	f := &File{Path: path}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return f, nil
		}
		return nil, err
	}
	text := strings.ReplaceAll(string(data), "\r\n", "\n")
	text = strings.TrimSuffix(text, "\n")
	if text != "" {
		f.lines = strings.Split(text, "\n")
	}
	return f, nil
}

// Parse returns key → unquoted value of a dotenv text.
func Parse(text string) map[string]string {
	out := map[string]string{}
	for _, line := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n") {
		if k, v, ok := parseLine(line); ok {
			if _, seen := out[k]; !seen {
				out[k] = v
			}
		}
	}
	return out
}

// Read parses a file (missing → empty map).
func Read(path string) map[string]string {
	data, err := os.ReadFile(path)
	if err != nil {
		return map[string]string{}
	}
	return Parse(string(data))
}

func parseLine(line string) (string, string, bool) {
	if strings.HasPrefix(strings.TrimSpace(line), "#") {
		return "", "", false
	}
	m := keyLine.FindStringSubmatch(line)
	if m == nil {
		return "", "", false
	}
	return m[1], Unquote(m[2]), true
}

// Unquote strips one pair of matching quotes and surrounding blanks.
func Unquote(v string) string {
	v = strings.TrimSpace(v)
	if len(v) >= 2 && (v[0] == '"' && v[len(v)-1] == '"' || v[0] == '\'' && v[len(v)-1] == '\'') {
		return v[1 : len(v)-1]
	}
	return v
}

// Map of the document.
func (f *File) Map() map[string]string { return Parse(strings.Join(f.lines, "\n")) }

// Get returns the unquoted value of a key.
func (f *File) Get(key string) (string, bool) {
	for _, l := range f.lines {
		if k, v, ok := parseLine(l); ok && k == key {
			return v, true
		}
	}
	return "", false
}

// Has reports whether a key line exists (even empty).
func (f *File) Has(key string) bool { _, ok := f.Get(key); return ok }

// Set replaces the first line of key or appends it. Values are written raw (quote them yourself).
func (f *File) Set(key, raw string) {
	line := key + "=" + raw
	for i, l := range f.lines {
		if k, _, ok := parseLine(l); ok && k == key {
			f.lines[i] = line
			return
		}
	}
	f.lines = append(f.lines, line)
}

// SetIfEmpty sets key only when it is missing or blank. Returns true when it changed.
func (f *File) SetIfEmpty(key, raw string) bool {
	if v, ok := f.Get(key); ok && strings.TrimSpace(v) != "" {
		return false
	}
	f.Set(key, raw)
	return true
}

// AppendLine adds a raw line (used to copy missing keys with their original formatting).
func (f *File) AppendLine(line string) { f.lines = append(f.lines, line) }

// Lines of the document.
func (f *File) Lines() []string { return f.lines }

// String renders the document with a trailing newline.
func (f *File) String() string {
	if len(f.lines) == 0 {
		return ""
	}
	return strings.Join(f.lines, "\n") + "\n"
}

// Save writes atomically with mode 0600.
func (f *File) Save() error { return WriteAtomic(f.Path, []byte(f.String()), 0o600) }

// WriteAtomic writes through a temporary sibling and a rename.
func WriteAtomic(path string, data []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name)
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	_ = os.Chmod(name, mode)
	if err := os.Rename(name, path); err != nil {
		// Windows cannot rename over a file another process holds open: fall back to a direct write.
		return os.WriteFile(path, data, mode)
	}
	return nil
}

// Quote returns a value safe for dotenv parsers: single quotes when it has spaces, quotes or JSON.
func Quote(v string) string {
	if v == "" || !strings.ContainsAny(v, " \t\"'#$`\\{}[]") {
		return v
	}
	if !strings.Contains(v, "'") {
		return "'" + v + "'"
	}
	return "\"" + strings.ReplaceAll(strings.ReplaceAll(v, "\\", "\\\\"), "\"", "\\\"") + "\""
}

// MissingLines returns the key lines of src whose key is absent from dst.
func MissingLines(dst *File, srcText string) []string {
	var out []string
	for _, line := range strings.Split(strings.ReplaceAll(srcText, "\r\n", "\n"), "\n") {
		k, _, ok := parseLine(line)
		if !ok || dst.Has(k) {
			continue
		}
		out = append(out, line)
	}
	return out
}
