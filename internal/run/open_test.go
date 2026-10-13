package run

import (
	"testing"

	"github.com/ipalpha-dev/tooling/internal/workspace"
)

func TestShouldOpenAlwaysIncludesMailpit(t *testing.T) {
	s := &workspace.Settings{}
	s.SetBrowsers([]string{"oikos-webapp"})
	c := &Controller{S: &Session{W: &workspace.Workspace{Settings: s}}}
	cases := map[string]bool{"oikos-webapp": true, "mailpit": true, "sign-webapp": false, "auth-webapp": false}
	for id, want := range cases {
		sp := &Spec{ID: id, URL: "http://127.0.0.1:1/"}
		if got := c.shouldOpen(sp); got != want {
			t.Errorf("%s: got %v want %v", id, got, want)
		}
	}
	if c.shouldOpen(&Spec{ID: "mailpit"}) {
		t.Error("no URL must not open")
	}
}
