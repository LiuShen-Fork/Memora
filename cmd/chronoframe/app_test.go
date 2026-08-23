package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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
