package ui

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/charmbracelet/huh"

	"github.com/ipalpha-dev/tooling/internal/i18n"
)

// ErrCancelled is returned when the developer quits a prompt (Esc / Ctrl+C).
var ErrCancelled = errors.New("cancelled")

// Option of a Select prompt.
type Option struct {
	Value string
	Label string
	Hint  string
}

func theme() *huh.Theme {
	t := huh.ThemeBase()
	t.Focused.Title = t.Focused.Title.Foreground(Accent).Bold(true)
	t.Focused.SelectSelector = t.Focused.SelectSelector.Foreground(Accent)
	t.Focused.SelectedOption = t.Focused.SelectedOption.Foreground(Accent)
	t.Focused.Description = t.Focused.Description.Foreground(Grey)
	t.Focused.ErrorMessage = t.Focused.ErrorMessage.Foreground(Red)
	t.Focused.Base = t.Focused.Base.BorderForeground(Blue)
	return t
}

func run(f *huh.Form) error {
	err := f.WithTheme(theme()).WithShowHelp(true).Run()
	if errors.Is(err, huh.ErrUserAborted) {
		return ErrCancelled
	}
	return err
}

// Select asks for one option. Not interactive → the default value.
func Select(title, desc string, opts []Option, def string) (string, error) {
	if !Interactive() {
		if def == "" && len(opts) > 0 {
			def = opts[0].Value
		}
		return def, nil
	}
	value := def
	var options []huh.Option[string]
	for _, o := range opts {
		label := o.Label
		if o.Hint != "" {
			label += Muted.Render("  " + o.Hint)
		}
		options = append(options, huh.NewOption(label, o.Value))
	}
	sel := huh.NewSelect[string]().Title(title).Options(options...).Value(&value)
	if desc != "" {
		sel.Description(desc)
	}
	err := run(huh.NewForm(huh.NewGroup(sel)))
	return value, err
}

// MultiSelect asks for several options (all of def pre-selected).
func MultiSelect(title, desc string, opts []Option, def []string) ([]string, error) {
	if !Interactive() {
		return def, nil
	}
	value := append([]string(nil), def...)
	var options []huh.Option[string]
	for _, o := range opts {
		label := o.Label
		if o.Hint != "" {
			label += Muted.Render("  " + o.Hint)
		}
		options = append(options, huh.NewOption(label, o.Value))
	}
	ms := huh.NewMultiSelect[string]().Title(title).Options(options...).Value(&value)
	if desc != "" {
		ms.Description(desc)
	}
	err := run(huh.NewForm(huh.NewGroup(ms)))
	return value, err
}

// Input asks for a text with validation. Not interactive → def (validated).
func Input(title, desc, def string, validate func(string) error) (string, error) {
	if !Interactive() {
		if validate != nil {
			if err := validate(def); err != nil {
				return "", err
			}
		}
		return def, nil
	}
	value := def
	in := huh.NewInput().Title(title).Value(&value)
	if desc != "" {
		in.Description(desc)
	}
	if validate != nil {
		in.Validate(validate)
	}
	err := run(huh.NewForm(huh.NewGroup(in)))
	return strings.TrimSpace(value), err
}

// Secret asks for a hidden text.
func Secret(title, desc string) (string, error) {
	if !Interactive() {
		return "", nil
	}
	var value string
	in := huh.NewInput().Title(title).EchoMode(huh.EchoModePassword).Value(&value)
	if desc != "" {
		in.Description(desc)
	}
	err := run(huh.NewForm(huh.NewGroup(in)))
	return strings.TrimSpace(value), err
}

// Confirm asks yes/no. Not interactive → def.
func Confirm(title, desc string, def bool) (bool, error) {
	if !Interactive() {
		return def, nil
	}
	value := def
	c := huh.NewConfirm().Title(title).Affirmative(i18n.T("ui_yes")).Negative(i18n.T("ui_no")).Value(&value)
	if desc != "" {
		c.Description(desc)
	}
	err := run(huh.NewForm(huh.NewGroup(c)))
	return value, err
}

// TypeToConfirm asks the developer to type an exact word (destructive actions).
func TypeToConfirm(prompt, word string) bool {
	fmt.Fprintf(os.Stdout, "%s %s: ", prompt, Bold.Render(word))
	line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
	return strings.TrimSpace(line) == word
}

// ReadLine reads one line from stdin (fallback prompts).
func ReadLine(prompt string) string {
	fmt.Fprint(os.Stdout, prompt)
	line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
	return strings.TrimSpace(line)
}

// Atoi is a lenient integer parse.
func Atoi(s string) int { n, _ := strconv.Atoi(strings.TrimSpace(s)); return n }
