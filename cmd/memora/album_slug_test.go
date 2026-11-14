package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// quoteJSON keeps a title in a test readable while still sending it as JSON.
func quoteJSON(value string) string {
	encoded, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return string(encoded)
}

// createAlbumAsAdmin posts an album and returns the id and slug it was given.
func createAlbumAsAdmin(t *testing.T, app *App, body string) (int64, string) {
	t.Helper()
	response := httptest.NewRecorder()
	app.ServeHTTP(response, adminRequest(t, app, http.MethodPost, "/api/albums", []byte(body)))
	if response.Code != http.StatusOK {
		t.Fatalf("create %s status = %d body = %s", body, response.Code, response.Body.String())
	}
	var album struct {
		ID   int64  `json:"id"`
		Slug string `json:"slug"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &album); err != nil {
		t.Fatal(err)
	}
	return album.ID, album.Slug
}

// updateAlbumAsAdmin sends a PUT and returns the response for the caller to
// assert on.
func updateAlbumAsAdmin(t *testing.T, app *App, target, body string) *httptest.ResponseRecorder {
	t.Helper()
	response := httptest.NewRecorder()
	app.ServeHTTP(response, adminRequest(t, app, http.MethodPut, target, []byte(body)))
	return response
}

// An album's slug is what its address shows, so it has to come from the title
// without losing characters that are not Latin letters: a gallery of Chinese
// titles would otherwise be nothing but album-3, album-4.
func TestAlbumSlugsAreMadeFromTheTitle(t *testing.T) {
	cases := []struct {
		title string
		want  string
	}{
		{"Kyoto 2024", "kyoto-2024"},
		{"  Spring/Festival!! ", "spring-festival"},
		{"2024年旅行", "2024年旅行"},
		{"日本 語", "日本-語"},
		{"Café", "café"},
		{"Kyoto---2024", "kyoto-2024"},
	}
	for _, test := range cases {
		app := newTestApp(t)
		_, slug := createAlbumAsAdmin(t, app, `{"title":`+quoteJSON(test.title)+`}`)
		if slug != test.want {
			t.Fatalf("title %q gave slug %q, want %q", test.title, slug, test.want)
		}
	}
}

// A title with nothing a slug can be made of, and a title that is only a
// number, both fall back to a name built from the album's ID: a slug of digits
// would be read back as an album ID, and an empty one is not an address.
func TestUnusableTitlesFallBackToTheAlbumNumber(t *testing.T) {
	for _, title := range []string{"🎉🎉", "2024", "!!!"} {
		app := newTestApp(t)
		id, slug := createAlbumAsAdmin(t, app, `{"title":`+quoteJSON(title)+`}`)
		want := "album-" + itoa(id)
		if slug != want {
			t.Fatalf("title %q gave slug %q, want %q", title, slug, want)
		}
	}
}

// Two albums may be called the same thing; their addresses may not.
func TestAlbumsWithTheSameTitleGetDifferentSlugs(t *testing.T) {
	app := newTestApp(t)
	_, first := createAlbumAsAdmin(t, app, `{"title":"Summer"}`)
	_, second := createAlbumAsAdmin(t, app, `{"title":"Summer"}`)
	_, third := createAlbumAsAdmin(t, app, `{"title":"summer"}`)
	if first != "summer" || second != "summer-1" || third != "summer-2" {
		t.Fatalf("slugs = %q %q %q, want summer summer-1 summer-2", first, second, third)
	}
}

// A slug the user typed is taken as written, folded to the stored form, and
// refused rather than reworded when it collides.
func TestAChosenSlugIsKeptUnlessItIsTaken(t *testing.T) {
	app := newTestApp(t)
	_, slug := createAlbumAsAdmin(t, app, `{"title":"One","slug":"My-Trip"}`)
	if slug != "my-trip" {
		t.Fatalf("chosen slug = %q, want my-trip", slug)
	}
	response := httptest.NewRecorder()
	app.ServeHTTP(response, adminRequest(t, app, http.MethodPost, "/api/albums", []byte(`{"title":"Two","slug":"MY-TRIP"}`)))
	if response.Code != http.StatusConflict {
		t.Fatalf("conflicting slug status = %d body = %s", response.Code, response.Body.String())
	}
	if body := response.Body.String(); !strings.Contains(body, "in use") {
		t.Fatalf("conflict body = %s", body)
	}
}

// The address is a URL segment, so punctuation and digits-only are refused
// where they are chosen rather than stored and broken later.
func TestUnusableChosenSlugsAreRefused(t *testing.T) {
	app := newTestApp(t)
	for _, slug := range []string{"123", "with space", "dots.here", "slash/here", "-leading", "trailing-", "a--b"} {
		response := httptest.NewRecorder()
		app.ServeHTTP(response, adminRequest(t, app, http.MethodPost, "/api/albums", []byte(`{"title":"Album","slug":`+quoteJSON(slug)+`}`)))
		if response.Code != http.StatusBadRequest {
			t.Fatalf("slug %q status = %d body = %s", slug, response.Code, response.Body.String())
		}
	}
}

// The address an album used to answer to keeps resolving to it, and says so,
// so the page can rewrite the address bar instead of showing a stale one.
func TestARetiredSlugStillFindsTheAlbum(t *testing.T) {
	app := newTestApp(t)
	id, slug := createAlbumAsAdmin(t, app, `{"title":"Summer"}`)
	response := updateAlbumAsAdmin(t, app, "/api/albums/"+itoa(id), `{"slug":"summer-holiday"}`)
	if response.Code != http.StatusOK {
		t.Fatalf("update status = %d body = %s", response.Code, response.Body.String())
	}
	var updated struct {
		Slug string `json:"slug"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &updated); err != nil {
		t.Fatal(err)
	}
	if updated.Slug != "summer-holiday" {
		t.Fatalf("updated slug = %q, want summer-holiday", updated.Slug)
	}
	if resolved, current, canonical, ok := app.resolveAlbumSegment(slug); !ok || resolved != id || canonical || current != "summer-holiday" {
		t.Fatalf("old slug resolved to (%d,%q,%v,%v), want (%d,summer-holiday,false,true)", resolved, current, canonical, ok, id)
	}
	if resolved, current, canonical, ok := app.resolveAlbumSegment("summer-holiday"); !ok || resolved != id || !canonical || current != "summer-holiday" {
		t.Fatalf("new slug resolved to (%d,%q,%v,%v), want (%d,summer-holiday,true,true)", resolved, current, canonical, ok, id)
	}
	detail := httptest.NewRecorder()
	app.ServeHTTP(detail, httptest.NewRequest(http.MethodGet, "/api/albums/"+slug, nil))
	if detail.Code != http.StatusOK {
		t.Fatalf("detail through the retired slug status = %d body = %s", detail.Code, detail.Body.String())
	}
}

// Every address handed out before slugs existed is an album ID, and those have
// to keep working; only an unknown segment is a 404.
func TestAnAlbumKeepsAnsweringToItsNumber(t *testing.T) {
	app := newTestApp(t)
	id, slug := createAlbumAsAdmin(t, app, `{"title":"Summer"}`)
	response := httptest.NewRecorder()
	app.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/albums/"+itoa(id), nil))
	if response.Code != http.StatusOK {
		t.Fatalf("detail by id status = %d body = %s", response.Code, response.Body.String())
	}
	var detail struct {
		ID   int64  `json:"id"`
		Slug string `json:"slug"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &detail); err != nil {
		t.Fatal(err)
	}
	if detail.ID != id || detail.Slug != slug {
		t.Fatalf("detail = %+v, want id %d slug %q", detail, id, slug)
	}
	if _, _, _, ok := app.resolveAlbumSegment("no-such-album"); ok {
		t.Fatal("an unknown slug resolved to an album")
	}
	if _, _, _, ok := app.resolveAlbumSegment("9999"); ok {
		t.Fatal("an unknown album number resolved to an album")
	}
}

// An empty slug in an edit asks for a new one made from the title the album now
// has, which is what the dashboard sends after the title was changed.
func TestAnEmptySlugIsRemadeFromTheNewTitle(t *testing.T) {
	app := newTestApp(t)
	id, slug := createAlbumAsAdmin(t, app, `{"title":"Summer"}`)
	response := updateAlbumAsAdmin(t, app, "/api/albums/"+itoa(id), `{"title":"Autumn Light","slug":""}`)
	if response.Code != http.StatusOK {
		t.Fatalf("update status = %d body = %s", response.Code, response.Body.String())
	}
	var updated struct {
		Slug string `json:"slug"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &updated); err != nil {
		t.Fatal(err)
	}
	if updated.Slug != "autumn-light" {
		t.Fatalf("slug = %q, want autumn-light", updated.Slug)
	}
	if resolved, _, canonical, ok := app.resolveAlbumSegment(slug); !ok || resolved != id || canonical {
		t.Fatalf("retired slug resolved to (%d,%v,%v), want (%d,false,true)", resolved, canonical, ok, id)
	}
}

// Besides the title, an edit that does not mention the slug must not change it.
func TestEditingAnAlbumLeavesItsSlugAlone(t *testing.T) {
	app := newTestApp(t)
	id, slug := createAlbumAsAdmin(t, app, `{"title":"Summer"}`)
	if slug != "summer" {
		t.Fatalf("slug = %q, want summer", slug)
	}
	response := updateAlbumAsAdmin(t, app, "/api/albums/"+itoa(id), `{"description":"A trip"}`)
	if response.Code != http.StatusOK {
		t.Fatalf("update status = %d body = %s", response.Code, response.Body.String())
	}
	var updated struct {
		Slug string `json:"slug"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &updated); err != nil {
		t.Fatal(err)
	}
	if updated.Slug != "summer" {
		t.Fatalf("slug after unrelated edit = %q, want summer", updated.Slug)
	}
}

// An install that predates slugs gets one per album on its first start, once;
// running it again must not renumber anything.
func TestEveryAlbumGetsASlugOnce(t *testing.T) {
	app := newTestApp(t)
	if _, err := app.db.Exec(`DELETE FROM schema_migrations WHERE name='album_slug_backfill'`); err != nil {
		t.Fatal(err)
	}
	if _, err := app.db.Exec(`INSERT INTO albums(title,is_hidden) VALUES('Old Album',0),('Old Album',0)`); err != nil {
		t.Fatal(err)
	}
	if err := app.backfillAlbumSlugs(); err != nil {
		t.Fatal(err)
	}
	slugs := []string{}
	rows, err := app.db.Query(`SELECT COALESCE(slug,'') FROM albums ORDER BY id ASC`)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var slug string
		if err := rows.Scan(&slug); err != nil {
			t.Fatal(err)
		}
		slugs = append(slugs, slug)
	}
	_ = rows.Close()
	if len(slugs) != 2 || slugs[0] != "old-album" || slugs[1] != "old-album-1" {
		t.Fatalf("backfilled slugs = %v, want [old-album old-album-1]", slugs)
	}
	if err := app.backfillAlbumSlugs(); err != nil {
		t.Fatalf("second backfill: %v", err)
	}
	var marker int
	if err := app.db.QueryRow(`SELECT 1 FROM schema_migrations WHERE name='album_slug_backfill'`).Scan(&marker); err != nil {
		t.Fatalf("backfill marker missing: %v", err)
	}
}
