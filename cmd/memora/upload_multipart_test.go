package main

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// fakeObjectStore answers the handful of calls an upload of parts makes to an
// object store, so the multipart path can be exercised without a bucket.
type fakeObjectStore struct {
	mu       sync.Mutex
	uploadID string
	parts    []fakePart
	calls    []string
	aborted  bool
}

type fakePart struct {
	number int
	etag   string
}

type listPartsResult struct {
	XMLName  xml.Name `xml:"ListPartsResult"`
	Bucket   string   `xml:"Bucket"`
	Key      string   `xml:"Key"`
	UploadID string   `xml:"UploadId"`
	Parts    []struct {
		PartNumber int    `xml:"PartNumber"`
		ETag       string `xml:"ETag"`
		Size       int64  `xml:"Size"`
	} `xml:"Part"`
}

func newFakeObjectStore(t *testing.T) (*fakeObjectStore, *httptest.Server) {
	t.Helper()
	store := &fakeObjectStore{uploadID: "upload-1"}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.RawQuery
		store.mu.Lock()
		store.calls = append(store.calls, r.Method+" "+r.URL.Path+"?"+query)
		store.mu.Unlock()
		switch {
		case r.Method == http.MethodPost && strings.Contains(query, "uploads"):
			w.Header().Set("Content-Type", "application/xml")
			fmt.Fprintf(w, `<?xml version="1.0" encoding="UTF-8"?><InitiateMultipartUploadResult><Bucket>bucket</Bucket><Key>%s</Key><UploadId>%s</UploadId></InitiateMultipartUploadResult>`, r.URL.Path, store.uploadID)
		case r.Method == http.MethodGet && strings.Contains(query, "uploadId="):
			store.mu.Lock()
			result := listPartsResult{Bucket: "bucket", Key: r.URL.Path, UploadID: store.uploadID}
			for _, part := range store.parts {
				result.Parts = append(result.Parts, struct {
					PartNumber int    `xml:"PartNumber"`
					ETag       string `xml:"ETag"`
					Size       int64  `xml:"Size"`
				}{PartNumber: part.number, ETag: part.etag, Size: uploadPartSize})
			}
			store.mu.Unlock()
			w.Header().Set("Content-Type", "application/xml")
			_ = xml.NewEncoder(w).Encode(result)
		case r.Method == http.MethodPost && strings.Contains(query, "uploadId="):
			w.Header().Set("Content-Type", "application/xml")
			fmt.Fprintf(w, `<?xml version="1.0" encoding="UTF-8"?><CompleteMultipartUploadResult><Location>http://example.test/bucket%s</Location><Bucket>bucket</Bucket><Key>%s</Key><ETag>"joined"</ETag></CompleteMultipartUploadResult>`, r.URL.Path, r.URL.Path)
		case r.Method == http.MethodDelete && strings.Contains(query, "uploadId="):
			store.mu.Lock()
			store.aborted = true
			store.mu.Unlock()
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodGet:
			w.Header().Set("Content-Type", "application/xml")
			fmt.Fprint(w, `<?xml version="1.0" encoding="UTF-8"?><LocationConstraint>us-east-1</LocationConstraint>`)
		default:
			w.WriteHeader(http.StatusNoContent)
		}
	}))
	t.Cleanup(server.Close)
	return store, server
}

func (s *fakeObjectStore) setParts(numbers ...int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.parts = nil
	for index, number := range numbers {
		s.parts = append(s.parts, fakePart{number: number, etag: fmt.Sprintf("\"etag-%d-%d\"", number, index)})
	}
}

func (s *fakeObjectStore) sawRequest(method, fragment string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, call := range s.calls {
		if strings.HasPrefix(call, method+" ") && strings.Contains(call, fragment) {
			return true
		}
	}
	return false
}

// appOnObjectStore points an app at the fake store, the way a site configured
// for S3 uploads is.
func appOnObjectStore(t *testing.T, endpoint string) *App {
	t.Helper()
	app := newTestApp(t)
	cfg := app.cfg
	cfg.S3Endpoint = endpoint
	cfg.S3AccessKey = "access"
	cfg.S3SecretKey = "secret"
	cfg.S3Bucket = "bucket"
	cfg.S3Region = "us-east-1"
	cfg.S3PathStyle = true
	storage, err := NewS3Storage(cfg)
	if err != nil {
		t.Fatal(err)
	}
	app.storage = storage
	return app
}

// A site backed by an object store hands the browser one signed URL per part and
// finishes the upload from what the store itself says it holds, so a browser
// that cannot read the ETag of a part still uploads.
func TestUploadsToAnObjectStoreGoStraightToIt(t *testing.T) {
	store, server := newFakeObjectStore(t)
	app := appOnObjectStore(t, server.URL)
	prepared := prepareAs(t, app, adminRequest(t, app, http.MethodPost, "/api/photos", nil), "large.jpg", int64(uploadPartSize)+1024)

	if prepared.Upload.Mode != "multipart" {
		t.Fatalf("mode = %q, want the file sent straight to the store", prepared.Upload.Mode)
	}
	if prepared.Upload.PartCount != 2 || len(prepared.Upload.PartURLs) != 2 {
		t.Fatalf("plan = %+v, want two signed parts", prepared.Upload)
	}
	for index, signed := range prepared.Upload.PartURLs {
		if !strings.Contains(signed, fmt.Sprintf("partNumber=%d", index+1)) || !strings.Contains(signed, "uploadId=upload-1") {
			t.Fatalf("part %d was signed as %s, want it tied to the store's upload", index, signed)
		}
	}

	// Only one of the two parts reached the store, so completing is refused.
	store.setParts(1)
	response := httptest.NewRecorder()
	app.ServeHTTP(response, adminRequest(t, app, http.MethodPost, prepared.Upload.URL, nil))
	if response.Code != http.StatusBadRequest {
		t.Fatalf("completing an upload missing a part = %d, %s", response.Code, response.Body.String())
	}

	// Once the store holds both, the upload finishes without the browser having
	// reported anything about them.
	store.setParts(1, 2)
	response = httptest.NewRecorder()
	app.ServeHTTP(response, adminRequest(t, app, http.MethodPost, prepared.Upload.URL, nil))
	if response.Code != http.StatusOK {
		t.Fatalf("completing the upload = %d, %s", response.Code, response.Body.String())
	}
	if !store.sawRequest(http.MethodPost, "uploadId=upload-1") {
		t.Fatalf("the store was never asked to join the parts: %v", store.calls)
	}
	// And the upload is forgotten, so it cannot be completed twice.
	if response := sendPart(t, app, prepared.Upload.URL, 0, nil); response.Code != http.StatusNotFound {
		t.Fatalf("completing a finished upload = %d, want it refused", response.Code)
	}
}

// Giving up on an upload that went straight to the store abandons it there too,
// so a half-sent file is not left to be paid for.
func TestGivingUpOnAnObjectStoreUploadAbandonsIt(t *testing.T) {
	store, server := newFakeObjectStore(t)
	app := appOnObjectStore(t, server.URL)
	prepared := prepareAs(t, app, adminRequest(t, app, http.MethodPost, "/api/photos", nil), "large.jpg", int64(uploadPartSize)*3)

	response := httptest.NewRecorder()
	app.ServeHTTP(response, adminRequest(t, app, http.MethodDelete, prepared.Upload.URL, nil))
	if response.Code != http.StatusOK {
		t.Fatalf("giving up on the upload = %d, %s", response.Code, response.Body.String())
	}
	if !store.sawRequest(http.MethodDelete, "uploadId=upload-1") {
		t.Fatalf("the upload was left on the store: %v", store.calls)
	}
	if !store.aborted {
		t.Fatal("the store was never asked to throw the parts away")
	}
	// The session is gone with it.
	response = httptest.NewRecorder()
	app.ServeHTTP(response, adminRequest(t, app, http.MethodPost, prepared.Upload.URL, nil))
	if response.Code != http.StatusNotFound {
		t.Fatalf("finishing an abandoned upload = %d, want it refused", response.Code)
	}
}

// A store that refuses to start an upload leaves the browser the signed URL it
// has always used, rather than a plan that cannot work.
func TestAnObjectStoreThatRefusesLeavesTheSingleRequest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "no", http.StatusForbidden)
	}))
	defer server.Close()
	app := appOnObjectStore(t, server.URL)
	body := fmt.Sprintf(`{"fileName":"large.jpg","contentType":"image/jpeg","skipDuplicateCheck":true,"size":%d}`, int64(uploadPartSize)*2)
	request := adminRequest(t, app, http.MethodPost, "/api/photos", nil)
	request.Body = io.NopCloser(strings.NewReader(body))
	request.ContentLength = int64(len(body))
	response := httptest.NewRecorder()
	app.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("preparing an upload = %d, %s", response.Code, response.Body.String())
	}
	var prepared preparedUpload
	if err := json.Unmarshal(response.Body.Bytes(), &prepared); err != nil {
		t.Fatal(err)
	}
	if prepared.Upload != nil {
		t.Fatalf("a plan was offered by a store that refused: %+v", prepared.Upload)
	}
	// The signed URL is still there for the single request to use.
	if prepared.SignedURL == "" {
		t.Fatal("no signed URL was offered either")
	}
}
