// Package ai runs the AI command a developer chose (any coding CLI, any model) to write release
// decisions. Every engine gets the prompt the same way and is bounded by a timeout; failures carry
// the engine's own error so the developer can fix the configuration.
package ai

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/ipalpha-dev/tooling/internal/i18n"
	"github.com/ipalpha-dev/tooling/internal/sys"
)

// Preset is a known coding-agent CLI with a non-interactive mode.
type Preset struct {
	ID       string
	Name     string
	Command  string
	Args     func(prompt, model string) []string
	Stdin    bool // prompt goes through stdin (no argv length limits)
	Install  string
	Models   string // example model ids shown in setup
	Login    string // how to sign in
	NeedsKey bool
}

// Presets in the order offered at setup.
var Presets = []Preset{
	{ID: "pi", Name: "Pi", Command: "pi", Stdin: true,
		Args:    func(_, m string) []string { return opt([]string{"-p", "--no-session"}, "--model", m) },
		Install: "npm install -g @earendil-works/pi-coding-agent", Models: "anthropic/claude-sonnet-4-5, openai/gpt-5, openrouter/…", Login: "pi (then /login)"},
	{ID: "claude", Name: "Claude Code", Command: "claude", Stdin: true,
		Args:    func(_, m string) []string { return opt([]string{"-p"}, "--model", m) },
		Install: "npm install -g @anthropic-ai/claude-code", Models: "sonnet, opus, haiku", Login: "claude (first run signs in)"},
	{ID: "codex", Name: "OpenAI Codex", Command: "codex", Stdin: true,
		Args:    func(_, m string) []string { return opt([]string{"exec", "--skip-git-repo-check"}, "--model", m) },
		Install: "npm install -g @openai/codex", Models: "gpt-5, gpt-5-codex, o4-mini", Login: "codex login"},
	{ID: "gemini", Name: "Gemini CLI", Command: "gemini", Stdin: true,
		Args:    func(_, m string) []string { return opt([]string{"-p", " "}, "--model", m) },
		Install: "npm install -g @google/gemini-cli", Models: "gemini-2.5-pro, gemini-2.5-flash", Login: "gemini (first run signs in)"},
	{ID: "opencode", Name: "OpenCode", Command: "opencode", Stdin: true,
		Args:    func(_, m string) []string { return opt([]string{"run"}, "--model", m) },
		Install: "npm install -g opencode-ai", Models: "provider/model (anthropic/claude-sonnet-4-5, ollama/qwen3…)", Login: "opencode auth login"},
	{ID: "copilot", Name: "GitHub Copilot CLI", Command: "copilot", Stdin: false,
		Args:    func(p, m string) []string { return opt([]string{"-p", p}, "--model", m) },
		Install: "npm install -g @github/copilot", Models: "claude-sonnet-4.5, gpt-5", Login: "copilot (then /login)"},
	{ID: "grok", Name: "Grok CLI", Command: "grok", Stdin: false,
		Args:    func(p, m string) []string { return opt([]string{"-p", p}, "--model", m) },
		Install: "https://x.ai/cli", Models: "grok-4, grok-code-fast-1", Login: "grok login"},
	{ID: "ollama", Name: "Ollama (local, free)", Command: "ollama", Stdin: true,
		Args:    func(_, m string) []string { return []string{"run", firstNonEmpty(m, "qwen3:8b")} },
		Install: "https://ollama.com/download", Models: "qwen3:8b, llama3.1:8b, gemma3", Login: "ollama pull <model>"},
}

func opt(base []string, flag, value string) []string {
	if strings.TrimSpace(value) == "" {
		return base
	}
	return append(base, flag, value)
}

func firstNonEmpty(v ...string) string {
	for _, s := range v {
		if strings.TrimSpace(s) != "" {
			return s
		}
	}
	return ""
}

// Find a preset by id.
func Find(id string) (Preset, bool) {
	for _, p := range Presets {
		if p.ID == id {
			return p, true
		}
	}
	return Preset{}, false
}

// Installed lists presets whose command is on PATH.
func Installed() []Preset {
	var out []Preset
	for _, p := range Presets {
		if sys.Has(p.Command) {
			out = append(out, p)
		}
	}
	return out
}

// Config of the chosen engine.
type Config struct {
	CLI     string // preset id, "custom", "none" or ""
	Model   string
	Command string // custom template
	Timeout time.Duration
}

// Enabled reports whether an AI is configured.
func (c Config) Enabled() bool { return c.CLI != "" && c.CLI != "none" }

// Label for messages.
func (c Config) Label() string {
	switch c.CLI {
	case "", "none":
		return i18n.T("ai_none")
	case "custom":
		return "custom: " + c.Command
	}
	if c.Model != "" {
		return c.CLI + " · " + c.Model
	}
	return c.CLI + " · " + i18n.T("ai_default_model")
}

// Error is an engine failure with its own output.
type Error struct {
	Engine string
	Detail string
	Hint   string
}

func (e *Error) Error() string { return e.Engine + ": " + e.Detail }

// Ask sends a prompt and returns the reply text.
func Ask(cfg Config, prompt string) (string, error) {
	if !cfg.Enabled() {
		return "", &Error{Engine: "ai", Detail: i18n.T("ai_none"), Hint: i18n.T("ai_hint_configure")}
	}
	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = 120 * time.Second
	}
	name, args, stdin, cleanup, err := command(cfg, prompt)
	if err != nil {
		return "", err
	}
	defer cleanup()
	if _, ok := lookPath(name); !ok {
		hint := i18n.T("ai_hint_install_any")
		if p, ok := Find(cfg.CLI); ok {
			hint = p.Install
		}
		return "", &Error{Engine: cfg.CLI, Detail: i18n.T("ai_not_installed", name), Hint: hint}
	}
	// npm installs CLIs as .cmd shims on Windows; cmd.exe would re-parse the prompt (& | % …), so
	// run the shim's node script directly.
	name, args = sys.UnwrapShim(name, args)
	out, errOut, runErr := sys.RunTimeout(timeout, os.TempDir(), stdin, name, args...)
	if runErr != nil {
		detail := collapse(errOut)
		if detail == "" {
			detail = collapse(out)
		}
		if detail == "" {
			detail = runErr.Error()
		} else if strings.Contains(runErr.Error(), "timed out") {
			detail = runErr.Error() + " — " + detail
		}
		return "", &Error{Engine: cfg.CLI, Detail: detail, Hint: hintFor(cfg, detail)}
	}
	if strings.TrimSpace(out) == "" {
		return "", &Error{Engine: cfg.CLI, Detail: i18n.T("ai_empty_reply"), Hint: hintFor(cfg, errOut)}
	}
	return out, nil
}

func lookPath(name string) (string, bool) {
	if filepath.IsAbs(name) {
		_, err := os.Stat(name)
		return name, err == nil
	}
	return name, sys.Has(name)
}

func command(cfg Config, prompt string) (string, []string, string, func(), error) {
	noop := func() {}
	if cfg.CLI == "custom" {
		tmpl := strings.TrimSpace(cfg.Command)
		if tmpl == "" {
			return "", nil, "", noop, &Error{Engine: "custom", Detail: i18n.T("ai_custom_empty"), Hint: i18n.T("ai_hint_configure")}
		}
		parts := SplitArgs(tmpl)
		var file string
		stdin := prompt
		for i, p := range parts {
			if strings.Contains(p, "{prompt_file}") {
				if file == "" {
					f, err := os.CreateTemp("", "ipalpha-prompt-*.txt")
					if err != nil {
						return "", nil, "", noop, err
					}
					_, _ = f.WriteString(prompt)
					f.Close()
					file = f.Name()
				}
				parts[i] = strings.ReplaceAll(p, "{prompt_file}", file)
				stdin = ""
			}
			if strings.Contains(p, "{prompt}") {
				parts[i] = strings.ReplaceAll(parts[i], "{prompt}", prompt)
				stdin = ""
			}
			parts[i] = strings.ReplaceAll(parts[i], "{model}", cfg.Model)
		}
		cleanup := func() {
			if file != "" {
				os.Remove(file)
			}
		}
		return parts[0], parts[1:], stdin, cleanup, nil
	}
	p, ok := Find(cfg.CLI)
	if !ok {
		return "", nil, "", noop, &Error{Engine: cfg.CLI, Detail: i18n.T("ai_unknown", cfg.CLI), Hint: i18n.T("ai_hint_configure")}
	}
	stdin := ""
	if p.Stdin {
		stdin = prompt
	}
	return p.Command, p.Args(prompt, cfg.Model), stdin, noop, nil
}

func collapse(s string) string {
	s = strings.Join(strings.Fields(stripANSI(s)), " ")
	if len(s) > 400 {
		s = "…" + s[len(s)-400:]
	}
	return s
}

var reANSI = regexp.MustCompile(`\x1b\[[0-9;?]*[ -/]*[@-~]`)

func stripANSI(s string) string { return reANSI.ReplaceAllString(s, "") }

func hintFor(cfg Config, detail string) string {
	low := strings.ToLower(detail)
	switch {
	case strings.Contains(low, "timed out"):
		return i18n.T("ai_hint_timeout")
	case strings.Contains(low, "model") && (strings.Contains(low, "not found") || strings.Contains(low, "unknown") || strings.Contains(low, "invalid") || strings.Contains(low, "does not exist")):
		return i18n.T("ai_hint_model")
	case strings.Contains(low, "auth") || strings.Contains(low, "login") || strings.Contains(low, "api key") || strings.Contains(low, "401") || strings.Contains(low, "unauthorized"):
		if p, ok := Find(cfg.CLI); ok {
			return i18n.T("ai_hint_login", p.Login)
		}
		return i18n.T("ai_hint_login", cfg.CLI)
	case strings.Contains(low, "rate") || strings.Contains(low, "quota") || strings.Contains(low, "429"):
		return i18n.T("ai_hint_quota")
	}
	return i18n.T("ai_hint_configure")
}

// SplitArgs splits a command template like a shell would for simple quoting ('…', "…", \).
func SplitArgs(s string) []string {
	var out []string
	var cur strings.Builder
	inS, inD, has := false, false, false
	for i := 0; i < len(s); i++ {
		ch := s[i]
		switch {
		case ch == '\'' && !inD:
			inS = !inS
			has = true
		case ch == '"' && !inS:
			inD = !inD
			has = true
		case ch == '\\' && !inS && i+1 < len(s) && !sys.Windows:
			i++
			cur.WriteByte(s[i])
			has = true
		case (ch == ' ' || ch == '\t') && !inS && !inD:
			if has {
				out = append(out, cur.String())
				cur.Reset()
				has = false
			}
		default:
			cur.WriteByte(ch)
			has = true
		}
	}
	if has {
		out = append(out, cur.String())
	}
	return out
}

// Decision is the release decision for one repository.
type Decision struct {
	Reason  string `json:"reason"`
	Bump    string `json:"bump"`
	Message string `json:"message"`
}

// ParseDecision extracts {reason,bump,message} from a reply (fences and chatter tolerated).
func ParseDecision(reply string) (Decision, error) {
	var lines []string
	for _, l := range strings.Split(stripANSI(reply), "\n") {
		if !strings.HasPrefix(strings.TrimSpace(l), "```") {
			lines = append(lines, l)
		}
	}
	s := strings.Join(lines, "\n")
	// Use the last balanced JSON object that parses (CLIs may echo the prompt's example first).
	for end := strings.LastIndex(s, "}"); end >= 0; end = strings.LastIndex(s[:end], "}") {
		for start := strings.LastIndex(s[:end], "{"); start >= 0; start = strings.LastIndex(s[:start], "{") {
			var d Decision
			if json.Unmarshal([]byte(s[start:end+1]), &d) == nil && (d.Bump != "" || d.Message != "") {
				d.Bump = strings.ToLower(strings.TrimSpace(d.Bump))
				switch d.Bump {
				case "none", "patch", "minor", "major":
				default:
					d.Bump = "patch"
				}
				d.Reason = strings.Join(strings.Fields(d.Reason), " ")
				d.Message = strings.Join(strings.Fields(d.Message), " ")
				return d, nil
			}
			if start == 0 {
				break
			}
		}
		if end == 0 {
			break
		}
	}
	return Decision{}, errors.New(i18n.T("ai_bad_reply"))
}

// Test sends a tiny prompt and checks the engine answers.
func Test(cfg Config) error {
	cfg.Timeout = 90 * time.Second
	out, err := Ask(cfg, `Reply with exactly this JSON and nothing else: {"reason":"ok","bump":"none","message":"ok"}`)
	if err != nil {
		return err
	}
	if _, err := ParseDecision(out); err != nil {
		return &Error{Engine: cfg.CLI, Detail: fmt.Sprintf("%s: %q", i18n.T("ai_bad_reply"), collapse(out)), Hint: i18n.T("ai_hint_model")}
	}
	return nil
}
