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

func TestSeedSettingRoundTrip(t *testing.T) {
	s := NewSettings()
	if s.SeedAsked() || !s.SeedOn() || strings.Contains(s.Render(), "seed=") {
		t.Fatal("unset: not asked, default yes, nothing written")
	}
	s.SetSeed(false)
	path := filepath.Join(t.TempDir(), "settings")
	if err := os.WriteFile(path, []byte(s.Render()), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := LoadSettings(path)
	if err != nil || !got.SeedAsked() || got.SeedOn() || !strings.Contains(got.Render(), "seed=off\n") {
		t.Fatalf("round trip: %+v %v", got, err)
	}
	if NormalizeSeed("TRUE") != "on" || NormalizeSeed("no") != "off" || NormalizeSeed("maybe") != "" {
		t.Fatal("NormalizeSeed")
	}
}
