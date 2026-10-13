package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// Browsers frequently report no type at all for HEIC and fall back to
// application/octet-stream, so the upload key's extension has to be enough.
func TestUploadAcceptsOctetStreamForKnownExtension(t *testing.T) {
	app := newTestApp(t)
	recorder := httptest.NewRecorder()
	request := adminRequest(t, app, http.MethodPut, "/api/photos/upload?key=photos/IMG_0001.heic", []byte("heic payload"))
	request.Header.Set("Content-Type", "application/octet-stream")
	app.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected the heic upload to be accepted, got %d: %s", recorder.Code, recorder.Body.String())
	}
}

// The extension fallback must not become a way around the whitelist.
func TestUploadRejectsOctetStreamForUnknownExtension(t *testing.T) {
	app := newTestApp(t)
	recorder := httptest.NewRecorder()
	request := adminRequest(t, app, http.MethodPut, "/api/photos/upload?key=photos/archive.zip", []byte("zip payload"))
	request.Header.Set("Content-Type", "application/octet-stream")
	app.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("expected unsupported media type, got %d: %s", recorder.Code, recorder.Body.String())
	}
}

// A type the client did report is still authoritative, even if it disagrees
// with the extension.
func TestUploadRejectsMismatchedDeclaredType(t *testing.T) {
	app := newTestApp(t)
	recorder := httptest.NewRecorder()
	request := adminRequest(t, app, http.MethodPut, "/api/photos/upload?key=photos/IMG_0002.heic", []byte("payload"))
	request.Header.Set("Content-Type", "application/x-msdownload")
	app.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("expected unsupported media type, got %d: %s", recorder.Code, recorder.Body.String())
	}
}

// The -sequence variants belong to the same registered HEIF family, and the
// dashboard lets burst uploads through by extension, so the declared type must
// not be the one thing that rejects them.
func TestUploadAcceptsHeifSequenceTypes(t *testing.T) {
	for _, contentType := range []string{"image/heic-sequence", "image/heif-sequence"} {
		if !uploadMIMEAllowed(contentType, "photos/burst.heic") {
			t.Fatalf("expected %s to be accepted", contentType)
		}
	}
}
