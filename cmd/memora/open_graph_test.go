package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// shell is the shape the client build ships: a head with nothing in it that
// describes the page, and a body the client takes over.
const testShell = `<!DOCTYPE html><html><head><meta charset="utf-8"><script src="/_nuxt/entry.js"></script></head><body><div id="__nuxt"></div></body></html>`

func shellTestApp(t *testing.T) *App {
	t.Helper()
	app := newTestApp(t)
	app.cfg.WebDir = t.TempDir()
	if err := os.WriteFile(filepath.Join(app.cfg.WebDir, "index.html"), []byte(testShell), 0o644); err != nil {
		t.Fatal(err)
	}
	return app
}

func servedShell(t *testing.T, app *App, target string) string {
	t.Helper()
	response := httptest.NewRecorder()
	app.ServeHTTP(response, httptest.NewRequest(http.MethodGet, target, nil))
	if response.Code != http.StatusOK {
		t.Fatalf("%s = %d, want the client shell", target, response.Code)
	}
	if response.Header().Get("Content-Type") != "text/html; charset=utf-8" {
		t.Fatalf("%s answered as %q, want html", target, response.Header().Get("Content-Type"))
	}
	if !strings.Contains(response.Body.String(), testShell[:40]) {
		t.Fatalf("%s is not the client shell: %s", target, response.Body.String())
	}
	return response.Body.String()
}

func assertPageHas(t *testing.T, page, want string) {
	t.Helper()
	if !strings.Contains(page, want) {
		t.Fatalf("the page is missing %s:\n%s", want, page)
	}
}

func assertPageLacks(t *testing.T, page, unwanted string) {
	t.Helper()
	if strings.Contains(page, unwanted) {
		t.Fatalf("the page should not carry %s:\n%s", unwanted, page)
	}
}

func insertTestPhoto(t *testing.T, app *App, id, title, description string) {
	t.Helper()
	if _, err := app.db.Exec(`INSERT INTO photos(id,title,description,storage_key,original_url,thumbnail_url,width,height) VALUES(?,?,?,?,?,?,?,?)`,
		id, title, description, "photos/"+id+".jpg", "/storage/photos/"+id+".jpg", "/storage/photos/thumbnails/"+id+".webp", 1200, 800); err != nil {
		t.Fatal(err)
	}
}

// A link to a photo has to arrive in a chat with the photo in it, not as a bare
// address: the crawler that builds the preview never runs the client.
func TestLinkPreviewDescribesASharedPhoto(t *testing.T) {
	app := shellTestApp(t)
	app.setSetting("app", "title", "Memora Test")
	app.setSetting("app", "slogan", "Every frame counts")
	insertTestPhoto(t, app, "sunset", "Sunset over the bay", "A calm evening")
	page := servedShell(t, app, "/sunset")
	assertPageHas(t, page, "<title>Sunset over the bay</title>")
	assertPageHas(t, page, `<meta property="og:title" content="Sunset over the bay">`)
	assertPageHas(t, page, `<meta property="og:description" content="A calm evening">`)
	assertPageHas(t, page, `<meta property="og:site_name" content="Memora Test">`)
	assertPageHas(t, page, `<meta property="og:type" content="article">`)
	assertPageHas(t, page, `<meta property="og:url" content="http://example.com/sunset">`)
	// The picture the preview shows is the card the server draws for this photo,
	// not the stored thumbnail: the card carries the photo's own text, and a
	// thumbnail address is a bare picture a chat client would show without any.
	assertPageHas(t, page, `<meta property="og:image" content="http://example.com/_og/photo/sunset.jpg">`)
	assertPageHas(t, page, `<meta property="og:image:width" content="1200">`)
	assertPageHas(t, page, `<meta property="og:image:height" content="630">`)
	assertPageHas(t, page, `<meta property="og:image:alt" content="Sunset over the bay">`)
	assertPageHas(t, page, `<meta name="twitter:image" content="http://example.com/_og/photo/sunset.jpg">`)
	assertPageHas(t, page, `<meta name="twitter:card" content="summary_large_image">`)
	// A photo the viewer cannot name is still described by its picture rather
	// than left to the site's own title.
	insertTestPhoto(t, app, "untitled", "", "")
	page = servedShell(t, app, "/untitled")
	assertPageHas(t, page, "<title>Memora Test</title>")
	assertPageHas(t, page, `<meta property="og:image" content="http://example.com/_og/photo/untitled.jpg">`)
	assertPageHas(t, page, `<meta property="og:description" content="Every frame counts">`)
}

// A preview is built for whoever the link is pasted in front of, so a photo an
// album keeps hidden or behind a password must not be named in the page.
func TestLinkPreviewKeepsPrivatePhotosOutOfThePage(t *testing.T) {
	for _, protected := range []struct {
		name    string
		columns string
		values  string
	}{
		{name: "a hidden album", columns: "is_hidden", values: "1"},
		{name: "a locked album", columns: "password_hash", values: "'$2a$10$locked'"},
	} {
		app := shellTestApp(t)
		app.setSetting("app", "title", "Memora Test")
		if _, err := app.db.Exec(`INSERT INTO albums(id,title,` + protected.columns + `) VALUES(1,'Private',` + protected.values + `)`); err != nil {
			t.Fatal(err)
		}
		insertTestPhoto(t, app, "secret", "At the party", "Nobody else should see this")
		if _, err := app.db.Exec(`INSERT INTO album_photos(album_id,photo_id) VALUES(1,'secret')`); err != nil {
			t.Fatal(err)
		}
		page := servedShell(t, app, "/secret")
		if strings.Contains(page, "At the party") {
			t.Fatalf("%s is named in the page:\n%s", protected.name, page)
		}
		assertPageLacks(t, page, "thumbnails/secret.webp")
		assertPageLacks(t, page, "/_og/photo/secret.jpg")
		assertPageLacks(t, page, "Nobody else should see this")
		assertPageHas(t, page, `<meta property="og:title" content="Memora Test">`)
		// The gallery keeps the photo out of the same list, so the two cannot
		// drift apart.
		visible := httptest.NewRecorder()
		app.ServeHTTP(visible, httptest.NewRequest(http.MethodGet, "/api/photos/visible", nil))
		assertPageLacks(t, visible.Body.String(), "At the party")
	}
}

// Everything that is not a photo is described as the site itself.
func TestLinkPreviewDescribesTheSite(t *testing.T) {
	app := shellTestApp(t)
	app.setSetting("app", "title", "Memora Test")
	app.setSetting("app", "slogan", "Every frame counts")
	app.setSetting("app", "avatarUrl", "/storage/photos/avatars/avatar-0123456789abcdef.webp")
	for _, target := range []string{"/", "/albums", "/globe", "/no-such-photo"} {
		page := servedShell(t, app, target)
		assertPageHas(t, page, "<title>Memora Test</title>")
		assertPageHas(t, page, `<meta property="og:title" content="Memora Test">`)
		assertPageHas(t, page, `<meta property="og:description" content="Every frame counts">`)
		assertPageHas(t, page, `<meta property="og:type" content="website">`)
		// Every page that is not a photo is the site, and the site is shown as
		// the card drawn for it. The picture is drawn from what the settings say
		// at the moment the card is asked for, so an avatar changed a minute ago
		// is in the preview a minute later.
		assertPageHas(t, page, `<meta property="og:image" content="http://example.com/_og/site.jpg">`)
		assertPageHas(t, page, `<meta name="twitter:card" content="summary_large_image">`)
	}
}

// A title is whatever the uploader wrote, and it is written into the page
// markup, so it cannot be allowed to close the tag it sits in.
func TestLinkPreviewEscapesWhatAPhotoIsCalled(t *testing.T) {
	app := shellTestApp(t)
	insertTestPhoto(t, app, "sunset", `Bay <script>alert("x")</script>`, `A "calm" evening & more`)
	page := servedShell(t, app, "/sunset")
	assertPageLacks(t, page, "<script>alert")
	assertPageHas(t, page, `<meta property="og:title" content="Bay &lt;script&gt;alert(&#34;x&#34;)&lt;/script&gt;">`)
	assertPageHas(t, page, `<meta property="og:description" content="A &#34;calm&#34; evening &amp; more">`)
}

// A photo whose id reads like one of the client's own addresses is reached
// through its own page, so the preview has to describe that page.
func TestLinkPreviewLeavesTheClientsOwnRoutesAlone(t *testing.T) {
	app := shellTestApp(t)
	app.setSetting("app", "title", "Memora Test")
	insertTestPhoto(t, app, "dashboard", "A photo named like a page", "")
	page := servedShell(t, app, "/dashboard")
	assertPageLacks(t, page, "A photo named like a page")
	assertPageHas(t, page, `<meta property="og:title" content="Memora Test">`)
}

// A name that is not a photo is not looked up as one, and an address that
// carries a file name never reaches the photo lookup at all.
func TestPhotoIDFromPath(t *testing.T) {
	cases := map[string]string{
		"/sunset":           "sunset",
		"/sunset/":          "sunset",
		"/":                 "",
		"":                  "",
		"/albums":           "",
		"/albums/12":        "",
		"/dashboard":        "",
		"/_nuxt/entry.js":   "",
		"/favicon.ico":      "",
		"/globe":            "",
		"/web-app-icon.png": "",
		"/some/nested/page": "",
		"/signin":           "",
	}
	for path, want := range cases {
		if got := photoIDFromPath(path); got != want {
			t.Fatalf("photoIDFromPath(%q) = %q, want %q", path, got, want)
		}
	}
}

func TestPreviewAddressesAreAbsolute(t *testing.T) {
	cases := []struct {
		origin, address, want string
	}{
		{"https://gallery.example.com", "/storage/photos/a.jpg", "https://gallery.example.com/storage/photos/a.jpg"},
		{"https://gallery.example.com", "storage/photos/a.jpg", "https://gallery.example.com/storage/photos/a.jpg"},
		{"https://gallery.example.com", "https://cdn.example.com/a.jpg", "https://cdn.example.com/a.jpg"},
		{"https://gallery.example.com", "//cdn.example.com/a.jpg", "//cdn.example.com/a.jpg"},
		{"", "/storage/photos/a.jpg", "/storage/photos/a.jpg"},
		{"https://gallery.example.com", "", ""},
	}
	for _, testCase := range cases {
		if got := absoluteURL(testCase.origin, testCase.address); got != testCase.want {
			t.Fatalf("absoluteURL(%q, %q) = %q, want %q", testCase.origin, testCase.address, got, testCase.want)
		}
	}
}

// The server is regularly reached through a proxy that terminates TLS, and the
// preview addresses have to name the address a visitor typed.
func TestRequestOriginFollowsTheProxy(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/sunset", nil)
	request.Host = "internal:3000"
	request.Header.Set("X-Forwarded-Proto", "https")
	request.Header.Set("X-Forwarded-Host", "gallery.example.com")
	if got := requestOrigin(request); got != "https://gallery.example.com" {
		t.Fatalf("origin = %q, want the forwarded address", got)
	}
	request = httptest.NewRequest(http.MethodGet, "/sunset", nil)
	if got := requestOrigin(request); got != "http://example.com" {
		t.Fatalf("origin = %q, want the request's own address", got)
	}
}
