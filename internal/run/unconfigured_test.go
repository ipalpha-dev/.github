package run

import (
	"testing"

	"github.com/ipalpha-dev/tooling/internal/catalog"
)

// With sample data off, Services Kids API needs SERVICES_KIDS_PROJECT_ID; ./run reports it instead of crash-looping.
func TestServicesKidsAPINotStartedWithoutProjectWhenSeedOff(t *testing.T) {
	repo, _ := catalog.Get("services-kids-api")
	cases := []struct {
		env  []string
		want string
	}{
		{[]string{"SEED_DATA=", "SERVICES_KIDS_PROJECT_ID="}, "SERVICES_KIDS_PROJECT_ID"},
		{[]string{"SERVICES_KIDS_PROJECT_ID= "}, "SERVICES_KIDS_PROJECT_ID"},
		{[]string{"SEED_DATA=true", "SERVICES_KIDS_PROJECT_ID="}, ""},
		{[]string{"SEED_DATA=", "SERVICES_KIDS_PROJECT_ID=p1"}, ""},
	}
	for _, c := range cases {
		if got := unconfiguredKey(repo, c.env); got != c.want {
			t.Fatalf("%v: got %q, want %q", c.env, got, c.want)
		}
	}
	other, _ := catalog.Get("projects-api")
	if got := unconfiguredKey(other, nil); got != "" {
		t.Fatalf("projects-api: %q", got)
	}
}
