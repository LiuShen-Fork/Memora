package main

import (
	"context"
	"strings"
)

// A photo's media is reachable by a storage key, but the gallery hands the
// browser a public URL for it, and the two spellings differ: a photo whose
// storage key is photos/a.jpg is served from /storage/photos/a.jpg and its
// thumbnail URL is stored in that same served form. Deciding whether a media
// request may proceed therefore starts with "which photo is this?", and asking
// that by scanning photos and comparing six columns is too slow to do on every
// image request.
//
// photo_access_keys is that lookup, as an index: every spelling a client can
// ask for, mapped to the photo it reaches. The triggers below keep it correct
// no matter which code path writes a photo, which matters because a missed
// update would either expose a locked photo or break a working one.

// photoAccessValuesTrigger exposes the six stored media columns of the row
// being written as one column of values, in the shape photoAccessKeyFrom reads.
const photoAccessValuesTrigger = `(SELECT new.id AS id,new.storage_key AS v
			UNION ALL SELECT new.id,new.thumbnail_key
			UNION ALL SELECT new.id,new.original_url
			UNION ALL SELECT new.id,new.thumbnail_url
			UNION ALL SELECT new.id,new.live_photo_video_key
			UNION ALL SELECT new.id,new.live_photo_video_url)`

// photoAccessValuesTable is the same shape over every photo already stored,
// for the backfill that runs once on an install that predates the triggers.
const photoAccessValuesTable = `(SELECT id,storage_key AS v FROM photos
			UNION ALL SELECT id,thumbnail_key FROM photos
			UNION ALL SELECT id,original_url FROM photos
			UNION ALL SELECT id,thumbnail_url FROM photos
			UNION ALL SELECT id,live_photo_video_key FROM photos
			UNION ALL SELECT id,live_photo_video_url FROM photos)`

// photoAccessKeySpellings are the forms of a stored media value that all have
// to resolve to the same photo: the value itself, the value without a leading
// slash, and the storage key underneath a /storage/ or /image/ URL. Absolute
// URLs get no derived form, because a provider that serves them directly is not
// routed through this server and the key would never be looked up.
var photoAccessKeySpellings = []struct{ expression, filter string }{
	{"v", ""},
	{"ltrim(v,'/')", ""},
	{"ltrim(substr(v,10),'/')", " WHERE v LIKE '/storage/%'"},
	{"ltrim(substr(v,8),'/')", " WHERE v LIKE '/image/%'"},
}

// photoAccessKeyFrom records those spellings for whichever photos values
// describes. Both triggers and the backfill build their statement from this, so
// the spellings a newly written photo gets and the spellings an existing one
// was given cannot drift apart.
func photoAccessKeyFrom(values string) string {
	selects := make([]string, 0, len(photoAccessKeySpellings))
	for _, spelling := range photoAccessKeySpellings {
		selects = append(selects, "SELECT id,"+spelling.expression+" AS key FROM ("+values+") AS vals"+spelling.filter)
	}
	return "INSERT OR IGNORE INTO photo_access_keys(access_key,photo_id)\n\t\tSELECT DISTINCT key,id FROM (\n\t\t\t" +
		strings.Join(selects, "\n\t\t\tUNION ALL ") +
		"\n\t\t) WHERE key IS NOT NULL AND key <> ''"
}

// photoAccessKeyBackfill gives the photos that already existed the same keys a
// newly written photo gets. It runs once, recorded in schema_migrations.
var photoAccessKeyBackfill = photoAccessKeyFrom(photoAccessValuesTable)

// photoAccessKeysStatements creates the table, its reverse index and the three
// triggers. They are idempotent, so they belong with the other CREATE
// statements rather than in the column migration list.
var photoAccessKeysStatements = []string{
	`CREATE TABLE IF NOT EXISTS photo_access_keys (access_key TEXT NOT NULL, photo_id TEXT NOT NULL REFERENCES photos(id) ON DELETE CASCADE, PRIMARY KEY(access_key,photo_id))`,
	`CREATE INDEX IF NOT EXISTS photo_access_keys_photo_idx ON photo_access_keys(photo_id)`,
	`CREATE TRIGGER IF NOT EXISTS photos_access_keys_insert AFTER INSERT ON photos BEGIN ` + photoAccessKeyFrom(photoAccessValuesTrigger) + `; END`,
	// An update rebuilds the photo's keys, because a value that changed may no
	// longer match anything and a value that appeared has to start matching.
	`CREATE TRIGGER IF NOT EXISTS photos_access_keys_update AFTER UPDATE OF storage_key,thumbnail_key,original_url,thumbnail_url,live_photo_video_key,live_photo_video_url ON photos BEGIN
		DELETE FROM photo_access_keys WHERE photo_id=new.id;
		` + photoAccessKeyFrom(photoAccessValuesTrigger) + `; END`,
	`CREATE TRIGGER IF NOT EXISTS photos_access_keys_delete AFTER DELETE ON photos BEGIN
		DELETE FROM photo_access_keys WHERE photo_id=old.id; END`,
}

// normalizeAccessKey puts an incoming media path into the spelling the table
// stores. A key that would not survive this unchanged is rejected by the caller
// rather than normalized on its behalf, so a request cannot reach a photo
// through a variant that the storage provider would silently resolve to the
// same object.
func normalizeAccessKey(key string) string {
	if isAbsoluteURL(key) {
		return key
	}
	return strings.TrimLeft(strings.ReplaceAll(key, "\\", "/"), "/")
}

func isAbsoluteURL(value string) bool {
	lower := strings.ToLower(value)
	return strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://")
}

// photoIDsForAccessKey lists the photos a media request can reach. More than
// one photo can share a key, and a photo that belongs to no album is reachable
// by everyone, so the caller decides what to do with the result rather than
// having this return a yes or no.
func (a *App) photoIDsForAccessKey(ctx context.Context, key string) ([]string, error) {
	rows, err := a.db.QueryContext(ctx, `SELECT photo_id FROM photo_access_keys WHERE access_key=?`, key)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}
