package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

// resetUnlockLimits clears the attempt windows, which are process-wide and
// would otherwise carry a block from one test into the next.
func resetUnlockLimits() {
	albumUnlockLimits.Lock()
	defer albumUnlockLimits.Unlock()
	albumUnlockLimits.clients = map[string]albumUnlockAttempts{}
	albumUnlockLimits.albums = map[int64]albumUnlockAttempts{}
}

func anonymous(method, target string, body []byte) *http.Request {
	request := httptest.NewRequest(method, target, bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	return request
}

func itoa(value int64) string { return strconv.FormatInt(value, 10) }

// aLockedAlbum adds one photo and puts it in an album behind a password.
func aLockedAlbum(t *testing.T, app *App, password string) int64 {
	t.Helper()
	for _, object := range []string{"photos/secret.jpg", "photos/thumbnails/secret.webp"} {
		if _, err := app.storage.Create(context.Background(), object, []byte("original bytes"), "image/jpeg"); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := app.db.Exec(`INSERT INTO photos(id,title,storage_key,thumbnail_key,original_url,thumbnail_url) VALUES('secret','Secret','photos/secret.jpg','photos/thumbnails/secret.webp','/storage/photos/secret.jpg','/storage/photos/thumbnails/secret.webp')`); err != nil {
		t.Fatal(err)
	}
	body := `{"title":"Private","photoIds":["secret"]`
	if password != "" {
		body += `,"password":"` + password + `"`
	}
	body += `}`
	response := httptest.NewRecorder()
	app.ServeHTTP(response, adminRequest(t, app, http.MethodPost, "/api/albums", []byte(body)))
	if response.Code != http.StatusOK {
		t.Fatalf("create album status = %d body = %s", response.Code, response.Body.String())
	}
	var created struct {
		ID int64 `json:"id"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	return created.ID
}

func TestAlbumPasswordHashingRejectsOtherPasswords(t *testing.T) {
	hash, err := hashAlbumPassword("correct horse")
	if err != nil {
		t.Fatal(err)
	}
	if !verifyAlbumPassword("correct horse", hash) {
		t.Fatal("stored hash did not verify its own password")
	}
	for _, wrong := range []string{"", "correct horse ", "Correct Horse", "correcthorse"} {
		if verifyAlbumPassword(wrong, hash) {
			t.Fatalf("password %q verified against a different one", wrong)
		}
	}
	// Two albums given the same password must not share a hash, or one hash
	// would confirm a guess about the other.
	other, err := hashAlbumPassword("correct horse")
	if err != nil {
		t.Fatal(err)
	}
	if other == hash {
		t.Fatal("the same password produced the same hash twice")
	}
	for _, broken := range []string{"", "nosalt:", ":nohash", "salt:nothex", "salt:" + string(make([]byte, 128))} {
		if verifyAlbumPassword("correct horse", broken) {
			t.Fatalf("a malformed hash %q verified", broken)
		}
	}
}

func TestCreatedAlbumNeverReturnsItsPasswordHash(t *testing.T) {
	app := newTestApp(t)
	a := aLockedAlbum(t, app, "hunter2")

	list := httptest.NewRecorder()
	app.ServeHTTP(list, anonymous(http.MethodGet, "/api/albums", nil))
	if body := list.Body.String(); strings.Contains(body, "password_hash") || strings.Contains(body, "passwordHash") {
		t.Fatalf("album list exposed the password hash: %s", body)
	}
	detail := httptest.NewRecorder()
	app.ServeHTTP(detail, adminRequest(t, app, http.MethodGet, "/api/albums/"+itoa(a), nil))
	if body := detail.Body.String(); strings.Contains(body, "password_hash") || strings.Contains(body, "passwordHash") {
		t.Fatalf("album detail exposed the password hash: %s", body)
	}
}

// The whole point of the feature: without the password, neither the album's
// contents nor the files behind them are reachable.
func TestLockedAlbumIsClosedUntilItIsUnlocked(t *testing.T) {
	resetUnlockLimits()
	app := newTestApp(t)
	id := aLockedAlbum(t, app, "hunter2")
	path := "/api/albums/" + itoa(id)

	locked := httptest.NewRecorder()
	app.ServeHTTP(locked, anonymous(http.MethodGet, path, nil))
	if locked.Code != http.StatusForbidden {
		t.Fatalf("locked album status = %d, want 403", locked.Code)
	}
	// The unlock screen names what it is asking about. The album list already
	// shows the title of a locked album, so the refusal may repeat it.
	var refusal struct {
		Locked bool   `json:"locked"`
		Title  string `json:"title"`
	}
	if err := json.Unmarshal(locked.Body.Bytes(), &refusal); err != nil {
		t.Fatal(err)
	}
	if !refusal.Locked || refusal.Title != "Private" {
		t.Fatalf("locked album refusal = %+v", refusal)
	}

	for _, key := range []string{"photos/secret.jpg", "photos/thumbnails/secret.webp"} {
		media := httptest.NewRecorder()
		app.ServeHTTP(media, anonymous(http.MethodGet, "/image/"+key, nil))
		if media.Code != http.StatusNotFound {
			t.Fatalf("locked media %s status = %d, want 404", key, media.Code)
		}
		storage := httptest.NewRecorder()
		app.ServeHTTP(storage, anonymous(http.MethodGet, "/storage/"+key, nil))
		if storage.Code != http.StatusNotFound {
			t.Fatalf("locked storage %s status = %d, want 404", key, storage.Code)
		}
	}

	// The public album list may say that the album exists and is protected, but
	// not what is in it.
	list := httptest.NewRecorder()
	app.ServeHTTP(list, anonymous(http.MethodGet, "/api/albums", nil))
	var albums []struct {
		ID          int64    `json:"id"`
		HasPassword bool     `json:"hasPassword"`
		PhotoIDs    []string `json:"photoIds"`
		CoverPhoto  any      `json:"coverPhotoId"`
	}
	if err := json.Unmarshal(list.Body.Bytes(), &albums); err != nil {
		t.Fatal(err)
	}
	if len(albums) != 1 || !albums[0].HasPassword || len(albums[0].PhotoIDs) != 0 || albums[0].CoverPhoto != nil {
		t.Fatalf("public album list leaked a locked album's contents: %+v", albums)
	}

	// A wrong password is refused, and one wrong guess does not unlock anything.
	wrong := httptest.NewRecorder()
	app.ServeHTTP(wrong, anonymous(http.MethodPost, path+"/unlock", []byte(`{"password":"wrong"}`)))
	if wrong.Code != http.StatusUnauthorized {
		t.Fatalf("wrong password status = %d, want 401", wrong.Code)
	}
	if len(wrong.Result().Cookies()) != 0 {
		t.Fatal("a wrong password handed out a grant")
	}

	unlocked := httptest.NewRecorder()
	app.ServeHTTP(unlocked, anonymous(http.MethodPost, path+"/unlock", []byte(`{"password":"hunter2"}`)))
	if unlocked.Code != http.StatusOK {
		t.Fatalf("unlock status = %d body = %s", unlocked.Code, unlocked.Body.String())
	}
	grant := unlocked.Result().Cookies()
	if len(grant) != 1 || grant[0].Name != albumAccessCookie || !grant[0].HttpOnly {
		t.Fatalf("unlock cookie = %+v", grant)
	}

	open := httptest.NewRequest(http.MethodGet, path, nil)
	open.AddCookie(grant[0])
	openRecorder := httptest.NewRecorder()
	app.ServeHTTP(openRecorder, open)
	if openRecorder.Code != http.StatusOK {
		t.Fatalf("unlocked album status = %d body = %s", openRecorder.Code, openRecorder.Body.String())
	}
	var detail struct {
		HasPassword bool `json:"hasPassword"`
		Photos      []struct {
			ID string `json:"id"`
		} `json:"photos"`
	}
	if err := json.Unmarshal(openRecorder.Body.Bytes(), &detail); err != nil {
		t.Fatal(err)
	}
	if !detail.HasPassword || len(detail.Photos) != 1 || detail.Photos[0].ID != "secret" {
		t.Fatalf("unlocked album detail = %+v", detail)
	}

	for _, target := range []string{
		"/image/photos/secret.jpg",
		"/image/photos/thumbnails/secret.webp",
		"/storage/photos/secret.jpg",
		"/thumb/photos/thumbnails/secret.webp",
	} {
		media := httptest.NewRequest(http.MethodGet, target, nil)
		media.AddCookie(grant[0])
		recorder := httptest.NewRecorder()
		app.ServeHTTP(recorder, media)
		if recorder.Code != http.StatusOK {
			t.Fatalf("unlocked media %s status = %d", target, recorder.Code)
		}
		// A photo held back for a password must not be left in a shared cache,
		// or the next visitor behind it would be served without unlocking.
		if cache := recorder.Header().Get("Cache-Control"); cache != "private, no-store" {
			t.Fatalf("unlocked media %s cache-control = %q, want private, no-store", target, cache)
		}
	}
}

// The grant is tied to the password it was given for, so an album whose
// password changes closes again for everyone who had the old one.
func TestChangingThePasswordEndsExistingGrants(t *testing.T) {
	resetUnlockLimits()
	app := newTestApp(t)
	id := aLockedAlbum(t, app, "hunter2")
	path := "/api/albums/" + itoa(id)

	unlocked := httptest.NewRecorder()
	app.ServeHTTP(unlocked, anonymous(http.MethodPost, path+"/unlock", []byte(`{"password":"hunter2"}`)))
	grant := unlocked.Result().Cookies()[0]

	changed := httptest.NewRecorder()
	app.ServeHTTP(changed, adminRequest(t, app, http.MethodPut, path, []byte(`{"password":"something else"}`)))
	if changed.Code != http.StatusOK {
		t.Fatalf("change password status = %d body = %s", changed.Code, changed.Body.String())
	}

	stale := httptest.NewRequest(http.MethodGet, path, nil)
	stale.AddCookie(grant)
	staleRecorder := httptest.NewRecorder()
	app.ServeHTTP(staleRecorder, stale)
	if staleRecorder.Code != http.StatusForbidden {
		t.Fatalf("a grant for the old password still opened the album: %d", staleRecorder.Code)
	}

	// Leaving the password out of an unrelated edit must not remove it.
	renamed := httptest.NewRecorder()
	app.ServeHTTP(renamed, adminRequest(t, app, http.MethodPut, path, []byte(`{"title":"Renamed"}`)))
	if renamed.Code != http.StatusOK || !strings.Contains(renamed.Body.String(), `"hasPassword":true`) {
		t.Fatalf("an unrelated edit changed the password state: %d %s", renamed.Code, renamed.Body.String())
	}

	// An empty password is how the protection is removed.
	cleared := httptest.NewRecorder()
	app.ServeHTTP(cleared, adminRequest(t, app, http.MethodPut, path, []byte(`{"password":""}`)))
	if cleared.Code != http.StatusOK || !strings.Contains(cleared.Body.String(), `"hasPassword":false`) {
		t.Fatalf("clearing the password failed: %d %s", cleared.Code, cleared.Body.String())
	}
	open := httptest.NewRecorder()
	app.ServeHTTP(open, anonymous(http.MethodGet, path, nil))
	if open.Code != http.StatusOK {
		t.Fatalf("album still locked after clearing the password: %d", open.Code)
	}
}

// A hidden album is not a protected one, but neither is public: it stays
// invisible rather than announcing itself.
func TestHiddenAlbumIsNotFoundRatherThanLocked(t *testing.T) {
	app := newTestApp(t)
	response := httptest.NewRecorder()
	app.ServeHTTP(response, adminRequest(t, app, http.MethodPost, "/api/albums", []byte(`{"title":"Hidden","isHidden":true}`)))
	var created struct {
		ID int64 `json:"id"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	detail := httptest.NewRecorder()
	app.ServeHTTP(detail, anonymous(http.MethodGet, "/api/albums/"+itoa(created.ID), nil))
	if detail.Code != http.StatusNotFound {
		t.Fatalf("hidden album status = %d, want 404", detail.Code)
	}
}

// A photo shared with a locked album stays reachable through the album that
// does show it. Adding a photo to a private album must not take it away from
// the public one it is also in, and the media routes have to agree with the
// album list about that.
func TestAPhotoShownByAPublicAlbumStaysPublic(t *testing.T) {
	app := newTestApp(t)
	aSharedPhoto(t, app)
	for _, body := range []string{
		`{"title":"Public","photoIds":["shared"]}`,
		`{"title":"Private","photoIds":["shared"],"password":"hunter2"}`,
	} {
		if id := anAlbum(t, app, body); id == 0 {
			t.Fatal("album was not created")
		}
	}
	var publicID int64
	if err := app.db.QueryRow(`SELECT id FROM albums WHERE title='Public'`).Scan(&publicID); err != nil {
		t.Fatal(err)
	}

	media := httptest.NewRecorder()
	app.ServeHTTP(media, anonymous(http.MethodGet, "/image/photos/shared.jpg", nil))
	if media.Code != http.StatusOK {
		t.Fatalf("shared media status = %d, want 200", media.Code)
	}
	// It is still held back for a password, so it must not sit in a shared
	// cache where the next visitor would be handed it.
	if cache := media.Header().Get("Cache-Control"); cache != "private, no-store" {
		t.Fatalf("shared media cache-control = %q", cache)
	}

	detail := httptest.NewRecorder()
	app.ServeHTTP(detail, anonymous(http.MethodGet, "/api/albums/"+itoa(publicID), nil))
	if detail.Code != http.StatusOK {
		t.Fatalf("public album status = %d", detail.Code)
	}
	var album struct {
		Photos []struct {
			ID string `json:"id"`
		} `json:"photos"`
	}
	if err := json.Unmarshal(detail.Body.Bytes(), &album); err != nil {
		t.Fatal(err)
	}
	if len(album.Photos) != 1 || album.Photos[0].ID != "shared" {
		t.Fatalf("public album = %+v", album.Photos)
	}

	// The album list says the same thing: the public album holds the photo.
	list := httptest.NewRecorder()
	app.ServeHTTP(list, anonymous(http.MethodGet, "/api/albums", nil))
	var albums []struct {
		Title    string   `json:"title"`
		PhotoIDs []string `json:"photoIds"`
	}
	if err := json.Unmarshal(list.Body.Bytes(), &albums); err != nil {
		t.Fatal(err)
	}
	for _, listed := range albums {
		if listed.Title != "Public" {
			continue
		}
		if len(listed.PhotoIDs) != 1 || listed.PhotoIDs[0] != "shared" {
			t.Fatalf("public album in the list = %+v", listed)
		}
	}
}

// A hidden album is the one thing that does take a photo out of reach, however
// many albums on show also hold it.
func TestAPhotoHeldByAHiddenAlbumIsOutOfReach(t *testing.T) {
	app := newTestApp(t)
	aSharedPhoto(t, app)
	anAlbum(t, app, `{"title":"Public","photoIds":["shared"]}`)
	anAlbum(t, app, `{"title":"Hidden","photoIds":["shared"],"isHidden":true}`)
	var publicID int64
	if err := app.db.QueryRow(`SELECT id FROM albums WHERE title='Public'`).Scan(&publicID); err != nil {
		t.Fatal(err)
	}

	media := httptest.NewRecorder()
	app.ServeHTTP(media, anonymous(http.MethodGet, "/image/photos/shared.jpg", nil))
	if media.Code != http.StatusNotFound {
		t.Fatalf("hidden media status = %d, want 404", media.Code)
	}

	detail := httptest.NewRecorder()
	app.ServeHTTP(detail, anonymous(http.MethodGet, "/api/albums/"+itoa(publicID), nil))
	if detail.Code != http.StatusOK {
		t.Fatalf("public album status = %d", detail.Code)
	}
	var album struct {
		Photos []any `json:"photos"`
	}
	if err := json.Unmarshal(detail.Body.Bytes(), &album); err != nil {
		t.Fatal(err)
	}
	if len(album.Photos) != 0 {
		t.Fatalf("public album exposed a photo a hidden album holds: %+v", album.Photos)
	}

	list := httptest.NewRecorder()
	app.ServeHTTP(list, anonymous(http.MethodGet, "/api/albums", nil))
	if strings.Contains(list.Body.String(), `"shared"`) {
		t.Fatalf("album list exposed a photo a hidden album holds: %s", list.Body.String())
	}
}

// aSharedPhoto puts one photo in storage and in the gallery, ready to be put in
// several albums.
func aSharedPhoto(t *testing.T, app *App) {
	t.Helper()
	for _, key := range []string{"photos/shared.jpg", "photos/thumbnails/shared.jpg"} {
		if _, err := app.storage.Create(context.Background(), key, []byte("original bytes"), "image/jpeg"); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := app.db.Exec(`INSERT INTO photos(id,title,storage_key,thumbnail_key,thumbnail_url) VALUES('shared','Shared','photos/shared.jpg','photos/thumbnails/shared.jpg','/storage/photos/thumbnails/shared.jpg')`); err != nil {
		t.Fatal(err)
	}
}

func anAlbum(t *testing.T, app *App, body string) int64 {
	t.Helper()
	response := httptest.NewRecorder()
	app.ServeHTTP(response, adminRequest(t, app, http.MethodPost, "/api/albums", []byte(body)))
	if response.Code != http.StatusOK {
		t.Fatalf("create album status = %d body = %s", response.Code, response.Body.String())
	}
	var created struct {
		ID int64 `json:"id"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	return created.ID
}

// A signed-in administrator is not asked for album passwords.
func TestAdminBypassesAlbumPasswords(t *testing.T) {
	app := newTestApp(t)
	id := aLockedAlbum(t, app, "hunter2")
	detail := httptest.NewRecorder()
	app.ServeHTTP(detail, adminRequest(t, app, http.MethodGet, "/api/albums/"+itoa(id), nil))
	if detail.Code != http.StatusOK {
		t.Fatalf("admin album status = %d", detail.Code)
	}
	media := httptest.NewRecorder()
	app.ServeHTTP(media, adminRequest(t, app, http.MethodGet, "/image/photos/secret.jpg", nil))
	if media.Code != http.StatusOK {
		t.Fatalf("admin media status = %d", media.Code)
	}
	if cache := media.Header().Get("Cache-Control"); cache != mediaCacheControl(false) {
		t.Fatalf("admin media cache-control = %q", cache)
	}
}

func TestUnlockAttemptsAreRateLimited(t *testing.T) {
	resetUnlockLimits()
	app := newTestApp(t)
	id := aLockedAlbum(t, app, "hunter2")
	path := "/api/albums/" + itoa(id) + "/unlock"

	for attempt := 0; attempt < albumUnlockPerClient; attempt++ {
		response := httptest.NewRecorder()
		app.ServeHTTP(response, anonymous(http.MethodPost, path, []byte(`{"password":"wrong"}`)))
		if response.Code != http.StatusUnauthorized {
			t.Fatalf("attempt %d status = %d, want 401", attempt, response.Code)
		}
	}
	blocked := httptest.NewRecorder()
	app.ServeHTTP(blocked, anonymous(http.MethodPost, path, []byte(`{"password":"hunter2"}`)))
	if blocked.Code != http.StatusTooManyRequests {
		t.Fatalf("blocked attempt status = %d, want 429", blocked.Code)
	}
	if len(blocked.Result().Cookies()) != 0 {
		t.Fatal("a blocked attempt handed out a grant")
	}
	resetUnlockLimits()
}

// An album with no password and an album that does not exist are both things
// the unlock endpoint must not confirm.
func TestUnlockOnlySpeaksForLockedAlbums(t *testing.T) {
	resetUnlockLimits()
	app := newTestApp(t)
	response := httptest.NewRecorder()
	app.ServeHTTP(response, adminRequest(t, app, http.MethodPost, "/api/albums", []byte(`{"title":"Open"}`)))
	var created struct {
		ID int64 `json:"id"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	for _, target := range []string{"/api/albums/" + itoa(created.ID) + "/unlock", "/api/albums/9999/unlock"} {
		recorder := httptest.NewRecorder()
		app.ServeHTTP(recorder, anonymous(http.MethodPost, target, []byte(`{"password":"hunter2"}`)))
		if recorder.Code != http.StatusNotFound {
			t.Fatalf("%s status = %d, want 404", target, recorder.Code)
		}
	}
}

// The dashboard warns before an admin relies on a protection the active
// storage provider cannot deliver, so the answer has to come from the server
// that knows which provider is in use.
func TestAlbumProtectionReportsWhatTheStorageCanHide(t *testing.T) {
	app := newTestApp(t)
	anonymousProbe := httptest.NewRecorder()
	app.ServeHTTP(anonymousProbe, anonymous(http.MethodGet, "/api/albums/protection", nil))
	if anonymousProbe.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous protection probe status = %d, want 401", anonymousProbe.Code)
	}
	probe := httptest.NewRecorder()
	app.ServeHTTP(probe, adminRequest(t, app, http.MethodGet, "/api/albums/protection", nil))
	if probe.Code != http.StatusOK || !strings.Contains(probe.Body.String(), `"protectsMedia":true`) {
		t.Fatalf("local storage probe = %d %s", probe.Code, probe.Body.String())
	}
}

func TestAlbumPasswordsBelowTheMinimumAreRejected(t *testing.T) {
	app := newTestApp(t)
	response := httptest.NewRecorder()
	app.ServeHTTP(response, adminRequest(t, app, http.MethodPost, "/api/albums", []byte(`{"title":"Short","password":"12345"}`)))
	if response.Code != http.StatusBadRequest {
		t.Fatalf("short password status = %d, want 400", response.Code)
	}
	response = httptest.NewRecorder()
	app.ServeHTTP(response, adminRequest(t, app, http.MethodPost, "/api/albums", []byte(`{"title":"Long","password":"`+strings.Repeat("x", 129)+`"}`)))
	if response.Code != http.StatusBadRequest {
		t.Fatalf("long password status = %d, want 400", response.Code)
	}
	response = httptest.NewRecorder()
	app.ServeHTTP(response, adminRequest(t, app, http.MethodPost, "/api/albums", []byte(`{"title":"Fine","password":"123456"}`)))
	if response.Code != http.StatusOK {
		t.Fatalf("valid password status = %d body = %s", response.Code, response.Body.String())
	}
}
