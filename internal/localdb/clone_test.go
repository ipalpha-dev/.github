package localdb

import (
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func TestPortRemapper(t *testing.T) {
	re := PortRemapper(map[int]int{5115: 5215, 3017: 3117})
	for in, want := range map[string]string{
		"http://localhost:5115":          "http://localhost:5215",
		"http://localhost:5115/cb?x=1":   "http://localhost:5215/cb?x=1",
		"http://127.0.0.1:3017/a":        "http://127.0.0.1:3117/a",
		"http://localhost:9999":          "http://localhost:9999",
		"http://localhost":               "http://localhost",
		"https://kids.example.org:5115/": "https://kids.example.org:5215/",
	} {
		if got := re(in); got != want {
			t.Errorf("%s → %s, want %s", in, got, want)
		}
	}
}

func TestRemapAppMovesLoopbackOnly(t *testing.T) {
	app := bson.M{"slug": "services-kids", "builtin": false, "entryPoints": bson.A{
		bson.M{"id": "ep1", "clientId": "c1", "allowedOrigins": bson.A{"http://localhost:5115", "https://kids.example.org"},
			"redirectUris": bson.A{"http://localhost:5115/cb"}, "approvedAudiences": bson.A{"ipalpha:notifications"}},
	}}
	out := RemapApp(app, PortRemapper(map[int]int{5115: 5215}))
	ep := entryPoints(out)[0]
	o, r := stringList(ep["allowedOrigins"]), stringList(ep["redirectUris"])
	if len(o) != 2 || o[0] != "http://localhost:5215" || o[1] != "https://kids.example.org" || r[0] != "http://localhost:5215/cb" {
		t.Fatalf("%v %v", o, r)
	}
	if a := stringList(ep["approvedAudiences"]); len(a) != 1 || a[0] != "ipalpha:notifications" {
		t.Fatalf("audiences changed: %v", a)
	}
	// The source document is untouched.
	if stringList(entryPoints(app)[0]["allowedOrigins"])[0] != "http://localhost:5115" {
		t.Fatal("source mutated")
	}
}

// An existing feature client is only completed, never rewritten: secret, status and local scopes stay.
func TestClientFillIsAdditive(t *testing.T) {
	base := bson.M{"clientId": "kids", "appId": "app-1", "entryPointId": "ep1", "secretHash": "base-hash",
		"allowedScopes": bson.A{"login:relay", "notifications:send-template"}}
	cur := bson.M{"clientId": "kids", "secretHash": "local-hash", "allowedScopes": bson.A{"notifications:send-template", "local:extra"}}
	set := ClientFill(cur, base)
	if set["appId"] != "app-1" || set["entryPointId"] != "ep1" || set["secretHash"] != nil {
		t.Fatalf("%v", set)
	}
	sc := set["allowedScopes"].([]string)
	if len(sc) != 3 || sc[0] != "notifications:send-template" || sc[1] != "local:extra" || sc[2] != "login:relay" {
		t.Fatalf("scopes %v", sc)
	}
	// A client with its own appId and every scope is left alone (idempotent).
	full := bson.M{"appId": "mine", "entryPointId": "mine", "allowedScopes": bson.A{"login:relay", "notifications:send-template"}}
	if ClientFill(full, base) != nil {
		t.Fatal("complete client changed")
	}
}
