package main

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// avatarTestPNG draws a picture whose every pixel differs, so two sizes do not
// encode to the same bytes.
func avatarTestPNG(t *testing.T, width, height int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for x := 0; x < width; x++ {
		for y := 0; y < height; y++ {
			img.Set(x, y, color.RGBA{R: uint8(x), G: uint8(y), B: uint8(x + y), A: 255})
		}
	}
	var buffer bytes.Buffer
	if err := png.Encode(&buffer, img); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

func requireFFmpeg(t *testing.T, app *App) {
	t.Helper()
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg is not available")
	}
	app.cfg.FFmpeg = "ffmpeg"
}

func avatarUploadRequest(t *testing.T, app *App, fileName, contentType string, data []byte) *http.Request {
	t.Helper()
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	header := make(map[string][]string)
	header["Content-Disposition"] = []string{`form-data; name="file"; filename="` + fileName + `"`}
	if contentType != "" {
		header["Content-Type"] = []string{contentType}
	}
	part, err := writer.CreatePart(header)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(data); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	request := adminRequest(t, app, http.MethodPost, "/api/system/settings/avatar", body.Bytes())
	request.Header.Set("Content-Type", writer.FormDataContentType())
	return request
}

func uploadedAvatarURL(t *testing.T, app *App, fileName string, data []byte) string {
	t.Helper()
	response := httptest.NewRecorder()
	app.ServeHTTP(response, avatarUploadRequest(t, app, fileName, "image/png", data))
	if response.Code != http.StatusOK {
		t.Fatalf("upload status = %d, body = %s", response.Code, response.Body.String())
	}
	var body struct {
		AvatarURL string `json:"avatarUrl"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.AvatarURL == "" {
		t.Fatal("upload answered without an avatar address")
	}
	return body.AvatarURL
}

// storedAvatarFile is where the local provider keeps the picture an address
// points at.
func storedAvatarFile(app *App, avatarURL string) string {
	key := strings.TrimPrefix(avatarURL, "/storage/")
	return filepath.Join(app.cfg.LocalPath, filepath.FromSlash(key))
}

// An uploaded avatar is stored in the configured provider, reduced to the size
// the interface uses, and pointed at by the public setting — which is what the
// loading screen and the header read.
func TestAvatarUploadStoresThePictureAndPointsTheSettingAtIt(t *testing.T) {
	app := newTestApp(t)
	requireFFmpeg(t, app)
	// Warm the snapshot first, the way the process does at startup, so the
	// upload has to keep it current rather than being read back from SQLite.
	if _, err := app.publicSettingsSnapshot(context.Background()); err != nil {
		t.Fatal(err)
	}
	avatarURL := uploadedAvatarURL(t, app, "selfie.png", avatarTestPNG(t, 800, 600))
	if !strings.HasPrefix(avatarURL, "/storage/photos/avatars/avatar-") || !strings.HasSuffix(avatarURL, ".webp") {
		t.Fatalf("avatar address = %q, want a stored webp under the storage provider", avatarURL)
	}
	stored, err := os.ReadFile(storedAvatarFile(app, avatarURL))
	if err != nil {
		t.Fatalf("avatar was not stored: %v", err)
	}
	if !bytes.HasPrefix(stored, []byte("RIFF")) || !bytes.Contains(stored[:16], []byte("WEBP")) {
		t.Fatalf("stored avatar is not a webp: % x", stored[:16])
	}
	// The picture is downsized to the square the interface shows, and never
	// enlarged past its own size.
	config, format, err := image.DecodeConfig(bytes.NewReader(stored))
	if err != nil {
		t.Fatalf("stored avatar cannot be decoded: %v", err)
	}
	if format != "webp" || config.Width != avatarPixels || config.Height != avatarPixels {
		t.Fatalf("stored avatar = %s %dx%d, want webp %dx%d", format, config.Width, config.Height, avatarPixels, avatarPixels)
	}
	var setting any
	if !app.readSetting("app", "avatarUrl", &setting) || setting != avatarURL {
		t.Fatalf("avatarUrl setting = %#v, want %q", setting, avatarURL)
	}
	snapshot, err := app.publicSettingsSnapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if snapshot["app"]["avatarUrl"] != avatarURL {
		t.Fatalf("public avatarUrl = %#v, want %q", snapshot["app"]["avatarUrl"], avatarURL)
	}
	// The address a visitor's browser is given serves the picture.
	response := httptest.NewRecorder()
	app.ServeHTTP(response, httptest.NewRequest(http.MethodGet, avatarURL, nil))
	if response.Code != http.StatusOK {
		t.Fatalf("avatar request = %d, want 200", response.Code)
	}
	if !bytes.Equal(response.Body.Bytes(), stored) {
		t.Fatal("the served avatar is not the stored picture")
	}
}

// Replacing the avatar has to leave one picture behind, under the address the
// setting now names.
func TestAvatarUploadReplacesThePreviousPicture(t *testing.T) {
	app := newTestApp(t)
	requireFFmpeg(t, app)
	first := uploadedAvatarURL(t, app, "first.png", avatarTestPNG(t, 400, 400))
	firstFile := storedAvatarFile(app, first)
	second := uploadedAvatarURL(t, app, "second.png", avatarTestPNG(t, 300, 500))
	if first == second {
		t.Fatalf("both uploads were stored as %q", first)
	}
	if _, err := os.Stat(firstFile); !os.IsNotExist(err) {
		t.Fatalf("the replaced picture is still stored: %v", err)
	}
	if _, err := os.Stat(storedAvatarFile(app, second)); err != nil {
		t.Fatalf("the new picture was not stored: %v", err)
	}
	if current := app.avatarURL(); current != second {
		t.Fatalf("avatarUrl = %q, want %q", current, second)
	}
	// The same picture again lands on the same address, and the picture it
	// replaced is the one that goes.
	if again := uploadedAvatarURL(t, app, "first.png", avatarTestPNG(t, 400, 400)); again != first {
		t.Fatalf("re-uploading the same picture stored it as %q, want %q", again, first)
	}
	if _, err := os.Stat(firstFile); err != nil {
		t.Fatalf("the re-uploaded picture is missing: %v", err)
	}
	if _, err := os.Stat(storedAvatarFile(app, second)); !os.IsNotExist(err) {
		t.Fatalf("the replaced picture is still stored: %v", err)
	}
}

func TestAvatarUploadRefusesWhatIsNotAPicture(t *testing.T) {
	app := newTestApp(t)
	requireFFmpeg(t, app)
	// A text file, whether the browser is honest about it or not. The bytes are
	// asked first, so a declared image type cannot let it through.
	for _, declared := range []string{"text/plain", "image/png", ""} {
		response := httptest.NewRecorder()
		app.ServeHTTP(response, avatarUploadRequest(t, app, "notes.txt", declared, []byte("this is not a picture")))
		if response.Code != http.StatusBadRequest {
			t.Fatalf("a text file declared as %q was accepted: %d %s", declared, response.Code, response.Body.String())
		}
	}
	if current := app.avatarURL(); current != "" {
		t.Fatalf("avatarUrl = %q after refused uploads, want it untouched", current)
	}
	// A camera-sized file is fine; one past the limit is not, and the limit is
	// reported rather than being a silent failure.
	oversized := bytes.Repeat([]byte{0x00}, avatarMaxBytes+1)
	response := httptest.NewRecorder()
	app.ServeHTTP(response, avatarUploadRequest(t, app, "huge.png", "image/png", oversized))
	if response.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("an oversized upload = %d %s, want 413", response.Code, response.Body.String())
	}
}

// Removing the avatar has to take the stored picture with it, or every removal
// would leave a file behind that nothing points at.
func TestDeletingTheAvatarRemovesThePictureAndClearsTheSetting(t *testing.T) {
	app := newTestApp(t)
	requireFFmpeg(t, app)
	avatarURL := uploadedAvatarURL(t, app, "logo.png", avatarTestPNG(t, 200, 200))
	response := httptest.NewRecorder()
	app.ServeHTTP(response, adminRequest(t, app, http.MethodDelete, "/api/system/settings/avatar", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("delete status = %d, body = %s", response.Code, response.Body.String())
	}
	if _, err := os.Stat(storedAvatarFile(app, avatarURL)); !os.IsNotExist(err) {
		t.Fatalf("the picture is still stored: %v", err)
	}
	if current := app.avatarURL(); current != "" {
		t.Fatalf("avatarUrl = %q after removal, want it cleared", current)
	}
	snapshot, err := app.publicSettingsSnapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if snapshot["app"]["avatarUrl"] != "" {
		t.Fatalf("public avatarUrl = %#v after removal, want it cleared", snapshot["app"]["avatarUrl"])
	}
	// Removing an avatar that is already gone is not an error; the interface
	// offers the button whenever an address is set.
	response = httptest.NewRecorder()
	app.ServeHTTP(response, adminRequest(t, app, http.MethodDelete, "/api/system/settings/avatar", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("repeated delete status = %d, body = %s", response.Code, response.Body.String())
	}
}

// An address the administrator typed in points at somebody else's server, and
// nothing of theirs may be deleted through it.
func TestAvatarAddressesTypedInByHandAreLeftAlone(t *testing.T) {
	app := newTestApp(t)
	requireFFmpeg(t, app)
	if err := app.updateSetting("app", "avatarUrl", "https://cdn.example.com/my-avatars/avatar.png", nil, false); err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	app.ServeHTTP(response, adminRequest(t, app, http.MethodDelete, "/api/system/settings/avatar", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("delete status = %d, body = %s", response.Code, response.Body.String())
	}
	if current := app.avatarURL(); current != "" {
		t.Fatalf("avatarUrl = %q after removal, want it cleared", current)
	}
}

func TestAvatarStorageKeysAreReadOnlyFromStoredAddresses(t *testing.T) {
	cases := map[string]string{
		"/storage/photos/avatars/avatar-0123456789abcdef.webp":                   "avatars/avatar-0123456789abcdef.webp",
		"https://cdn.example.com/photos/avatars/avatar-fedcba9876543210.png?v=2": "avatars/avatar-fedcba9876543210.png",
		"https://example.com/avatar.png":                                         "",
		"https://example.com/my-avatars/avatar-0123456789abcdef.webp":            "",
		"/storage/photos/avatars/avatar-0123456789ABCDEF.webp":                   "",
		"/storage/photos/avatars/avatar-selfie.webp":                             "",
		"": "",
	}
	for avatarURL, want := range cases {
		if got := avatarStorageKey(avatarURL); got != want {
			t.Fatalf("avatarStorageKey(%q) = %q, want %q", avatarURL, got, want)
		}
	}
}

func TestAvatarUploadNeedsAnAdministrator(t *testing.T) {
	app := newTestApp(t)
	anonymous := httptest.NewRecorder()
	app.ServeHTTP(anonymous, httptest.NewRequest(http.MethodPost, "/api/system/settings/avatar", nil))
	if anonymous.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous upload = %d, want 401", anonymous.Code)
	}
	anonymous = httptest.NewRecorder()
	app.ServeHTTP(anonymous, httptest.NewRequest(http.MethodDelete, "/api/system/settings/avatar", nil))
	if anonymous.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous delete = %d, want 401", anonymous.Code)
	}
	// A signed-in visitor who is not an administrator has no say over the site's
	// appearance either.
	if _, err := app.db.Exec(`INSERT INTO users(id,name,email,password,created_at,is_admin) VALUES(2,'visitor','visitor@example.com','password',unixepoch(),0)`); err != nil {
		t.Fatal(err)
	}
	cookieRecorder := httptest.NewRecorder()
	app.setSession(cookieRecorder, 2)
	request := httptest.NewRequest(http.MethodPost, "/api/system/settings/avatar", nil)
	request.AddCookie(cookieRecorder.Result().Cookies()[0])
	response := httptest.NewRecorder()
	app.ServeHTTP(response, request)
	if response.Code != http.StatusForbidden {
		t.Fatalf("upload by a visitor = %d, want 403", response.Code)
	}
}

// Only the formats a browser draws are accepted, and the bytes decide.
func TestAvatarContentType(t *testing.T) {
	picture := avatarTestPNG(t, 8, 8)
	cases := []struct {
		name     string
		data     []byte
		declared string
		want     string
	}{
		{"bytes over a wrong declaration", picture, "text/plain", "image/png"},
		{"bytes over no declaration", picture, "", "image/png"},
		{"a text file declared as an image", []byte("not a picture"), "image/png", ""},
		{"an empty file", nil, "image/png", ""},
		// Go has no signature for AVIF, so the browser's own answer stands in.
		{"a format Go cannot identify", append([]byte{0x00, 0x00, 0x00, 0x20}, []byte("ftypavif\x00\x00\x00\x01")...), "image/avif", "image/avif"},
		{"a video declared as one", []byte("\x00\x00\x00\x18ftypmp42"), "video/mp4", ""},
	}
	for _, testCase := range cases {
		if got := avatarContentType(testCase.data, testCase.declared); got != testCase.want {
			t.Fatalf("%s: avatarContentType = %q, want %q", testCase.name, got, testCase.want)
		}
	}
}
