package feature

import (
	"testing"

	"github.com/ipalpha-dev/tooling/internal/catalog"
)

// auth-webapp 5100+900 = 6000 (X11): Firefox and Chrome refuse it, so the sign-in popup never opened.
func TestOffsetSkipsBrowserRestrictedPorts(t *testing.T) {
	if !offsetBlocked(900) {
		t.Fatal("offset 900 puts auth-webapp on 6000 and must be skipped")
	}
	if offsetBlocked(100) {
		t.Fatal("offset 100 is fine")
	}
	for k := 1; k <= 60; k++ {
		if offsetBlocked(k * 100) {
			continue
		}
		for _, p := range catalog.DefaultPorts() {
			if catalog.BrowserRestrictedPort(p + k*100) {
				t.Fatalf("offset %d lands on restricted port %d", k*100, p+k*100)
			}
		}
	}
	off, err := portOffset(t.TempDir())
	if err != nil || off != 100 {
		t.Fatalf("first free block = %d, %v", off, err)
	}
}
