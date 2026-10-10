package run

import (
	"strings"
	"testing"
)

func TestSanitize(t *testing.T) {
	cases := map[string]string{
		"\x1bc\x1b[2J\x1b[3J\x1b[H12:05 - Starting compilation": "12:05 - Starting compilation",
		"\x1b[32m[Nest]\x1b[39m ok":                             "\x1b[32m[Nest]\x1b[39m ok",
		"downloading 10%\rdownloading 100%":                     "downloading 100%",
		"\x1b]0;title\x07text":                                  "text",
		"a\tb\x00c":                                             "a    bc",
		"\x1b[1A\x1b[2Kline":                                    "line",
	}
	for in, want := range cases {
		if got := Sanitize(in); got != want {
			t.Errorf("Sanitize(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestDiagnose(t *testing.T) {
	spec := &Spec{ID: "auth-api"}
	cases := []struct {
		lines []string
		code  int
		want  string
		port  int
	}{
		{[]string{"Error: listen EADDRINUSE: address already in use :::3005"}, 1, "3005", 3005},
		{[]string{"Error: Cannot find module '@ipalpha/shared-js'"}, 1, "@ipalpha/shared-js", 0},
		{[]string{"MongoServerError: Authentication failed."}, 1, "MongoDB", 0},
		{[]string{"Error: missing required env var(s): APP_LINKS_OIKOS_REPOSITORIES"}, 1, "APP_LINKS_OIKOS_REPOSITORIES", 0},
		{[]string{"src/a.ts(3,1): error TS2304: Cannot find name 'x'."}, 2, "TypeScript", 0},
		{[]string{"connect ECONNREFUSED 127.0.0.1:27017"}, 1, "27017", 0},
		{[]string{"bye"}, 7, "7", 0},
	}
	for _, c := range cases {
		d := Diagnose(c.lines, c.code, spec)
		if !strings.Contains(d.Summary, c.want) || d.Fix == "" || d.Port != c.port || len(d.Lines) == 0 {
			t.Errorf("%v → %+v", c.lines, d)
		}
	}
}
