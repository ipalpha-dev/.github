package run

import (
	"reflect"
	"testing"

	"github.com/ipalpha-dev/tooling/internal/workspace"
)

func TestProcessOrderGroupsEachAppBackendWithFrontend(t *testing.T) {
	specs := []*Spec{
		{ID: "mongo", Kind: KindInfra},
		{ID: "forms-api", Kind: KindAPI, Group: "forms"},
		{ID: "ai-api", Kind: KindAPI},
		{ID: "sign-webapp", Kind: KindWeb, Group: "sign"},
		{ID: "oikos-webapp", Kind: KindWeb},
		{ID: "sign-api", Kind: KindAPI, Group: "sign"},
		{ID: "auth-api", Kind: KindAPI},
		{ID: "forms-webapp", Kind: KindWeb, Group: "forms"},
		{ID: "redis", Kind: KindInfra},
	}
	original := append([]*Spec(nil), specs...)
	want := []string{"ai-api", "auth-api", "oikos-webapp", "forms-api", "forms-webapp", "sign-api", "sign-webapp", "mongo", "redis"}
	ordered := OrderSpecs(specs)
	var ids []string
	for _, spec := range ordered {
		ids = append(ids, spec.ID)
	}
	if !reflect.DeepEqual(ids, want) {
		t.Fatalf("process order = %v, want %v", ids, want)
	}
	if !reflect.DeepEqual(specs, original) {
		t.Fatal("sorting mutated the session specs")
	}
	controller := NewController(&Session{W: &workspace.Workspace{Root: t.TempDir(), Settings: workspace.NewSettings()}, Specs: specs})
	defer controller.cancel()
	ids = nil
	for _, proc := range controller.Procs {
		ids = append(ids, proc.Spec.ID)
	}
	if !reflect.DeepEqual(ids, want) {
		t.Fatalf("controller order = %v, want %v", ids, want)
	}
}

func TestProcessOrderHandlesEmptyAndPartialSections(t *testing.T) {
	if len(OrderSpecs(nil)) != 0 {
		t.Fatal("empty process list is not empty")
	}
	for _, specs := range [][]*Spec{
		{{ID: "mongo", Kind: KindInfra}},
		{{ID: "forms-webapp", Kind: KindWeb, Group: "forms"}},
		{{ID: "auth-api", Kind: KindAPI}},
	} {
		if got := OrderSpecs(specs); len(got) != 1 || got[0] != specs[0] {
			t.Fatal("single section changed", got)
		}
	}
}
