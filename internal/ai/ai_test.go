package ai

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestParseDecision(t *testing.T) {
	for _, reply := range []string{
		`{"reason":"adds filter","bump":"minor","message":"feat: add filter"}`,
		"```json\n{\"reason\":\"adds filter\",\"bump\":\"Minor\",\"message\":\"feat: add filter\"}\n```",
		"Sure! Respond with {\"reason\",\"bump\",\"message\"}\nHere it is:\n{\"reason\": \"adds filter\", \"bump\": \"minor\", \"message\": \"feat: add filter\"}\nDone.",
	} {
		d, err := ParseDecision(reply)
		if err != nil || d.Bump != "minor" || d.Message != "feat: add filter" {
			t.Errorf("%q → %+v %v", reply, d, err)
		}
	}
	if d, _ := ParseDecision(`{"reason":"r","bump":"huge","message":"m"}`); d.Bump != "patch" {
		t.Fatal("unknown bump must default to patch")
	}
	if _, err := ParseDecision("I cannot help"); err == nil {
		t.Fatal("no JSON accepted")
	}
}

func TestSplitArgs(t *testing.T) {
	got := SplitArgs(`llm -m "gpt 4" --system 'be brief' {prompt}`)
	want := []string{"llm", "-m", "gpt 4", "--system", "be brief", "{prompt}"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("%q", got)
	}
}

// A custom command gets the prompt on stdin, via {prompt} and via {prompt_file}.
func TestCustomCommand(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses a POSIX shell script")
	}
	dir := t.TempDir()
	script := filepath.Join(dir, "fake-ai")
	os.WriteFile(script, []byte("#!/bin/sh\nif [ -n \"$1\" ] && [ -f \"$1\" ]; then cat \"$1\"; elif [ -n \"$1\" ]; then printf '%s' \"$1\"; else cat; fi\n"), 0o755)
	for _, tmpl := range []string{script, script + " {prompt}", script + " {prompt_file}"} {
		out, err := Ask(Config{CLI: "custom", Command: tmpl}, `{"reason":"r","bump":"none","message":"m"}`)
		if err != nil {
			t.Fatalf("%s: %v", tmpl, err)
		}
		if _, err := ParseDecision(out); err != nil {
			t.Fatalf("%s: %q", tmpl, out)
		}
	}
	_, err := Ask(Config{CLI: "custom", Command: filepath.Join(dir, "missing")}, "x")
	if err == nil || !strings.Contains(err.Error(), "missing") {
		t.Fatalf("missing command: %v", err)
	}
	failing := filepath.Join(dir, "fail")
	os.WriteFile(failing, []byte("#!/bin/sh\necho 'Error: model gpt-x not found' >&2\nexit 2\n"), 0o755)
	_, err = Ask(Config{CLI: "custom", Command: failing}, "x")
	ae, ok := err.(*Error)
	if !ok || !strings.Contains(ae.Detail, "model gpt-x not found") || ae.Hint == "" {
		t.Fatalf("engine error must carry its own message: %#v", err)
	}
}

func TestPresetsArgs(t *testing.T) {
	for _, p := range Presets {
		args := p.Args("PROMPT", "m1")
		joined := strings.Join(args, " ")
		if p.ID != "ollama" && !strings.Contains(joined, "m1") {
			t.Errorf("%s ignores the model: %v", p.ID, args)
		}
		if !p.Stdin && !strings.Contains(joined, "PROMPT") {
			t.Errorf("%s gets no prompt: %v", p.ID, args)
		}
		if p.Stdin && strings.Contains(joined, "PROMPT") {
			t.Errorf("%s sends the prompt twice: %v", p.ID, args)
		}
	}
}

func TestParseModelLists(t *testing.T) {
	cases := []struct {
		name string
		got  []string
		want string
	}{
		{"pi", parseTable(true)("provider  model  context\ncpamc  gpt-6-sol  1M\nollama-native  qwen3:8b  32K\n"), "cpamc/gpt-6-sol|ollama-native/qwen3:8b"},
		{"ollama", parseTable(false)("NAME  ID  SIZE\nqwen2.5:3b  357c  1.9 GB  7 weeks ago\n"), "qwen2.5:3b"},
		{"opencode", parseLines("cpamc/glm-5.3\n\ncpamc/gpt-6-luna\n"), "cpamc/glm-5.3|cpamc/gpt-6-luna"},
		{"grok", parseBullets("You are not authenticated.\n\nDefault model: x\n\nAvailable models:\n  - grok-4.7\n  * gpt-6.1-sol (default)\n"), "grok-4.7|gpt-6.1-sol"},
		{"codex", parseCodex(`{"models":[{"slug":"gpt-6-sol","visibility":"list"},{"slug":"gpt-reserve","visibility":"hide"},{"slug":"gpt-5"}]}`), "gpt-6-sol|gpt-5"},
	}
	for _, c := range cases {
		if strings.Join(c.got, "|") != c.want {
			t.Errorf("%s: %q", c.name, c.got)
		}
	}
	for _, p := range Presets {
		if p.ListArgs == nil && len(p.Known) == 0 {
			t.Errorf("%s has no way to offer models", p.ID)
		}
		if p.ListArgs != nil && p.Parse == nil {
			t.Errorf("%s lists models without a parser", p.ID)
		}
	}
}

// ListModels reads the CLI's own list and drops duplicates.
func TestListModels(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses a POSIX shell script")
	}
	dir := t.TempDir()
	script := filepath.Join(dir, "fake-cli")
	os.WriteFile(script, []byte("#!/bin/sh\nprintf 'a/m1\\na/m2\\na/m1\\n'\n"), 0o755)
	got, err := ListModels(Preset{Command: script, ListArgs: []string{"models"}, Parse: parseLines})
	if err != nil || strings.Join(got, "|") != "a/m1|a/m2" {
		t.Fatalf("%q %v", got, err)
	}
	empty := filepath.Join(dir, "empty-cli")
	os.WriteFile(empty, []byte("#!/bin/sh\nexit 0\n"), 0o755)
	if _, err := ListModels(Preset{Command: empty, ListArgs: []string{"models"}, Parse: parseLines}); err == nil {
		t.Fatal("an empty list must be an error so setup falls back to typing")
	}
	if got, _ := ListModels(Preset{Known: []string{"opus", "sonnet"}}); len(got) != 2 {
		t.Fatalf("known list: %q", got)
	}
}
