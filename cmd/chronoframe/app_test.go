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
	_, err := app.db.Exec(`INSERT INTO settings(namespace,key,type,value,is_public) VALUES ('app','title','string','Public Gallery',1),('storage','token','string','secret-token',0)`)
	if err != nil {
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
