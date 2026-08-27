package main

import (
	"context"
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
