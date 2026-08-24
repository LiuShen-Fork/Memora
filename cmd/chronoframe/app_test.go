package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func newTestApp(t *testing.T) *App {
	t.Helper()
	dataDir := t.TempDir()
	cfg := Config{
		DBPath:       filepath.Join(dataDir, "app.sqlite3"),
		DataDir:      dataDir,
		LocalPath:    filepath.Join(dataDir, "storage"),
		LocalBaseURL: "/storage",
		LocalPrefix:  "photos/",
		SessionKey:   []byte("test-session-key"),
		WorkerCount:  1,
	}
	db, err := openDatabase(cfg.DBPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	app := &App{
		cfg:       cfg,
		db:        db,
		queueWake: make(chan struct{}, 1),
		stop:      make(chan struct{}),
		logs:      NewLogBuffer(),
	}
	if err := app.ensureSchema(); err != nil {
		t.Fatal(err)
	}
	if err := app.ensureDefaultSettings(); err != nil {
		t.Fatal(err)
	}
	app.storage = &LocalStorage{base: cfg.LocalPath, baseURL: cfg.LocalBaseURL, prefix: cfg.LocalPrefix}
	return app
}

func adminRequest(t *testing.T, app *App, method, target string, body []byte) *http.Request {
	t.Helper()
	if _, err := app.db.Exec(`INSERT INTO users(name,email,password,created_at,is_admin) VALUES('admin','admin@example.com','password',unixepoch(),1) ON CONFLICT(email) DO NOTHING`); err != nil {
		t.Fatal(err)
	}
	cookieRecorder := httptest.NewRecorder()
	app.setSession(cookieRecorder, 1)
	request := httptest.NewRequest(method, target, bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.AddCookie(cookieRecorder.Result().Cookies()[0])
	return request
}

func TestMediaStreamingAndReactionContracts(t *testing.T) {
	app := newTestApp(t)
	key := "photos/blob.bin"
	payload := []byte("0123456789")
	if _, err := app.storage.Create(context.Background(), key, payload, "application/octet-stream"); err != nil {
		t.Fatal(err)
	}
	if _, err := app.db.Exec(`INSERT INTO photos(id,title,storage_key) VALUES('photo-stream','Stream',?)`, key); err != nil {
		t.Fatal(err)
	}
	media := httptest.NewRecorder()
	app.ServeHTTP(media, httptest.NewRequest(http.MethodGet, "/image/"+key, nil))
	if media.Code != http.StatusOK || media.Body.String() != string(payload) {
		t.Fatalf("streaming image failed: %d %q", media.Code, media.Body.String())
	}
	rangeRequest := httptest.NewRequest(http.MethodGet, "/image/"+key, nil)
	rangeRequest.Header.Set("Range", "bytes=2-5")
	ranged := httptest.NewRecorder()
	app.ServeHTTP(ranged, rangeRequest)
	if ranged.Code != http.StatusPartialContent || ranged.Body.String() != "2345" {
		t.Fatalf("image range failed: %d %q", ranged.Code, ranged.Body.String())
	}

	post := httptest.NewRecorder()
	postRequest := httptest.NewRequest(http.MethodPost, "/api/photos/photo-stream/reactions", strings.NewReader(`{"reactionType":"love"}`))
	postRequest.Header.Set("Content-Type", "application/json")
	app.ServeHTTP(post, postRequest)
	if post.Code != http.StatusOK {
		t.Fatalf("reaction post failed: %d %s", post.Code, post.Body.String())
	}
	get := httptest.NewRecorder()
	app.ServeHTTP(get, httptest.NewRequest(http.MethodGet, "/api/photos/photo-stream/reactions", nil))
	if get.Code != http.StatusOK || !strings.Contains(get.Body.String(), `"love":1`) || !strings.Contains(get.Body.String(), `"userReaction":"love"`) {
		t.Fatalf("reaction get failed: %d %s", get.Code, get.Body.String())
	}
}

func TestQueueResponseContracts(t *testing.T) {
	app := newTestApp(t)
	if _, err := app.db.Exec(`INSERT INTO pipeline_queue(payload,status,created_at) VALUES('{"type":"photo"}','failed',unixepoch())`); err != nil {
		t.Fatal(err)
	}
	list := httptest.NewRecorder()
	app.ServeHTTP(list, adminRequest(t, app, http.MethodGet, "/api/queue/task/list?status=failed&type=photo", nil))
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), `"data"`) || !strings.Contains(list.Body.String(), `"status":"failed"`) {
		t.Fatalf("queue list failed: %d %s", list.Code, list.Body.String())
	}
	retry := httptest.NewRecorder()
	app.ServeHTTP(retry, adminRequest(t, app, http.MethodPost, "/api/queue/task/retry-batch", []byte(`{"retryAll":true}`)))
	if retry.Code != http.StatusOK || !strings.Contains(retry.Body.String(), `"retriedCount":1`) {
		t.Fatalf("queue retry-all failed: %d %s", retry.Code, retry.Body.String())
	}
}

func TestExifReindexAndLivePhotoDetection(t *testing.T) {
	app := newTestApp(t)
	imageKey := "photos/sample.jpg"
	if _, err := app.storage.Create(context.Background(), imageKey, []byte("not-a-jpeg"), "image/jpeg"); err != nil {
		t.Fatal(err)
	}
	if _, err := app.db.Exec(`INSERT INTO photos(id,title,storage_key,is_live_photo) VALUES('photo-1','Old title',?,0)`, imageKey); err != nil {
		t.Fatal(err)
	}
	reindex := httptest.NewRecorder()
	app.ServeHTTP(reindex, adminRequest(t, app, http.MethodPost, "/api/photos/exif/reindex", []byte(`{"action":"single-reindex","photoId":"photo-1"}`)))
	if reindex.Code != http.StatusOK || !strings.Contains(reindex.Body.String(), `"success":true`) || !strings.Contains(reindex.Body.String(), `"photoId":"photo-1"`) {
		t.Fatalf("EXIF reindex failed: %d %s", reindex.Code, reindex.Body.String())
	}
	if _, err := app.storage.Create(context.Background(), "photos/sample.mov", []byte("video"), "video/quicktime"); err != nil {
		t.Fatal(err)
	}
	live := httptest.NewRecorder()
	app.ServeHTTP(live, adminRequest(t, app, http.MethodPost, "/api/photos/livephoto/manage", []byte(`{"action":"update-photo","photoId":"photo-1"}`)))
	if live.Code != http.StatusOK || !strings.Contains(live.Body.String(), `"success":true`) {
		t.Fatalf("Live Photo detection failed: %d %s", live.Code, live.Body.String())
	}
	var isLive int
	if err := app.db.QueryRow(`SELECT is_live_photo FROM photos WHERE id='photo-1'`).Scan(&isLive); err != nil || isLive != 1 {
		t.Fatalf("Live Photo state not persisted: %d %v", isLive, err)
	}
}

func TestUploadValidationAndDuplicateContract(t *testing.T) {
	app := newTestApp(t)
	unsupported := httptest.NewRecorder()
	request := adminRequest(t, app, http.MethodPut, "/api/photos/upload?key=photos/unsafe.exe", []byte("payload"))
	request.Header.Set("Content-Type", "application/x-msdownload")
	app.ServeHTTP(unsupported, request)
	if unsupported.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("expected unsupported media type, got %d: %s", unsupported.Code, unsupported.Body.String())
	}

	key := storageKey(app.storage.Prefix(), "existing.jpg")
	if _, err := app.db.Exec(`INSERT INTO photos(id,title) VALUES(?,?)`, safePhotoID(key), "Existing"); err != nil {
		t.Fatal(err)
	}
	body := []byte(`{"fileNames":["existing.jpg"]}`)
	duplicate := httptest.NewRecorder()
	app.ServeHTTP(duplicate, adminRequest(t, app, http.MethodPost, "/api/photos/check-duplicate", body))
	if duplicate.Code != http.StatusOK || !strings.Contains(duplicate.Body.String(), `"duplicatesFound":1`) {
		t.Fatalf("duplicate response failed: %d %s", duplicate.Code, duplicate.Body.String())
	}
}

func TestEnvironmentSettingsMigrationPreservesStoredValues(t *testing.T) {
	app := newTestApp(t)
	t.Setenv("NUXT_PUBLIC_APP_TITLE", "From Environment")
	app.migrateEnvironmentSettings()
	var title any
	if !app.readSetting("app", "title", &title) || title != "From Environment" {
		t.Fatalf("environment setting was not migrated: %#v", title)
	}
	app.setSetting("app", "title", "Stored Value")
	t.Setenv("NUXT_PUBLIC_APP_TITLE", "New Environment Value")
	app.migrateEnvironmentSettings()
	if !app.readSetting("app", "title", &title) || title != "Stored Value" {
		t.Fatalf("stored setting was overwritten: %#v", title)
	}
}

func TestSettingsEndpointContracts(t *testing.T) {
	app := newTestApp(t)
	fields := httptest.NewRecorder()
	app.ServeHTTP(fields, adminRequest(t, app, http.MethodGet, "/api/system/settings/fields?namespace=app", nil))
	if fields.Code != http.StatusOK || !strings.Contains(fields.Body.String(), `"key":"title"`) {
		t.Fatalf("settings fields contract failed: %d %s", fields.Code, fields.Body.String())
	}

	batch := httptest.NewRecorder()
	batchBody := []byte(`{"updates":[{"namespace":"app","key":"title","value":"Updated"}]}`)
	app.ServeHTTP(batch, adminRequest(t, app, http.MethodPut, "/api/system/settings/batch", batchBody))
	if batch.Code != http.StatusOK {
		t.Fatalf("settings batch failed: %d %s", batch.Code, batch.Body.String())
	}
	var title any
	if !app.readSetting("app", "title", &title) || title != "Updated" {
		t.Fatalf("batch update was not persisted: %#v", title)
	}

	provider := httptest.NewRecorder()
	app.ServeHTTP(provider, adminRequest(t, app, http.MethodGet, "/api/system/settings/storage/provider", nil))
	if provider.Code != http.StatusOK || !strings.Contains(provider.Body.String(), `"namespace":"storage"`) {
		t.Fatalf("setting value wrapper failed: %d %s", provider.Code, provider.Body.String())
	}
}

func TestPhotoRelationshipAndReactionContracts(t *testing.T) {
	app := newTestApp(t)
	if _, err := app.db.Exec(`INSERT INTO photos(id,title,is_live_photo) VALUES('photo-1','Test photo',1)`); err != nil {
		t.Fatal(err)
	}
	result, err := app.db.Exec(`INSERT INTO albums(title,created_at,updated_at) VALUES('Summer',unixepoch(),unixepoch())`)
	if err != nil {
		t.Fatal(err)
	}
	albumID, _ := result.LastInsertId()
	if _, err := app.db.Exec(`INSERT INTO album_photos(album_id,photo_id,position) VALUES(?,?,1)`, albumID, "photo-1"); err != nil {
		t.Fatal(err)
	}

	albums := httptest.NewRecorder()
	app.ServeHTTP(albums, httptest.NewRequest(http.MethodGet, "/api/photos/photo-1/albums", nil))
	if albums.Code != http.StatusOK || !strings.Contains(albums.Body.String(), "Summer") {
		t.Fatalf("photo albums contract failed: %d %s", albums.Code, albums.Body.String())
	}

	reactions := httptest.NewRecorder()
	app.ServeHTTP(reactions, httptest.NewRequest(http.MethodGet, "/api/photos/reactions?ids=photo-1", nil))
	if reactions.Code != http.StatusOK || !strings.Contains(reactions.Body.String(), `"sparkle":0`) {
		t.Fatalf("reaction defaults contract failed: %d %s", reactions.Code, reactions.Body.String())
	}

	live := httptest.NewRecorder()
	app.ServeHTTP(live, adminRequest(t, app, http.MethodGet, "/api/photos/photo-1/livephoto", nil))
	if live.Code != http.StatusOK || !strings.Contains(live.Body.String(), `"isLivePhoto":true`) {
		t.Fatalf("live photo contract failed: %d %s", live.Code, live.Body.String())
	}
}

func TestStorageConfigurationCRUD(t *testing.T) {
	app := newTestApp(t)
	createBody, err := json.Marshal(map[string]any{
		"name":     "Local files",
		"provider": "local",
		"config":   map[string]any{"provider": "local", "basePath": app.cfg.LocalPath, "baseUrl": "/storage", "prefix": "photos/"},
	})
	if err != nil {
		t.Fatal(err)
	}
	created := httptest.NewRecorder()
	app.ServeHTTP(created, adminRequest(t, app, http.MethodPost, "/api/system/settings/storage-config", createBody))
	if created.Code != http.StatusCreated {
		t.Fatalf("create failed: %d %s", created.Code, created.Body.String())
	}
	var createResult struct {
		ID int64 `json:"id"`
	}
	if err := json.Unmarshal(created.Body.Bytes(), &createResult); err != nil || createResult.ID == 0 {
		t.Fatalf("invalid create result: %v, %s", err, created.Body.String())
	}

	updateBody := []byte(`{"provider":"local","config":{"prefix":"gallery/"}}`)
	updated := httptest.NewRecorder()
	app.ServeHTTP(updated, adminRequest(t, app, http.MethodPut, "/api/system/settings/storage-config/"+strconv.FormatInt(createResult.ID, 10), updateBody))
	if updated.Code != http.StatusOK {
		t.Fatalf("update failed: %d %s", updated.Code, updated.Body.String())
	}
	config, err := app.findStorageConfig(createResult.ID)
	if err != nil || config.Config["prefix"] != "gallery/" {
		t.Fatalf("stored configuration was not updated: %#v, %v", config, err)
	}

	deleted := httptest.NewRecorder()
	app.ServeHTTP(deleted, adminRequest(t, app, http.MethodDelete, "/api/system/settings/storage-config/"+strconv.FormatInt(createResult.ID, 10), nil))
	if deleted.Code != http.StatusOK {
		t.Fatalf("delete failed: %d %s", deleted.Code, deleted.Body.String())
	}
}

func TestHealth(t *testing.T) {
	app := newTestApp(t)
	recorder := httptest.NewRecorder()
	app.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/health", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", recorder.Code)
	}
	if got := recorder.Body.String(); got != "{\"status\":\"ok\"}\n" {
		t.Fatalf("unexpected response: %s", got)
	}
}

func TestPhotoStatusAndPublicGalleryContracts(t *testing.T) {
	app := newTestApp(t)
	if _, err := app.db.Exec(`INSERT INTO photos(id,title,last_modified) VALUES('recent','Recent','2026-01-01T00:00:00Z')`); err != nil {
		t.Fatal(err)
	}
	public := httptest.NewRecorder()
	app.ServeHTTP(public, httptest.NewRequest(http.MethodGet, "/api/photos", nil))
	if public.Code != http.StatusOK || !strings.Contains(public.Body.String(), `"id":"recent"`) {
		t.Fatalf("public gallery failed: %d %s", public.Code, public.Body.String())
	}
	status := httptest.NewRecorder()
	app.ServeHTTP(status, adminRequest(t, app, http.MethodGet, "/api/photos/status", nil))
	if status.Code != http.StatusOK || !strings.Contains(status.Body.String(), `"recentPhotos"`) || !strings.Contains(status.Body.String(), `"timestamp"`) {
		t.Fatalf("photo status contract failed: %d %s", status.Code, status.Body.String())
	}
	unauthorized := httptest.NewRecorder()
	app.ServeHTTP(unauthorized, httptest.NewRequest(http.MethodPost, "/api/photos", strings.NewReader(`{"fileName":"x.jpg"}`)))
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("prepare upload should require a session: %d %s", unauthorized.Code, unauthorized.Body.String())
	}
}

func TestWizardSchemaAndSubmit(t *testing.T) {
	app := newTestApp(t)

	schema := httptest.NewRecorder()
	app.ServeHTTP(schema, httptest.NewRequest(http.MethodGet, "/api/wizard/schema?namespace=storage", nil))
	if schema.Code != http.StatusOK || !strings.Contains(schema.Body.String(), `"local.basePath"`) {
		t.Fatalf("storage schema was not returned: status=%d body=%s", schema.Code, schema.Body.String())
	}

	payload := map[string]any{
		"admin": map[string]any{"email": "admin@example.com", "password": "password", "username": "admin"},
		"site":  map[string]any{"title": "Test Gallery"},
		"storage": map[string]any{
			"name":   "Local files",
			"config": map[string]any{"provider": "local", "basePath": app.cfg.LocalPath, "baseUrl": "/storage", "prefix": "photos/"},
		},
		"map": map[string]any{"provider": "maplibre", "token": "map-token", "style": ""},
	}
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/wizard/submit", bytes.NewReader(data))
	request.Header.Set("Content-Type", "application/json")
	app.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("wizard submit failed: status=%d body=%s", response.Code, response.Body.String())
	}
	if len(response.Result().Cookies()) != 1 {
		t.Fatal("wizard submit did not create a session")
	}
	var title, provider, firstLaunch any
	if !app.readSetting("app", "title", &title) || title != "Test Gallery" {
		t.Fatalf("unexpected app title: %#v", title)
	}
	if !app.readSetting("storage", "provider", &provider) || provider == nil {
		t.Fatal("active storage provider was not saved")
	}
	if !app.readSetting("system", "firstLaunch", &firstLaunch) || firstLaunch != false {
		t.Fatalf("first launch was not marked complete: %#v", firstLaunch)
	}
}

func TestPublicSettingsExcludeSecrets(t *testing.T) {
	app := newTestApp(t)
	if _, err := app.db.Exec(`UPDATE settings SET value='Public Gallery',is_public=1 WHERE namespace='app' AND key='title'`); err != nil {
		t.Fatal(err)
	}
	if _, err := app.db.Exec(`INSERT INTO settings(namespace,key,type,value,is_public) VALUES ('storage','token','string','secret-token',0)`); err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	app.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/system/settings/all", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("settings request failed: %d", response.Code)
	}
	body := response.Body.String()
	if !strings.Contains(body, "Public Gallery") || strings.Contains(body, "secret-token") {
		t.Fatalf("unexpected public settings response: %s", body)
	}
}

func TestSessionRoundTrip(t *testing.T) {
	app := newTestApp(t)
	if _, err := app.db.Exec(`INSERT INTO users(name,email,password,created_at,is_admin) VALUES('admin','admin@example.com','password',unixepoch(),1)`); err != nil {
		t.Fatal(err)
	}

	recorder := httptest.NewRecorder()
	app.setSession(recorder, 1)
	response := recorder.Result()
	if len(response.Cookies()) != 1 {
		t.Fatalf("expected one session cookie, got %d", len(response.Cookies()))
	}

	request := httptest.NewRequest(http.MethodGet, "/api/profile", nil)
	request.AddCookie(response.Cookies()[0])
	user, ok := app.user(request)
	if !ok || user["email"] != "admin@example.com" {
		t.Fatalf("expected authenticated admin, got %#v (ok=%t)", user, ok)
	}
}

func TestLocalStorageRejectsTraversal(t *testing.T) {
	app := newTestApp(t)
	storage := app.storage.(*LocalStorage)
	if _, err := storage.Create(context.Background(), "../../outside.jpg", []byte("data"), "image/jpeg"); err == nil {
		t.Fatal("expected traversal key to be rejected")
	}
	if _, err := os.Stat(filepath.Join(app.cfg.DataDir, "outside.jpg")); !os.IsNotExist(err) {
		t.Fatalf("unexpected file outside storage root: %v", err)
	}
}

func TestLocalStorageRoundTrip(t *testing.T) {
	app := newTestApp(t)
	storage := app.storage.(*LocalStorage)
	key := "photos/2026/example.jpg"
	payload := []byte{0x00, 0xff, 0x10, 0x20}
	if _, err := storage.Create(context.Background(), key, payload, "image/jpeg"); err != nil {
		t.Fatal(err)
	}
	got, err := storage.Get(context.Background(), key)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(payload) {
		t.Fatalf("got %v, want %v", got, payload)
	}
}
