package main

import (
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// frontendApp answers with a web directory holding the given files.
func frontendApp(t *testing.T, files map[string]string) *App {
	t.Helper()
	dir := t.TempDir()
	for name, content := range files {
		path := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return &App{cfg: Config{WebDir: dir}}
}

// assetRequest asks for one file of the generated frontend.
func assetRequest(t *testing.T, app *App, target string, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(http.MethodGet, target, nil)
	for name, value := range headers {
		request.Header.Set(name, value)
	}
	recorder := httptest.NewRecorder()
	app.ServeHTTP(recorder, request)
	return recorder
}

const bundleBody = "export const answer = 42\n// the bundle is long enough to be worth compressing\n"

func bundle() string {
	return strings.Repeat(bundleBody, 40)
}

func TestTheFrontendGoesOutCompressedAndCacheable(t *testing.T) {
	app := frontendApp(t, map[string]string{"_nuxt/app.js": bundle()})

	compressed := assetRequest(t, app, "/_nuxt/app.js", map[string]string{"Accept-Encoding": "gzip, deflate"})
	if compressed.Code != http.StatusOK {
		t.Fatalf("compressed response is %d", compressed.Code)
	}
	if encoding := compressed.Header().Get("Content-Encoding"); encoding != "gzip" {
		t.Fatalf("response is served with Content-Encoding %q", encoding)
	}
	if cache := compressed.Header().Get("Cache-Control"); !strings.Contains(cache, "immutable") {
		t.Fatalf("build asset is cached with %q", cache)
	}
	if vary := compressed.Header().Get("Vary"); !strings.Contains(vary, "Accept-Encoding") {
		t.Fatalf("response is served with Vary %q", vary)
	}
	reader, err := gzip.NewReader(bytes.NewReader(compressed.Body.Bytes()))
	if err != nil {
		t.Fatalf("body is not gzip: %v", err)
	}
	decoded, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	if string(decoded) != bundle() {
		t.Fatalf("decompressed body differs from the file (%d bytes)", len(decoded))
	}

	plain := assetRequest(t, app, "/_nuxt/app.js", nil)
	if encoding := plain.Header().Get("Content-Encoding"); encoding != "" {
		t.Fatalf("a client without gzip is answered with Content-Encoding %q", encoding)
	}
	if plain.Body.String() != bundle() {
		t.Fatalf("plain body differs from the file (%d bytes)", plain.Body.Len())
	}
}

func TestTheFrontendIsRevalidatedWithAnETag(t *testing.T) {
	app := frontendApp(t, map[string]string{"_nuxt/app.js": bundle()})

	first := assetRequest(t, app, "/_nuxt/app.js", map[string]string{"Accept-Encoding": "gzip"})
	etag := first.Header().Get("ETag")
	if etag == "" {
		t.Fatal("compressed response carries no ETag")
	}
	again := assetRequest(t, app, "/_nuxt/app.js", map[string]string{
		"Accept-Encoding": "gzip",
		"If-None-Match":   etag,
	})
	if again.Code != http.StatusNotModified {
		t.Fatalf("revalidated request is answered %d", again.Code)
	}
	if again.Body.Len() != 0 {
		t.Fatalf("a 304 carries %d bytes", again.Body.Len())
	}
}

func TestTheCompressedAssetFollowsTheFileOnDisk(t *testing.T) {
	app := frontendApp(t, map[string]string{"_nuxt/app.js": bundle()})
	assetRequest(t, app, "/_nuxt/app.js", map[string]string{"Accept-Encoding": "gzip"})

	path := filepath.Join(app.cfg.WebDir, "_nuxt", "app.js")
	updated := strings.Repeat("// a new build\n", 200)
	if err := os.WriteFile(path, []byte(updated), 0o644); err != nil {
		t.Fatal(err)
	}
	later := time.Now().Add(2 * time.Second)
	if err := os.Chtimes(path, later, later); err != nil {
		t.Fatal(err)
	}

	response := assetRequest(t, app, "/_nuxt/app.js", map[string]string{"Accept-Encoding": "gzip"})
	reader, err := gzip.NewReader(bytes.NewReader(response.Body.Bytes()))
	if err != nil {
		t.Fatalf("body is not gzip: %v", err)
	}
	decoded, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	if string(decoded) != updated {
		t.Fatalf("the cached copy did not follow the file (%d bytes)", len(decoded))
	}
}

func TestTheOtherStaticFilesAreCachedForAnHour(t *testing.T) {
	icon := `<svg xmlns="http://www.w3.org/2000/svg"><rect width="32" height="32"/></svg>`
	app := frontendApp(t, map[string]string{"favicon.svg": icon})

	response := assetRequest(t, app, "/favicon.svg", nil)
	if response.Code != http.StatusOK {
		t.Fatalf("icon is answered %d", response.Code)
	}
	if cache := response.Header().Get("Cache-Control"); !strings.Contains(cache, "max-age=3600") {
		t.Fatalf("icon is cached with %q", cache)
	}
	if response.Body.String() != icon {
		t.Fatalf("icon body differs (%q)", response.Body.String())
	}
}
