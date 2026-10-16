package main

import (
	"context"
	"sort"
	"testing"
)

func insertAccessPhoto(t *testing.T, app *App, id string, values map[string]string) {
	t.Helper()
	column := func(name string) any {
		if value, ok := values[name]; ok {
			return value
		}
		return nil
	}
	_, err := app.db.Exec(`INSERT INTO photos(id,storage_key,thumbnail_key,original_url,thumbnail_url,live_photo_video_key,live_photo_video_url)
		VALUES(?,?,?,?,?,?,?)`,
		id, column("storage_key"), column("thumbnail_key"), column("original_url"),
		column("thumbnail_url"), column("live_photo_video_key"), column("live_photo_video_url"))
	if err != nil {
		t.Fatal(err)
	}
}

func accessKeysFor(t *testing.T, app *App, key string) []string {
	t.Helper()
	ids, err := app.photoIDsForAccessKey(context.Background(), normalizeAccessKey(key))
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(ids)
	return ids
}

func assertKeys(t *testing.T, app *App, key string, want ...string) {
	t.Helper()
	sort.Strings(want)
	got := accessKeysFor(t, app, key)
	if len(got) != len(want) {
		t.Fatalf("keys for %q = %v, want %v", key, got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("keys for %q = %v, want %v", key, got, want)
		}
	}
}

// A media request arrives as whatever the browser was given: the raw storage
// key from a /image/ URL, or the public /storage/ URL that the gallery stored
// as the photo's thumbnail. All of them have to reach the same photo, or the
// access check would either miss a locked photo or refuse a working one.
func TestAccessKeysResolveEverySpellingOfAPhoto(t *testing.T) {
	app := newTestApp(t)
	insertAccessPhoto(t, app, "a", map[string]string{
		"storage_key":          "photos/a.jpg",
		"thumbnail_key":        "photos/thumbnails/a.webp",
		"original_url":         "/storage/photos/a.jpg",
		"thumbnail_url":        "/storage/photos/thumbnails/a.webp",
		"live_photo_video_key": "photos/a.mp4",
		"live_photo_video_url": "/storage/photos/a.mp4",
	})

	for _, key := range []string{
		"photos/a.jpg",
		"/photos/a.jpg",
		"/storage/photos/a.jpg",
		"photos/thumbnails/a.webp",
		"/storage/photos/thumbnails/a.webp",
		"photos/a.mp4",
		"/storage/photos/a.mp4",
	} {
		assertKeys(t, app, key, "a")
	}
	assertKeys(t, app, "photos/other.jpg")
}

// The trigger has to rebuild the photo's keys on update, because a value that
// changed may no longer match anything a client can ask for.
func TestAccessKeysFollowUpdatesAndDeletes(t *testing.T) {
	app := newTestApp(t)
	insertAccessPhoto(t, app, "a", map[string]string{
		"storage_key":   "photos/a.jpg",
		"thumbnail_url": "/storage/photos/thumbnails/a.webp",
	})
	assertKeys(t, app, "/storage/photos/thumbnails/a.webp", "a")

	if _, err := app.db.Exec(`UPDATE photos SET thumbnail_key=?,thumbnail_url=? WHERE id='a'`, "photos/thumbnails/b.webp", "/storage/photos/thumbnails/b.webp"); err != nil {
		t.Fatal(err)
	}
	assertKeys(t, app, "/storage/photos/thumbnails/a.webp")
	assertKeys(t, app, "/storage/photos/thumbnails/b.webp", "a")

	// Clearing a column has to stop that spelling resolving rather than leave a
	// stale row behind.
	if _, err := app.db.Exec(`UPDATE photos SET thumbnail_key=NULL,thumbnail_url=NULL WHERE id='a'`); err != nil {
		t.Fatal(err)
	}
	assertKeys(t, app, "/storage/photos/thumbnails/b.webp")

	if _, err := app.db.Exec(`DELETE FROM photos WHERE id='a'`); err != nil {
		t.Fatal(err)
	}
	assertKeys(t, app, "photos/a.jpg")
}

// Two photos can be reachable through one key, and the access check has to see
// both rather than whichever row happened to come first.
func TestAccessKeysListEveryPhotoBehindOneKey(t *testing.T) {
	app := newTestApp(t)
	insertAccessPhoto(t, app, "a", map[string]string{"storage_key": "photos/shared.jpg"})
	insertAccessPhoto(t, app, "b", map[string]string{"storage_key": "photos/shared.jpg"})
	assertKeys(t, app, "photos/shared.jpg", "a", "b")
}

// The triggers only fire on writes, so an install that already held photos gets
// its keys from a backfill. That backfill runs once and is recorded.
func TestAccessKeyBackfillRunsOnceAndStopsReportingWork(t *testing.T) {
	app := newTestApp(t)
	insertAccessPhoto(t, app, "a", map[string]string{"storage_key": "photos/a.jpg"})
	// Put the database back in the state it was in before the backfill existed.
	if _, err := app.db.Exec(`DELETE FROM photo_access_keys`); err != nil {
		t.Fatal(err)
	}
	if _, err := app.db.Exec(`DELETE FROM schema_migrations`); err != nil {
		t.Fatal(err)
	}
	assertKeys(t, app, "photos/a.jpg")

	if err := app.migrateData(); err != nil {
		t.Fatal(err)
	}
	assertKeys(t, app, "photos/a.jpg", "a")

	// A recorded migration is skipped, not re-run, which is what keeps startup
	// from rescanning the whole library on every boot.
	var recorded int
	if err := app.db.QueryRow(`SELECT COUNT(*) FROM schema_migrations WHERE name='photo_access_keys_backfill'`).Scan(&recorded); err != nil {
		t.Fatal(err)
	}
	if recorded != 1 {
		t.Fatalf("recorded migrations = %d, want 1", recorded)
	}
	if _, err := app.db.Exec(`DELETE FROM photo_access_keys`); err != nil {
		t.Fatal(err)
	}
	if err := app.migrateData(); err != nil {
		t.Fatal(err)
	}
	assertKeys(t, app, "photos/a.jpg")
}

func TestNormalizeAccessKey(t *testing.T) {
	cases := map[string]string{
		"photos/a.jpg":           "photos/a.jpg",
		"/photos/a.jpg":          "photos/a.jpg",
		"//photos//a.jpg":        "photos//a.jpg",
		`photos\a.jpg`:           "photos/a.jpg",
		"/storage/photos/a.jpg":  "storage/photos/a.jpg",
		"https://cdn.test/a.jpg": "https://cdn.test/a.jpg",
		"HTTPS://cdn.test/a.jpg": "HTTPS://cdn.test/a.jpg",
		"":                       "",
	}
	for input, want := range cases {
		if got := normalizeAccessKey(input); got != want {
			t.Errorf("normalizeAccessKey(%q) = %q, want %q", input, got, want)
		}
	}
}
