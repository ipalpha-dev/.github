package ports

import (
	"testing"

	"github.com/ipalpha-dev/tooling/internal/catalog"
)

func defaults() map[string]int { return catalog.DefaultPorts() }

func TestResolveKeepsFreePorts(t *testing.T) {
	plan := Resolve(defaults(), catalog.PortKeys(), Probe{Busy: func(int) bool { return false }})
	if len(plan.Moves) != 0 {
		t.Fatalf("no port should move: %+v", plan.Moves)
	}
	if plan.Port("auth-api") != 3005 || plan.Port("mongo") != 27017 {
		t.Fatal("defaults not kept")
	}
}

func TestResolveMovesBusyPortsAvoidingDefaults(t *testing.T) {
	busy := map[int]bool{3005: true, 3006: true, 27017: true, 5110: true, 5111: true}
	plan := Resolve(defaults(), catalog.PortKeys(), Probe{Busy: func(p int) bool { return busy[p] }, Holder: func(int) string { return "other" }})
	seen := map[int]string{}
	for k, p := range plan.Actual {
		if other, dup := seen[p]; dup {
			t.Fatalf("%s and %s share port %d", k, other, p)
		}
		seen[p] = k
		if busy[p] {
			t.Fatalf("%s got a busy port %d", k, p)
		}
	}
	if plan.Port("auth-api") == 3005 || catalog.IsDefaultPort(plan.Port("auth-api")) {
		t.Fatalf("auth-api must move to a non-default port, got %d", plan.Port("auth-api"))
	}
	if plan.Port("forms-api") == 3006 {
		t.Fatal("forms-api must move")
	}
	if len(plan.Moves) != 5 || plan.Moves[0].Holder != "other" {
		t.Fatalf("moves: %+v", plan.Moves)
	}
}

func TestOwnedInfraPortIsKept(t *testing.T) {
	plan := Resolve(defaults(), catalog.PortKeys(), Probe{
		Busy: func(p int) bool { return p == 27017 },
		Ours: func(k string, p int) bool { return k == "mongo" && p == 27017 },
	})
	if plan.Port("mongo") != 27017 {
		t.Fatal("our own container's port must be reused")
	}
}

func TestMovedInfraPortStaysPut(t *testing.T) {
	plan := Resolve(defaults(), catalog.PortKeys(), Probe{
		Busy:    func(p int) bool { return p == 27017 || p == 27018 || p == 27019 },
		Current: func(k string) int { return map[string]int{"mongo": 27019}[k] },
	})
	if plan.Port("mongo") != 27019 {
		t.Fatalf("mongo must keep its current container port, got %d", plan.Port("mongo"))
	}
}

func TestRewriter(t *testing.T) {
	pref := defaults()
	pref["persons-api"] = 3102 // a feature workspace prefers +100
	plan := Plan{Actual: map[string]int{}, Preferred: pref}
	for k, v := range pref {
		plan.Actual[k] = v
	}
	plan.Actual["auth-api"] = 3205 // moved this run
	plan.Actual["mongo"] = 27117
	rw := NewRewriter(plan)
	env := map[string]string{
		"PORT":            "3005",
		"AUTH_API_URL":    "http://127.0.0.1:3005",
		"ISSUER":          "http://localhost:3005",
		"MONGO_URI":       "mongodb://ipalpha:ipalpha@127.0.0.1:27017/auth?authSource=admin",
		"PERSONS_API_URL": "http://127.0.0.1:3002",
		"REMOTE":          "https://auth.example.org:3005/x",
		"ORIGINS":         "http://localhost:5110,http://localhost:5106",
	}
	rw.Env(env)
	want := map[string]string{
		"PORT":            "3205",
		"AUTH_API_URL":    "http://127.0.0.1:3205",
		"ISSUER":          "http://localhost:3205",
		"MONGO_URI":       "mongodb://ipalpha:ipalpha@127.0.0.1:27117/auth?authSource=admin",
		"PERSONS_API_URL": "http://127.0.0.1:3102",
		"REMOTE":          "https://auth.example.org:3005/x",
		"ORIGINS":         "http://localhost:5110,http://localhost:5106",
	}
	for k, v := range want {
		if env[k] != v {
			t.Errorf("%s = %q, want %q", k, env[k], v)
		}
	}
}
