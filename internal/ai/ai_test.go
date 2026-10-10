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
