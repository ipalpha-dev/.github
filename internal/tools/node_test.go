package tools

import "testing"

func TestNodeSupportsPatchedCampaignSanitizer(t *testing.T) {
	for _, tc := range []struct {
		version string
		ok      bool
	}{
		{"20.19.0", false}, {"22.0.0", false}, {"22.11.9", false}, {"22.12.0", true}, {"v22.22.3", true}, {"24.0.0", true}, {"not-a-version", false},
	} {
		if got := nodeSupported(tc.version); got != tc.ok {
			t.Errorf("nodeSupported(%q) = %v, want %v", tc.version, got, tc.ok)
		}
	}
}
