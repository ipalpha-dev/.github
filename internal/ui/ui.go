// Package ui prints progress, steps and error boxes the same way in every command, and mirrors
// everything into a per-command log file so an unexpected failure can always be read later.
package ui

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"golang.org/x/term"

	"github.com/ipalpha-dev/tooling/internal/i18n"
)

var (
	Blue    = lipgloss.Color("33")
	Accent  = lipgloss.Color("39")
	Green   = lipgloss.Color("42")
	Yellow  = lipgloss.Color("214")
	Red     = lipgloss.Color("196")
	Grey    = lipgloss.Color("245")
	Title   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("15")).Background(Blue).Padding(0, 1)
	Bold    = lipgloss.NewStyle().Bold(true)
	Muted   = lipgloss.NewStyle().Foreground(Grey)
	OK      = lipgloss.NewStyle().Foreground(Green)
	Warn    = lipgloss.NewStyle().Foreground(Yellow)
	Bad     = lipgloss.NewStyle().Foreground(Red).Bold(true)
	Link    = lipgloss.NewStyle().Foreground(Accent).Underline(true)
	errorBx = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(Red).Padding(0, 1)
	warnBx  = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(Yellow).Padding(0, 1)
	infoBx  = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(Blue).Padding(0, 1)
)

// Interactive reports a terminal on stdin and stdout (prompts and the panel need both).
func Interactive() bool {
	if os.Getenv("IPALPHA_PLAIN") == "1" || os.Getenv("CI") != "" {
		return false
	}
	return term.IsTerminal(int(os.Stdin.Fd())) && term.IsTerminal(int(os.Stdout.Fd()))
}

// Width of the terminal (80 when unknown).
func Width() int {
	if w, _, err := term.GetSize(int(os.Stdout.Fd())); err == nil && w > 20 {
		return w
	}
	return 80
}

// Logger writes to the screen and a log file.
type Logger struct {
	mu   sync.Mutex
	out  io.Writer
	file *os.File
	Path string
}

var std = &Logger{out: os.Stdout}

// Log is the process-wide logger.
func Log() *Logger { return std }

// OpenLog starts mirroring output into <dir>/<name>-<timestamp>.log (keeps the 20 newest per name).
func OpenLog(dir, name string) {
	if dir == "" {
		return
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return
	}
	prune(dir, name, 19)
	path := filepath.Join(dir, fmt.Sprintf("%s-%s.log", name, time.Now().Format("20060102-150405")))
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return
	}
	std.mu.Lock()
	if std.file != nil {
		std.file.Close()
	}
	std.file, std.Path = f, path
	std.mu.Unlock()
	fmt.Fprintf(f, "# %s %s\n", time.Now().Format(time.RFC3339), strings.Join(os.Args, " "))
}

func prune(dir, name string, keep int) {
	matches, _ := filepath.Glob(filepath.Join(dir, name+"-*.log"))
	if len(matches) <= keep {
		return
	}
	for _, m := range matches[:len(matches)-keep] { // timestamped names sort chronologically
		os.Remove(m)
	}
}

// CloseLog flushes the file.
func CloseLog() {
	std.mu.Lock()
	defer std.mu.Unlock()
	if std.file != nil {
		std.file.Close()
		std.file = nil
	}
}

// Raw writes a line to the log file only (command output, details).
func Raw(format string, args ...any) {
	std.mu.Lock()
	defer std.mu.Unlock()
	if std.file != nil {
		fmt.Fprintf(std.file, format+"\n", args...)
	}
}

// LogWriter returns a writer into the log file only.
func LogWriter() io.Writer { return logOnly{} }

type logOnly struct{}

func (logOnly) Write(p []byte) (int, error) {
	std.mu.Lock()
	defer std.mu.Unlock()
	if std.file != nil {
		return std.file.Write(p)
	}
	return len(p), nil
}

// Println prints to the screen and the log.
func Println(a ...any) {
	s := fmt.Sprintln(a...)
	std.mu.Lock()
	defer std.mu.Unlock()
	fmt.Fprint(std.out, s)
	if std.file != nil {
		fmt.Fprint(std.file, ansi.Strip(s))
	}
}

// Printf prints to the screen and the log.
func Printf(format string, args ...any) {
	s := fmt.Sprintf(format, args...)
	std.mu.Lock()
	defer std.mu.Unlock()
	fmt.Fprint(std.out, s)
	if std.file != nil {
		fmt.Fprint(std.file, ansi.Strip(s))
	}
}

// Header prints the command banner.
func Header(title string) { Println(Title.Render("IPAlpha · " + title)) }

// Section prints a step heading.
func Section(title string) { Println("\n" + Bold.Foreground(Accent).Render("▸ "+title)) }

// Done prints a success line.
func Done(msg string) { Println(OK.Render("✔ ") + msg) }

// Info prints a neutral line.
func Info(msg string) { Println(Muted.Render("  " + msg)) }

// Warning prints a warning line.
func Warning(msg string) { Println(Warn.Render("! ") + msg) }

// Fail prints an error line.
func Fail(msg string) { Println(Bad.Render("✖ ") + msg) }

// Item prints "  name  status" aligned.
func Item(name, status string) { Printf("  %-26s %s\n", name, status) }

// Problem is an error with everything a volunteer needs to fix it.
type Problem struct {
	Step  string   // what was being done
	Cause string   // why it failed (plain language)
	Fix   []string // what to do (commands or steps)
	Tail  []string // last lines of the failing output
	Log   string   // full log path
	Err   error
}

func (p *Problem) Error() string {
	if p.Cause != "" {
		return p.Step + ": " + p.Cause
	}
	if p.Err != nil {
		return p.Step + ": " + p.Err.Error()
	}
	return p.Step
}

func (p *Problem) Unwrap() error { return p.Err }

// NewProblem builds a Problem.
func NewProblem(step, cause string, fix ...string) *Problem {
	return &Problem{Step: step, Cause: cause, Fix: fix}
}

// Wrap turns any error into a Problem for a step.
func Wrap(step string, err error, fix ...string) *Problem {
	var p *Problem
	if errors.As(err, &p) {
		if p.Step == "" {
			p.Step = step
		}
		p.Fix = append(p.Fix, fix...)
		return p
	}
	return &Problem{Step: step, Cause: firstLine(err), Err: err, Fix: fix}
}

func firstLine(err error) string {
	if err == nil {
		return ""
	}
	s := strings.TrimSpace(err.Error())
	if i := strings.IndexByte(s, '\n'); i > 0 {
		return s[:i]
	}
	return s
}

// ErrorBox renders a problem as a red box.
func ErrorBox(p *Problem) string {
	w := Width() - 4
	if w > 100 {
		w = 100
	}
	var b strings.Builder
	b.WriteString(Bad.Render("✖ "+p.Step) + "\n")
	if p.Cause != "" {
		b.WriteString(wrap(p.Cause, w-2) + "\n")
	}
	if len(p.Tail) > 0 {
		b.WriteString("\n" + Muted.Render(i18n.T("ui_last_lines")) + "\n")
		for _, l := range p.Tail {
			b.WriteString(Muted.Render("│ ") + ansi.Truncate(ansi.Strip(l), w-4, "…") + "\n")
		}
	}
	if len(p.Fix) > 0 {
		b.WriteString("\n" + Bold.Render(i18n.T("ui_how_to_fix")) + "\n")
		for _, f := range p.Fix {
			b.WriteString("  → " + wrap(f, w-6) + "\n")
		}
	}
	log := p.Log
	if log == "" {
		log = std.Path
	}
	if log != "" {
		b.WriteString("\n" + Muted.Render(i18n.T("ui_full_log")+" "+log) + "\n")
	}
	b.WriteString(Muted.Render(i18n.T("ui_doctor_hint")))
	return errorBx.Width(w).Render(b.String())
}

// WarnBox renders a yellow box.
func WarnBox(title string, lines ...string) string {
	return box(warnBx, Warn.Bold(true).Render("! "+title), lines)
}

// InfoBox renders a blue box.
func InfoBox(title string, lines ...string) string {
	return box(infoBx, Bold.Foreground(Accent).Render(title), lines)
}

func box(style lipgloss.Style, title string, lines []string) string {
	w := Width() - 4
	if w > 110 {
		w = 110
	}
	// Grow to fit the longest line (paths and commands must stay copyable on one line).
	need := lipgloss.Width(title) + 4
	for _, l := range lines {
		if n := lipgloss.Width(l) + 4; n > need {
			need = n
		}
	}
	if need < w {
		w = need
	}
	if max := Width() - 2; need > w && need <= max {
		w = need
	}
	var b strings.Builder
	b.WriteString(title)
	for _, l := range lines {
		b.WriteString("\n" + wrap(l, w-4))
	}
	return style.Width(w).Render(b.String())
}

func wrap(s string, w int) string {
	if w < 20 {
		w = 20
	}
	return ansi.Wordwrap(s, w, " ")
}

// Report prints an error (as a box when it is a Problem) and logs it.
func Report(err error) {
	if err == nil {
		return
	}
	var p *Problem
	if !errors.As(err, &p) {
		p = &Problem{Step: i18n.T("ui_unexpected"), Cause: err.Error()}
	}
	Raw("ERROR: %s", p.Error())
	for _, l := range p.Tail {
		Raw("  | %s", l)
	}
	fmt.Fprintln(os.Stderr, ErrorBox(p))
}

// Tail returns the last n non-empty lines of a text.
func Tail(text string, n int) []string {
	var lines []string
	for _, l := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n") {
		if strings.TrimSpace(l) != "" {
			lines = append(lines, l)
		}
	}
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return lines
}

// TailFile returns the last n lines of a file.
func TailFile(path string, n int) []string {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	if len(data) > 256*1024 {
		data = data[len(data)-256*1024:]
	}
	return Tail(string(data), n)
}

// Spinner shows an animated line while fn runs (plain text when not interactive).
func Spinner(label string, fn func() error) error {
	if !Interactive() {
		Info(label + "…")
		return fn()
	}
	frames := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
	done := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		t := time.NewTicker(90 * time.Millisecond)
		defer t.Stop()
		for i := 0; ; i++ {
			select {
			case <-done:
				fmt.Fprint(os.Stdout, "\r\x1b[2K")
				return
			case <-t.C:
				fmt.Fprintf(os.Stdout, "\r\x1b[2K%s %s", lipgloss.NewStyle().Foreground(Accent).Render(frames[i%len(frames)]), label)
			}
		}
	}()
	err := fn()
	close(done)
	wg.Wait()
	Raw("%s: %v", label, err)
	return err
}
