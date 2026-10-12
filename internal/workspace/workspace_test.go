package workspace

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// tmux=on is remembered in .ipalpha/settings; it stays off (and unwritten) by default.
func TestTmuxSetting(t *testing.T) {
	p := filepath.Join(t.TempDir(), "settings")
	s, _ := LoadSettings(p)
	if s.Tmux || strings.Contains(s.Render(), "tmux") {
		t.Fatal("tmux must default to off and stay unwritten")
	}
	os.WriteFile(p, []byte("tmux=on\n"), 0o600)
	s, err := LoadSettings(p)
	if err != nil || !s.Tmux || !strings.Contains(s.Render(), "tmux=on\n") {
		t.Fatalf("%v %+v", err, s)
	}
}
