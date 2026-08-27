package main

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestOpenListCreateReaderUsesRawFilePath(t *testing.T) {
	var filePath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut || r.URL.Path != "/api/fs/put" {
			http.Error(w, "unexpected request", http.StatusBadRequest)
			return
		}
		filePath = r.Header.Get("File-Path")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	storage := &OpenListStorage{baseURL: server.URL, root: "photos", token: "token"}
	if _, err := storage.CreateReader(context.Background(), "folder/photo.jpg", strings.NewReader("data"), 4, "image/jpeg"); err != nil {
		t.Fatalf("CreateReader() error = %v", err)
	}
	if filePath != "/photos/folder/photo.jpg" {
		t.Fatalf("File-Path = %q, want raw Unix path", filePath)
	}
}

func TestOpenListCreateReaderRejectsBusinessError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"code":500,"message":"failed get storage"}`)
	}))
	defer server.Close()

	storage := &OpenListStorage{baseURL: server.URL, root: "photos", token: "token"}
	if _, err := storage.CreateReader(context.Background(), "photo.jpg", strings.NewReader("data"), 4, "image/jpeg"); err == nil || !strings.Contains(err.Error(), "provider code 500") {
		t.Fatalf("CreateReader() error = %v, want provider business error", err)
	}
}

func TestOpenListPathEscapesSegments(t *testing.T) {
	if got := openListPath("/photos/folder/a%2Fb c.jpg"); got != "/photos/folder/a%252Fb%20c.jpg" {
		t.Fatalf("openListPath() = %q", got)
	}
}

func TestOpenListOpenFallsBackToRawURL(t *testing.T) {
	const token = "token"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/d/"):
			http.Error(w, "driver failed", http.StatusInternalServerError)
		case r.Method == http.MethodPost && r.URL.Path == "/api/fs/get":
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, "{\"data\":{\"raw_url\":\"/raw/photo.jpg\"}}")
		case r.Method == http.MethodGet && r.URL.Path == "/raw/photo.jpg":
			if r.Header.Get("Authorization") != token {
				http.Error(w, "missing authorization", http.StatusUnauthorized)
				return
			}
			_, _ = io.WriteString(w, "image-data")
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	storage := &OpenListStorage{baseURL: server.URL, root: "photos", token: token, pathField: "path"}
	reader, object, err := storage.Open(context.Background(), "photo.jpg")
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer reader.Close()
	data, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("ReadAll() error = %v", err)
	}
	if string(data) != "image-data" || object.Key != "photos/photo.jpg" {
		t.Fatalf("fallback result = %q, key %q", data, object.Key)
	}
}

func TestOpenListMissingObjectSkipsRawURLFallback(t *testing.T) {
	var getCalls, metaCalls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/d/"):
			getCalls++
			w.WriteHeader(http.StatusBadRequest)
			_, _ = io.WriteString(w, `<error><code>FileNotFound</code><message>file is notfound or is delete!</message></error>`)
		case r.Method == http.MethodPost && r.URL.Path == "/api/fs/get":
			metaCalls++
			w.WriteHeader(http.StatusInternalServerError)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	storage := &OpenListStorage{baseURL: server.URL, root: "photos", token: "token"}
	_, _, err := storage.Open(context.Background(), "missing.jpg")
	if !errors.Is(err, ErrStorageNotFound) {
		t.Fatalf("Open() error = %v, want ErrStorageNotFound", err)
	}
	if getCalls != 1 || metaCalls != 0 {
		t.Fatalf("missing object requests = GET %d, meta %d; want GET 1, meta 0", getCalls, metaCalls)
	}
}

func TestOpenListMissingObjectIsNotRetryable(t *testing.T) {
	app := newTestApp(t)
	result, err := app.db.Exec(`INSERT INTO pipeline_queue(payload,max_attempts,status) VALUES('{}',3,'in-stages')`)
	if err != nil {
		t.Fatal(err)
	}
	id, _ := result.LastInsertId()
	app.failTaskPermanent(id, ErrStorageNotFound.Error())

	var status string
	var attempts int
	if err := app.db.QueryRow(`SELECT status,attempts FROM pipeline_queue WHERE id=?`, id).Scan(&status, &attempts); err != nil {
		t.Fatal(err)
	}
	if status != "failed" || attempts != 3 {
		t.Fatalf("permanent failure = status %q attempts %d, want failed/3", status, attempts)
	}
}

func TestOpenListCustomDownloadEscapesQuery(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("file"); got != "/photos/folder/a b.jpg" {
			t.Errorf("file query = %q", got)
		}
		_, _ = io.WriteString(w, "image-data")
	}))
	defer server.Close()

	storage := &OpenListStorage{baseURL: server.URL, root: "photos", download: "/download", pathField: "file"}
	reader, _, err := storage.Open(context.Background(), "folder/a b.jpg")
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer reader.Close()
}

func TestParseTimeOpenListFormat(t *testing.T) {
	got := parseTime("Aug 27, 2026, 12:40:55\u202fAM +08")
	_, offset := got.Zone()
	if got.IsZero() || got.Hour() != 0 || got.Minute() != 40 || offset != 8*60*60 {
		t.Fatalf("parseTime() = %v", got)
	}
}
