// Package localdb keeps auth-api's LOCAL database consistent with the workspace. auth-api seeds
// clients and built-in apps insert-if-absent, so a reused Mongo volume keeps old secrets and old
// loopback ports. This package only ever touches loopback MongoDB, system clients and the
// localhost origins/callbacks of built-in apps — never anything else, never a remote database.
package localdb

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"golang.org/x/crypto/argon2"

	"github.com/ipalpha-dev/tooling/internal/env"
)

// Client secrets are hashed the way node's argon2 package does by default (argon2id, m=64MiB, t=3, p=4).
const (
	argonMemory  = 65536
	argonTime    = 3
	argonThreads = 4
	argonKeyLen  = 32
)

// HashSecret returns a PHC argon2id string compatible with node-argon2 verify.
func HashSecret(secret string) string {
	salt := make([]byte, 16)
	_, _ = rand.Read(salt)
	key := argon2.IDKey([]byte(secret), salt, argonTime, argonMemory, argonThreads, argonKeyLen)
	enc := base64.RawStdEncoding
	return fmt.Sprintf("$argon2id$v=19$m=%d,t=%d,p=%d$%s$%s", argonMemory, argonTime, argonThreads, enc.EncodeToString(salt), enc.EncodeToString(key))
}

// VerifySecret checks a secret against a PHC argon2id/argon2i string.
func VerifySecret(phc, secret string) bool {
	parts := strings.Split(phc, "$")
	if len(parts) != 6 || (parts[1] != "argon2id" && parts[1] != "argon2i") {
		return false
	}
	var m, t uint32
	var p uint8
	for _, kv := range strings.Split(parts[3], ",") {
		k, v, _ := strings.Cut(kv, "=")
		n, err := strconv.ParseUint(v, 10, 32)
		if err != nil {
			return false
		}
		switch k {
		case "m":
			m = uint32(n)
		case "t":
			t = uint32(n)
		case "p":
			p = uint8(n)
		}
	}
	enc := base64.RawStdEncoding
	salt, err1 := enc.DecodeString(parts[4])
	want, err2 := enc.DecodeString(parts[5])
	if err1 != nil || err2 != nil || m == 0 || t == 0 || p == 0 {
		return false
	}
	var got []byte
	if parts[1] == "argon2id" {
		got = argon2.IDKey([]byte(secret), salt, t, m, p, uint32(len(want)))
	} else {
		got = argon2.Key([]byte(secret), salt, t, m, p, uint32(len(want)))
	}
	return subtle.ConstantTimeCompare(got, want) == 1
}

// IsLoopbackURI reports a mongodb:// URI whose host is this machine.
func IsLoopbackURI(uri string) bool {
	u, err := url.Parse(uri)
	if err != nil || u.Scheme != "mongodb" {
		return false
	}
	for _, h := range strings.Split(u.Host, ",") {
		host := h
		if i := strings.LastIndex(h, ":"); i > 0 && !strings.HasSuffix(h, "]") {
			host = h[:i]
		}
		host = strings.Trim(host, "[]")
		if host != "localhost" && host != "127.0.0.1" && host != "::1" {
			return false
		}
	}
	return true
}

// Result of a reconcile.
type Result struct {
	Clients   []string // client ids whose secret was re-hashed
	Redirects []string // entry points whose loopback origins/callbacks changed
	Skipped   string   // reason nothing was checked
}

// Desired loopback addresses of one built-in entry point.
type EntryURLs struct {
	ClientID  string
	Origins   []string
	Redirects []string
}

// Reconcile connects to auth's local database and fixes drifted clients and built-in loopback URLs.
func Reconcile(ctx context.Context, uri string, clients []env.SeedClient, external map[string]bool, entries []EntryURLs) (Result, error) {
	var res Result
	if !IsLoopbackURI(uri) {
		res.Skipped = "not a local database"
		return res, nil
	}
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	cli, err := mongo.Connect(options.Client().ApplyURI(uri).SetServerSelectionTimeout(5 * time.Second))
	if err != nil {
		return res, err
	}
	defer cli.Disconnect(context.Background())
	db := cli.Database(dbName(uri))
	if err := reconcileClients(ctx, db.Collection("clients"), clients, external, &res); err != nil {
		return res, err
	}
	if err := reconcileEntries(ctx, db.Collection("apps"), entries, &res); err != nil {
		return res, err
	}
	return res, nil
}

func dbName(uri string) string {
	u, err := url.Parse(uri)
	if err != nil {
		return "auth"
	}
	if name := strings.Trim(u.Path, "/"); name != "" {
		return name
	}
	return "auth"
}

func reconcileClients(ctx context.Context, col *mongo.Collection, clients []env.SeedClient, external map[string]bool, res *Result) error {
	for _, c := range clients {
		var doc clientDoc
		err := col.FindOne(ctx, bson.M{"clientId": c.ClientID, "clientClass": "system"}).Decode(&doc)
		if errors.Is(err, mongo.ErrNoDocuments) {
			continue // auth-api seeds it on boot
		}
		if err != nil {
			return err
		}
		update := clientUpdate(doc, c, time.Now())
		if external[c.ClientID] {
			update = secretUpdate(doc, c, time.Now())
		}
		if update == nil {
			continue
		}
		if _, err = col.UpdateOne(ctx, bson.M{"_id": doc.ID}, update); err != nil {
			return err
		}
		res.Clients = append(res.Clients, c.ClientID)
	}
	return nil
}

// clientDoc is the part of an auth.clients document the reconcile reads.
type clientDoc struct {
	ID            any      `bson:"_id"`
	SecretHash    string   `bson:"secretHash"`
	AllowedScopes []string `bson:"allowedScopes"`
	AppID         string   `bson:"appId"`
}

// clientUpdate is the update that makes a local system client match its SEED_CLIENTS_JSON entry (nil when it
// already does). auth-api seeding is insert-only, so without this a client that exists keeps the secret and
// scopes of its first boot: the secret is re-hashed when it drifted, allowedScopes become exactly the entry's
// scopes (new grants reach existing clients), an app-bound entry's appId is set and an unbound entry's stale appId
// is removed. Local database only.
func clientUpdate(doc clientDoc, c env.SeedClient, now time.Time) bson.M {
	set := bson.M{}
	update := bson.M{}
	if doc.SecretHash == "" || !VerifySecret(doc.SecretHash, c.Secret) {
		set["secretHash"] = HashSecret(c.Secret)
		set["rotatedAt"] = now
		update["$inc"] = bson.M{"credentialVersion": 1}
	}
	want := c.Scopes
	if want == nil {
		want = []string{}
	}
	if !sameSet(doc.AllowedScopes, want) {
		set["allowedScopes"] = want
	}
	if c.AppID != "" && doc.AppID != c.AppID {
		set["appId"] = c.AppID
	}
	// A client that is no longer app-bound (sign-api once Sign became a core tool) loses its stale appId.
	unbind := c.AppID == "" && doc.AppID != ""
	if len(set) == 0 && !unbind {
		return nil
	}
	if len(set) > 0 {
		update["$set"] = set
	}
	if unbind {
		update["$unset"] = bson.M{"appId": ""}
	}
	return update
}

// secretUpdate is clientUpdate for an outside app's client: only the secret is synced. Its appId and scopes
// (login:relay, notifications:send-template...) come from the portal/Oikos registration, never from the seed.
func secretUpdate(doc clientDoc, c env.SeedClient, now time.Time) bson.M {
	if doc.SecretHash != "" && VerifySecret(doc.SecretHash, c.Secret) {
		return nil
	}
	return bson.M{
		"$set": bson.M{"secretHash": HashSecret(c.Secret), "rotatedAt": now},
		"$inc": bson.M{"credentialVersion": 1},
	}
}

// sameSet reports whether a and b hold the same strings (order and duplicates ignored).
func sameSet(a, b []string) bool {
	in := func(list []string) map[string]bool {
		m := map[string]bool{}
		for _, v := range list {
			m[v] = true
		}
		return m
	}
	ma, mb := in(a), in(b)
	if len(ma) != len(mb) {
		return false
	}
	for v := range ma {
		if !mb[v] {
			return false
		}
	}
	return true
}

// isLoopback reports an http URL on localhost/127.0.0.1/[::1].
func isLoopback(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "http" {
		return false
	}
	h := u.Hostname()
	return h == "localhost" || h == "127.0.0.1" || h == "::1"
}

// merge keeps every non-loopback value and replaces the loopback ones with want.
func merge(current []string, want []string) ([]string, bool) {
	var out []string
	for _, v := range current {
		if !isLoopback(v) {
			out = append(out, v)
		}
	}
	out = append(out, want...)
	if len(out) != len(current) {
		return out, true
	}
	for i := range out {
		if out[i] != current[i] {
			return out, true
		}
	}
	return current, false
}

func reconcileEntries(ctx context.Context, col *mongo.Collection, entries []EntryURLs, res *Result) error {
	for _, e := range entries {
		var app struct {
			ID          any      `bson:"_id"`
			Builtin     bool     `bson:"builtin"`
			EntryPoints []bson.M `bson:"entryPoints"`
		}
		err := col.FindOne(ctx, bson.M{"entryPoints.clientId": e.ClientID, "builtin": true}).Decode(&app)
		if errors.Is(err, mongo.ErrNoDocuments) {
			continue // auth-api seeds the built-in app on boot with the .env values
		}
		if err != nil {
			return err
		}
		for i, ep := range app.EntryPoints {
			if ep["clientId"] != e.ClientID {
				continue
			}
			origins, ch1 := merge(stringList(ep["allowedOrigins"]), e.Origins)
			redirects, ch2 := merge(stringList(ep["redirectUris"]), e.Redirects)
			if !ch1 && !ch2 {
				continue
			}
			prefix := fmt.Sprintf("entryPoints.%d.", i)
			if _, err := col.UpdateOne(ctx, bson.M{"_id": app.ID}, bson.M{"$set": bson.M{
				prefix + "allowedOrigins": origins, prefix + "redirectUris": redirects, "updatedAt": time.Now(),
			}}); err != nil {
				return err
			}
			res.Redirects = append(res.Redirects, e.ClientID)
		}
	}
	return nil
}

func stringList(v any) []string {
	var out []string
	switch list := v.(type) {
	case []string:
		return list
	case bson.A:
		for _, x := range list {
			if s, ok := x.(string); ok {
				out = append(out, s)
			}
		}
	case []any:
		for _, x := range list {
			if s, ok := x.(string); ok {
				out = append(out, s)
			}
		}
	}
	return out
}
