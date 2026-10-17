package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

type uploadPlan struct {
	Mode      string   `json:"mode"`
	URL       string   `json:"url"`
	PartSize  int64    `json:"partSize"`
	PartCount int      `json:"partCount"`
	PartURLs  []string `json:"partUrls"`
}

type preparedUpload struct {
	SignedURL string      `json:"signedUrl"`
	FileKey   string      `json:"fileKey"`
	Upload    *uploadPlan `json:"upload"`
}

// prepareAs asks the server how a file of this size should be sent, the way the
// browser does before it starts.
func prepareAs(t *testing.T, app *App, request *http.Request, fileName string, size int64) preparedUpload {
	t.Helper()
	body := fmt.Sprintf(`{"fileName":%q,"contentType":"image/jpeg","skipDuplicateCheck":true,"size":%d}`, fileName, size)
	request.Body = io.NopCloser(bytes.NewReader([]byte(body)))
	request.ContentLength = int64(len(body))
	response := httptest.NewRecorder()
	app.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("preparing an upload = %d, %s", response.Code, response.Body.String())
	}
	var prepared preparedUpload
	if err := json.Unmarshal(response.Body.Bytes(), &prepared); err != nil {
		t.Fatalf("the upload plan is not readable: %v, %s", err, response.Body.String())
	}
	if prepared.Upload == nil {
		t.Fatalf("no upload plan was offered: %s", response.Body.String())
	}
	return prepared
}

func sendPart(t *testing.T, app *App, url string, index int, data []byte) *httptest.ResponseRecorder {
	t.Helper()
	request := adminRequest(t, app, http.MethodPut, fmt.Sprintf("%s?index=%d", url, index), nil)
	request.Body = io.NopCloser(bytes.NewReader(data))
	request.ContentLength = int64(len(data))
	response := httptest.NewRecorder()
	app.ServeHTTP(response, request)
	return response
}

// An upload sent in parts arrives as the same file it would have been in one
// request, whichever order the parts happen to arrive in.
func TestAnUploadSentInPartsArrivesWhole(t *testing.T) {
	app := newTestApp(t)
	size := int64(uploadPartSize) + 1024
	prepared := prepareAs(t, app, adminRequest(t, app, http.MethodPost, "/api/photos", nil), "large.jpg", size)
	if prepared.Upload.Mode != "chunks" {
		t.Fatalf("mode = %q, want a plan this server assembles", prepared.Upload.Mode)
	}
	if prepared.Upload.PartCount != 2 || prepared.Upload.PartSize != uploadPartSize {
		t.Fatalf("plan = %+v, want two parts of %d bytes", prepared.Upload, uploadPartSize)
	}
	first := bytes.Repeat([]byte("a"), uploadPartSize)
	second := bytes.Repeat([]byte("b"), 1024)
	// The second part is sent first, which is what two parts of one upload look
	// like from the outside.
	if response := sendPart(t, app, prepared.Upload.URL, 1, second); response.Code != http.StatusOK {
		t.Fatalf("sending the last part = %d, %s", response.Code, response.Body.String())
	}
	response := httptest.NewRecorder()
	app.ServeHTTP(response, adminRequest(t, app, http.MethodPost, prepared.Upload.URL, nil))
	if response.Code != http.StatusBadRequest {
		t.Fatalf("completing an upload that is missing a part = %d, want it refused", response.Code)
	}
	if response := sendPart(t, app, prepared.Upload.URL, 0, first); response.Code != http.StatusOK {
		t.Fatalf("sending the first part = %d, %s", response.Code, response.Body.String())
	}
	response = httptest.NewRecorder()
	app.ServeHTTP(response, adminRequest(t, app, http.MethodPost, prepared.Upload.URL, nil))
	if response.Code != http.StatusOK {
		t.Fatalf("completing the upload = %d, %s", response.Code, response.Body.String())
	}
	stored, err := app.storage.Get(context.Background(), prepared.FileKey)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(stored, append(append([]byte{}, first...), second...)) {
		t.Fatalf("stored %d bytes, want the %d bytes that were sent", len(stored), size)
	}
	// The temporary file is gone once the file is in storage.
	if entries, err := os.ReadDir(filepath.Join(app.cfg.DataDir, "uploads")); err == nil && len(entries) != 0 {
		t.Fatalf("the upload was left behind: %v", entries)
	}
}

// A part that is the wrong size, out of range or sent twice under different
// content is refused rather than written into the file.
func TestAnUploadRefusesPartsItCannotPlace(t *testing.T) {
	app := newTestApp(t)
	prepared := prepareAs(t, app, adminRequest(t, app, http.MethodPost, "/api/photos", nil), "large.jpg", int64(uploadPartSize)+1024)
	short := bytes.Repeat([]byte("a"), 512)
	if response := sendPart(t, app, prepared.Upload.URL, 0, short); response.Code != http.StatusBadRequest {
		t.Fatalf("a part of the wrong size = %d, want it refused", response.Code)
	}
	if response := sendPart(t, app, prepared.Upload.URL, 9, short); response.Code != http.StatusBadRequest {
		t.Fatalf("a part that is not part of the file = %d, want it refused", response.Code)
	}
	part := bytes.Repeat([]byte("a"), uploadPartSize)
	if response := sendPart(t, app, prepared.Upload.URL, 0, part); response.Code != http.StatusOK {
		t.Fatalf("sending a part = %d, %s", response.Code, response.Body.String())
	}
	// The same part again, unchanged, is what a retry looks like.
	if response := sendPart(t, app, prepared.Upload.URL, 0, part); response.Code != http.StatusOK {
		t.Fatalf("retrying a part = %d, %s", response.Code, response.Body.String())
	}
	different := bytes.Repeat([]byte("c"), uploadPartSize)
	if response := sendPart(t, app, prepared.Upload.URL, 0, different); response.Code != http.StatusConflict {
		t.Fatalf("a different part under the same number = %d, want it refused", response.Code)
	}
}

// An upload that is given up on takes its temporary file with it.
func TestAnUploadThatStopsLeavesNothingBehind(t *testing.T) {
	app := newTestApp(t)
	prepared := prepareAs(t, app, adminRequest(t, app, http.MethodPost, "/api/photos", nil), "large.jpg", int64(uploadPartSize)+1024)
	if response := sendPart(t, app, prepared.Upload.URL, 0, bytes.Repeat([]byte("a"), uploadPartSize)); response.Code != http.StatusOK {
		t.Fatalf("sending a part = %d", response.Code)
	}
	response := httptest.NewRecorder()
	app.ServeHTTP(response, adminRequest(t, app, http.MethodDelete, prepared.Upload.URL, nil))
	if response.Code != http.StatusOK {
		t.Fatalf("giving up on an upload = %d, %s", response.Code, response.Body.String())
	}
	if entries, err := os.ReadDir(filepath.Join(app.cfg.DataDir, "uploads")); err == nil && len(entries) != 0 {
		t.Fatalf("the abandoned upload was left behind: %v", entries)
	}
	// And the session is gone with it.
	if response := sendPart(t, app, prepared.Upload.URL, 1, bytes.Repeat([]byte("b"), 1024)); response.Code != http.StatusNotFound {
		t.Fatalf("sending a part after giving up = %d, want it refused", response.Code)
	}
}

// Only the person who started an upload may send its parts.
func TestUploadsBelongToThePersonWhoStartedThem(t *testing.T) {
	app := newTestApp(t)
	prepared := prepareAs(t, app, adminRequest(t, app, http.MethodPost, "/api/photos", nil), "large.jpg", int64(uploadPartSize)+1024)
	if _, err := app.db.Exec(`INSERT INTO users(name,email,password,created_at,is_admin) VALUES('visitor','visitor@example.com','password',unixepoch(),0)`); err != nil {
		t.Fatal(err)
	}
	var visitorID int64
	if err := app.db.QueryRow(`SELECT id FROM users WHERE email='visitor@example.com'`).Scan(&visitorID); err != nil {
		t.Fatal(err)
	}
	signedIn := httptest.NewRecorder()
	app.setSession(signedIn, visitorID)
	request := httptest.NewRequest(http.MethodPut, prepared.Upload.URL+"?index=0", bytes.NewReader(bytes.Repeat([]byte("a"), uploadPartSize)))
	request.AddCookie(signedIn.Result().Cookies()[0])
	request.ContentLength = uploadPartSize
	response := httptest.NewRecorder()
	app.ServeHTTP(response, request)
	if response.Code != http.StatusNotFound {
		t.Fatalf("another user sending a part = %d, want it refused", response.Code)
	}
	signedOut := httptest.NewRecorder()
	app.ServeHTTP(signedOut, httptest.NewRequest(http.MethodPut, prepared.Upload.URL+"?index=0", bytes.NewReader(nil)))
	if signedOut.Code != http.StatusUnauthorized {
		t.Fatalf("a signed-out sender = %d, want it refused", signedOut.Code)
	}
}

// A file small enough to send in one request keeps the single signed URL, and
// so does one the site would not accept at all.
func TestOnlyFilesWorthSplittingAreSplit(t *testing.T) {
	app := newTestApp(t)
	small := adminRequest(t, app, http.MethodPost, "/api/photos", nil)
	body := `{"fileName":"small.jpg","contentType":"image/jpeg","skipDuplicateCheck":true,"size":2048}`
	small.Body = io.NopCloser(bytes.NewReader([]byte(body)))
	small.ContentLength = int64(len(body))
	response := httptest.NewRecorder()
	app.ServeHTTP(response, small)
	var prepared preparedUpload
	if err := json.Unmarshal(response.Body.Bytes(), &prepared); err != nil {
		t.Fatal(err)
	}
	if prepared.SignedURL == "" {
		t.Fatalf("a small file has nothing to upload to: %s", response.Body.String())
	}
	if prepared.Upload != nil {
		t.Fatalf("a small file was offered a plan: %+v", prepared.Upload)
	}
	// A file the site refuses is not offered a way around its own limit.
	tooLarge := adminRequest(t, app, http.MethodPost, "/api/photos", nil)
	body = fmt.Sprintf(`{"fileName":"huge.jpg","contentType":"image/jpeg","skipDuplicateCheck":true,"size":%d}`, app.maxUploadBytes()+1)
	tooLarge.Body = io.NopCloser(bytes.NewReader([]byte(body)))
	tooLarge.ContentLength = int64(len(body))
	response = httptest.NewRecorder()
	app.ServeHTTP(response, tooLarge)
	prepared = preparedUpload{}
	if err := json.Unmarshal(response.Body.Bytes(), &prepared); err != nil {
		t.Fatal(err)
	}
	if prepared.Upload != nil {
		t.Fatalf("a file over the size limit was offered a plan: %+v", prepared.Upload)
	}
}

// An upload nobody is sending any more is collected, and one still in progress
// is left alone.
func TestAbandonedUploadsAreCollected(t *testing.T) {
	app := newTestApp(t)
	prepared := prepareAs(t, app, adminRequest(t, app, http.MethodPost, "/api/photos", nil), "large.jpg", int64(uploadPartSize)+1024)
	uploads := filepath.Join(app.cfg.DataDir, "uploads")
	orphan := filepath.Join(uploads, "abandoned-from-a-previous-run")
	if err := os.MkdirAll(orphan, 0o755); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(orphan, "file")
	if err := os.WriteFile(file, []byte("half a photo"), 0o644); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-2 * uploadSessionTTL)
	if err := os.Chtimes(file, old, old); err != nil {
		t.Fatal(err)
	}
	app.sweepUploads(time.Now())
	if _, err := os.Stat(orphan); !os.IsNotExist(err) {
		t.Fatalf("an abandoned upload was kept: %v", err)
	}
	// The upload that is still going keeps both its session and its file.
	live := prepared.Upload.URL[len("/api/photos/chunks/"):]
	if _, ok := app.uploadSessionByID(live); !ok {
		t.Fatal("the upload in progress was forgotten")
	}
	if _, err := os.Stat(filepath.Join(uploads, live)); err != nil {
		t.Fatalf("the upload in progress lost its temporary file: %v", err)
	}
}
