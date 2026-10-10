// Package i18n holds every user-facing string of the tools in the five church languages.
// The language comes from the OS (or IPALPHA_LANG / settings lang=); there is no switcher UI.
package i18n

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

// Langs supported, in message-array order.
var Langs = []string{"pt-BR", "en-US", "es", "fr", "de"}

var current = 0

// Set picks the language by tag ("pt", "pt-BR", "en_US.UTF-8", "es-AR"…); unknown → pt-BR.
func Set(tag string) { current = index(Normalize(tag)) }

// Current language tag.
func Current() string { return Langs[current] }

// Normalize maps any locale string to one of Langs (pt-BR default).
func Normalize(tag string) string {
	t := strings.ToLower(strings.TrimSpace(tag))
	t = strings.SplitN(t, ".", 2)[0]
	t = strings.ReplaceAll(t, "_", "-")
	switch {
	case strings.HasPrefix(t, "pt"):
		return "pt-BR"
	case strings.HasPrefix(t, "en"):
		return "en-US"
	case strings.HasPrefix(t, "es"):
		return "es"
	case strings.HasPrefix(t, "fr"):
		return "fr"
	case strings.HasPrefix(t, "de"):
		return "de"
	}
	return "pt-BR"
}

func index(tag string) int {
	for i, l := range Langs {
		if l == tag {
			return i
		}
	}
	return 0
}

// Detect reads the OS language: IPALPHA_LANG, then the platform's UI language, then LANG/LC_*.
func Detect() string {
	if v := os.Getenv("IPALPHA_LANG"); v != "" {
		return Normalize(v)
	}
	if v := platformLang(); v != "" {
		return Normalize(v)
	}
	for _, k := range []string{"LC_ALL", "LC_MESSAGES", "LANGUAGE", "LANG"} {
		if v := os.Getenv(k); v != "" && v != "C" && v != "POSIX" && !strings.HasPrefix(v, "C.") {
			return Normalize(strings.Split(v, ":")[0])
		}
	}
	return "pt-BR"
}

func platformLang() string {
	switch runtime.GOOS {
	case "darwin":
		out, err := exec.Command("defaults", "read", "-g", "AppleLanguages").Output()
		if err != nil {
			return ""
		}
		for _, line := range strings.Split(string(out), "\n") {
			line = strings.Trim(strings.TrimSpace(line), `",()`)
			if line != "" {
				return line
			}
		}
	case "windows":
		return windowsLang()
	}
	return ""
}

// T returns the message for key in the current language, formatted with args.
func T(key string, args ...any) string {
	m, ok := messages[key]
	if !ok {
		return key
	}
	s := m[current]
	if s == "" {
		s = m[0]
	}
	if len(args) > 0 {
		return fmt.Sprintf(s, args...)
	}
	return s
}

// Keys lists every message key (tests check coverage).
func Keys() []string {
	out := make([]string, 0, len(messages))
	for k := range messages {
		out = append(out, k)
	}
	return out
}

// Has reports whether a key exists.
func Has(key string) bool { _, ok := messages[key]; return ok }

// Raw returns the five translations of a key.
func Raw(key string) [5]string { return messages[key] }

// LangName is the native name of a language tag.
func LangName(tag string) string {
	switch Normalize(tag) {
	case "en-US":
		return "English"
	case "es":
		return "Español"
	case "fr":
		return "Français"
	case "de":
		return "Deutsch"
	}
	return "Português (Brasil)"
}
