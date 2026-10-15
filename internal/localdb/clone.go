package localdb

import (
	"context"
	"errors"
	"net/url"
	"strconv"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// CloneResult lists what CloneOutsideApps added to the feature's auth database.
type CloneResult struct {
	Apps    []string // slugs of apps inserted
	Clients []string // client ids inserted or completed
	Skipped string   // reason nothing was cloned
}

// CloneOutsideApps copies the registrations of apps outside core (non-builtin `apps`, the clients bound to them and
// their church allowlist rows) from the base stack's auth database into a feature's auth database, so a feature
// stack can run an outside app's sign-in without registering it again by hand. Local only: both URIs must be
// loopback MongoDB. Idempotent and additive: a record already present in the feature database is never
// overwritten (feature-local edits win); an existing client only gains what it lacks (appId, missing scopes).
// remapURL moves a loopback http URL from the base stack's ports to the feature's planned ports.
// Hashes are copied as stored and never printed or returned.
func CloneOutsideApps(ctx context.Context, baseURI, featureURI string, remapURL func(string) string) (CloneResult, error) {
	var res CloneResult
	switch {
	case !IsLoopbackURI(baseURI) || !IsLoopbackURI(featureURI):
		res.Skipped = "not a local database"
		return res, nil
	case baseURI == featureURI:
		res.Skipped = "same database"
		return res, nil
	}
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	src, err := mongo.Connect(options.Client().ApplyURI(baseURI).SetServerSelectionTimeout(4 * time.Second))
	if err != nil {
		return res, err
	}
	defer src.Disconnect(context.Background())
	dst, err := mongo.Connect(options.Client().ApplyURI(featureURI).SetServerSelectionTimeout(4 * time.Second))
	if err != nil {
		return res, err
	}
	defer dst.Disconnect(context.Background())
	sdb, ddb := src.Database(dbName(baseURI)), dst.Database(dbName(featureURI))

	var apps []bson.M
	cur, err := sdb.Collection("apps").Find(ctx, bson.M{"builtin": bson.M{"$ne": true}})
	if err != nil {
		return res, err
	}
	if err := cur.All(ctx, &apps); err != nil {
		return res, err
	}
	var appIDs, entryIDs []any
	for _, a := range apps {
		appIDs = append(appIDs, a["_id"])
		for _, ep := range entryPoints(a) {
			if id, ok := ep["id"]; ok {
				entryIDs = append(entryIDs, id)
			}
		}
	}
	if len(apps) == 0 {
		res.Skipped = "no outside apps in the base database"
		return res, nil
	}

	for _, a := range apps {
		ok, err := insertApp(ctx, ddb.Collection("apps"), a, remapURL)
		if err != nil {
			return res, err
		}
		if ok {
			res.Apps = append(res.Apps, str(a["slug"]))
		}
	}

	filter := bson.M{"$or": bson.A{bson.M{"appId": bson.M{"$in": appIDs}}, bson.M{"entryPointId": bson.M{"$in": entryIDs}}}}
	var clients []bson.M
	cur, err = sdb.Collection("clients").Find(ctx, filter)
	if err != nil {
		return res, err
	}
	if err := cur.All(ctx, &clients); err != nil {
		return res, err
	}
	for _, c := range clients {
		changed, err := cloneClient(ctx, ddb.Collection("clients"), c)
		if err != nil {
			return res, err
		}
		if changed {
			res.Clients = append(res.Clients, str(c["clientId"]))
		}
	}

	var rows []bson.M
	cur, err = sdb.Collection("churchEntryPointAllowlist").Find(ctx, bson.M{"appId": bson.M{"$in": appIDs}})
	if err != nil {
		return res, err
	}
	if err := cur.All(ctx, &rows); err != nil {
		return res, err
	}
	for _, r := range rows {
		if err := insertIfAbsent(ctx, ddb.Collection("churchEntryPointAllowlist"), r, bson.M{"entryPointId": r["entryPointId"]}); err != nil {
			return res, err
		}
	}
	return res, nil
}

func insertApp(ctx context.Context, col *mongo.Collection, app bson.M, remapURL func(string) string) (bool, error) {
	conflicts := bson.A{bson.M{"_id": app["_id"]}, bson.M{"slug": app["slug"]}}
	for _, ep := range entryPoints(app) {
		if id, ok := ep["clientId"]; ok {
			conflicts = append(conflicts, bson.M{"entryPoints.clientId": id})
		}
	}
	n, err := col.CountDocuments(ctx, bson.M{"$or": conflicts})
	if err != nil || n > 0 {
		return false, err
	}
	if _, err := col.InsertOne(ctx, RemapApp(app, remapURL)); err != nil {
		return false, err
	}
	return true, nil
}

// RemapApp returns a copy of an app document whose entry points have their loopback origins and redirect URIs
// moved with remapURL; every other value (remote URLs, hashes, audit) is copied unchanged.
func RemapApp(app bson.M, remapURL func(string) string) bson.M {
	out := bson.M{}
	for k, v := range app {
		out[k] = v
	}
	eps := entryPoints(app)
	if eps == nil {
		return out
	}
	list := make(bson.A, 0, len(eps))
	for _, ep := range eps {
		cp := bson.M{}
		for k, v := range ep {
			cp[k] = v
		}
		for _, key := range []string{"allowedOrigins", "redirectUris"} {
			var mapped []string
			for _, u := range stringList(ep[key]) {
				if isLoopback(u) {
					u = remapURL(u)
				}
				mapped = append(mapped, u)
			}
			if mapped != nil {
				cp[key] = mapped
			}
		}
		list = append(list, cp)
	}
	out["entryPoints"] = list
	return out
}

// cloneClient inserts a missing client; an existing one only gains an appId/entryPointId it lacks and scopes it
// does not have yet. Its secret hash, status and everything else stay as they are.
func cloneClient(ctx context.Context, col *mongo.Collection, base bson.M) (bool, error) {
	var cur bson.M
	err := col.FindOne(ctx, bson.M{"clientId": base["clientId"]}).Decode(&cur)
	if errors.Is(err, mongo.ErrNoDocuments) {
		_, err = col.InsertOne(ctx, base)
		return err == nil, err
	}
	if err != nil {
		return false, err
	}
	set := ClientFill(cur, base)
	if set == nil {
		return false, nil
	}
	_, err = col.UpdateOne(ctx, bson.M{"_id": cur["_id"]}, bson.M{"$set": set})
	return err == nil, err
}

// ClientFill is the additive update that completes an existing feature client from the base one (nil when
// nothing is missing): appId and entryPointId when absent, and base scopes the feature client lacks.
func ClientFill(cur, base bson.M) bson.M {
	set := bson.M{}
	for _, k := range []string{"appId", "entryPointId"} {
		if str(cur[k]) == "" && str(base[k]) != "" {
			set[k] = base[k]
		}
	}
	have := stringList(cur["allowedScopes"])
	merged := append([]string{}, have...)
	for _, s := range stringList(base["allowedScopes"]) {
		if !contains(merged, s) {
			merged = append(merged, s)
		}
	}
	if len(merged) != len(have) {
		set["allowedScopes"] = merged
	}
	if len(set) == 0 {
		return nil
	}
	return set
}

func insertIfAbsent(ctx context.Context, col *mongo.Collection, doc bson.M, key bson.M) error {
	n, err := col.CountDocuments(ctx, bson.M{"$or": bson.A{bson.M{"_id": doc["_id"]}, key}})
	if err != nil || n > 0 {
		return err
	}
	_, err = col.InsertOne(ctx, doc)
	return err
}

func entryPoints(app bson.M) []bson.M {
	var out []bson.M
	switch list := app["entryPoints"].(type) {
	case bson.A:
		for _, x := range list {
			switch ep := x.(type) {
			case bson.M:
				out = append(out, ep)
			case bson.D:
				m := bson.M{}
				for _, e := range ep {
					m[e.Key] = e.Value
				}
				out = append(out, m)
			}
		}
	case []bson.M:
		out = list
	}
	return out
}

func str(v any) string { s, _ := v.(string); return s }

func contains(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

// PortRemapper returns a function that moves the port of a loopback URL: from[basePort] → featurePort.
// Ports not in the map, and URLs that do not parse, are returned unchanged.
func PortRemapper(from map[int]int) func(string) string {
	return func(raw string) string {
		u, err := url.Parse(raw)
		if err != nil || u.Port() == "" {
			return raw
		}
		p, err := strconv.Atoi(u.Port())
		if err != nil {
			return raw
		}
		to, ok := from[p]
		if !ok {
			return raw
		}
		u.Host = u.Hostname() + ":" + strconv.Itoa(to)
		return u.String()
	}
}
