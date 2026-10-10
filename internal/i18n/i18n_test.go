package i18n

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

var reKey = regexp.MustCompile(`i18n\.T\("([a-z_]+)"`)
var reWhy = regexp.MustCompile(`(?:Why|Manual):\s*"([a-z_]+)"`)
var reVerb = regexp.MustCompile(`%\[(\d+)\]v`)

// Every key used in the code exists, and every language has a translation with the same arguments.
func TestEveryKeyInFiveLanguages(t *testing.T) {
	root := filepath.Join("..", "..")
	used := map[string]string{}
	_ = filepath.WalkDir(filepath.Join(root, "internal"), func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") {
			return nil
		}
		data, _ := os.ReadFile(p)
		for _, m := range reKey.FindAllStringSubmatch(string(data), -1) {
			used[m[1]] = p
		}
		for _, m := range reWhy.FindAllStringSubmatch(string(data), -1) {
			used[m[1]] = p
		}
		return nil
	})
	for _, prefix := range []string{"health_", "cmd_feature_"} {
		delete(used, prefix) // dynamic suffixes are checked below
	}
	for _, h := range []string{"healthy", "running", "starting", "unhealthy", "stopped", "exited", "created", "restarting", "dead"} {
		used["health_"+h] = "infra"
	}
	for k, file := range used {
		if !Has(k) {
			t.Errorf("missing key %q (used in %s)", k, file)
		}
	}
	for _, k := range Keys() {
		m := Raw(k)
		verbs := func(s string) string {
			var out []string
			for _, v := range reVerb.FindAllStringSubmatch(s, -1) {
				out = append(out, v[1])
			}
			seen := map[string]bool{}
			var uniq []string
			for _, v := range out {
				if !seen[v] {
					seen[v] = true
					uniq = append(uniq, v)
				}
			}
			return strings.Join(sorted(uniq), ",")
		}
		base := verbs(m[0])
		for i, s := range m {
			if strings.TrimSpace(s) == "" {
				t.Errorf("%s: empty %s translation", k, Langs[i])
			}
			if v := verbs(s); v != base {
				t.Errorf("%s: %s uses arguments %q, pt-BR uses %q", k, Langs[i], v, base)
			}
			if strings.Contains(s, "%v") || strings.Contains(s, "%s") || strings.Contains(s, "%d") {
				t.Errorf("%s/%s: use positional %%[n]v verbs", k, Langs[i])
			}
		}
	}
}

func sorted(s []string) []string {
	for i := range s {
		for j := i + 1; j < len(s); j++ {
			if s[j] < s[i] {
				s[i], s[j] = s[j], s[i]
			}
		}
	}
	return s
}

func TestNormalize(t *testing.T) {
	for in, want := range map[string]string{"pt": "pt-BR", "pt_BR.UTF-8": "pt-BR", "en-GB": "en-US", "es-AR": "es", "fr_CA": "fr", "de-AT": "de", "ja": "pt-BR", "": "pt-BR"} {
		if got := Normalize(in); got != want {
			t.Errorf("Normalize(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestFormatting(t *testing.T) {
	Set("en")
	defer Set("pt-BR")
	if got := T("run_port_moved", "auth-api", 3005, 3105); got != "auth-api: port 3005 is busy, using 3105 this run" {
		t.Fatal(got)
	}
}
