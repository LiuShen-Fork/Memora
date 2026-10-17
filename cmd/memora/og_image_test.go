package main

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/image/font/gofont/goregular"
)

// A card is the picture a chat client shows for a shared link. Every client
// reads the large card at one size, so the card has to be that size.
func TestTheSiteCardIsDrawnAtTheShareSize(t *testing.T) {
	app := shellTestApp(t)
	app.setSetting("app", "title", "Memora Test")
	app.setSetting("app", "slogan", "Every frame counts")
	response := getCard(t, app, siteCardAddress)
	picture := decodeCard(t, response)
	if bounds := picture.Bounds(); bounds.Dx() != ogCardWidth || bounds.Dy() != ogCardHeight {
		t.Fatalf("the card is %v, want %dx%d", bounds, ogCardWidth, ogCardHeight)
	}
	// A card is a picture of the site as it is now, so it is worth keeping and
	// worth asking about again later.
	if cache := response.Header().Get("Cache-Control"); !strings.Contains(cache, "max-age=") {
		t.Fatalf("the card is served with Cache-Control %q", cache)
	}
	// HEAD asks the same question as GET without the picture, so a crawler that
	// only wants to know the card is there does not download it twice.
	head := httptest.NewRecorder()
	app.ServeHTTP(head, httptest.NewRequest(http.MethodHead, siteCardAddress, nil))
	if head.Code != http.StatusOK {
		t.Fatalf("HEAD %s = %d, want the card", siteCardAddress, head.Code)
	}
	if head.Body.Len() != 0 {
		t.Fatalf("HEAD %s answered with %d bytes of picture", siteCardAddress, head.Body.Len())
	}
}

// A site with photographs on it shows them: the card of a site is meant to
// offer what the site holds.
func TestTheSiteCardShowsTheSitesPhotographs(t *testing.T) {
	app := shellTestApp(t)
	app.setSetting("app", "title", "Memora Test")
	insertTestPhoto(t, app, "sunset", "Sunset over the bay", "")
	unreadable := getCard(t, app, siteCardAddress).Body.Bytes()

	writeTestPicture(t, app, "photos/thumbnails/sunset.webp", color.RGBA{0xff, 0x88, 0x22, 0xff})
	pictured := getCard(t, app, siteCardAddress).Body.Bytes()
	if bytes.Equal(unreadable, pictured) {
		t.Fatal("the card of a site holding a picture is the card of one that is not")
	}
}

// A site too new to hold anything shows the picture it goes by instead.
func TestTheSiteCardCarriesTheSitesOwnPicture(t *testing.T) {
	app := shellTestApp(t)
	app.setSetting("app", "title", "Memora Test")
	plain := getCard(t, app, siteCardAddress).Body.Bytes()

	writeTestPicture(t, app, "photos/avatars/logo.png", color.RGBA{0x00, 0x80, 0xff, 0xff})
	app.setSetting("app", "avatarUrl", "/storage/photos/avatars/logo.png")
	pictured := getCard(t, app, siteCardAddress).Body.Bytes()
	if bytes.Equal(plain, pictured) {
		t.Fatal("the card of a site with a picture is the card of a site without one")
	}
}

// A card is drawn for whoever the link is pasted in front of, so the pictures
// it offers are the ones that visitor is allowed to see.
func TestTheSiteCardOnlyShowsPicturesAVisitorMaySee(t *testing.T) {
	app := shellTestApp(t)
	insertTestPhoto(t, app, "visible", "", "")
	insertTestPhoto(t, app, "hidden", "", "")
	if _, err := app.db.Exec(`INSERT INTO albums(id,title,is_hidden) VALUES(1,'Private',1)`); err != nil {
		t.Fatal(err)
	}
	if _, err := app.db.Exec(`INSERT INTO album_photos(album_id,photo_id) VALUES(1,'hidden')`); err != nil {
		t.Fatal(err)
	}
	covers := app.publicPhotoCovers(context.Background(), siteCardFrames)
	if len(covers) != 1 || covers[0] != "/storage/photos/thumbnails/visible.webp" {
		t.Fatalf("the card offers %q, want only the picture a visitor may see", covers)
	}
	if total := app.publicPhotoTotal(context.Background()); total != 1 {
		t.Fatalf("the site says it holds %d pictures, want 1", total)
	}
}

// The card says how many pictures the site holds, and it says it in the
// language the page that named it was served in.
func TestTheCardSaysItsCountInTheLanguageOfThePage(t *testing.T) {
	app := shellTestApp(t)
	app.setSetting("app", "title", "Memora Test")
	insertTestPhoto(t, app, "sunset", "Sunset over the bay", "")

	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.AddCookie(&http.Cookie{Name: localeCookie, Value: "zh-Hant-TW"})
	response := httptest.NewRecorder()
	app.ServeHTTP(response, request)
	assertPageHas(t, response.Body.String(), `<meta property="og:image" content="http://example.com/_og/site.jpg?lang=zh-Hant-TW">`)

	english := getCard(t, app, siteCardAddress).Body.Bytes()
	chinese := getCard(t, app, siteCardAddress+"?lang=zh-Hant-TW").Body.Bytes()
	if bytes.Equal(english, chinese) {
		t.Fatal("the card of a site is the same picture in Chinese as in English")
	}
}

// The words the card says are the words the client says for the same count.
func TestTheCountOfPicturesIsWrittenInEachLanguage(t *testing.T) {
	for _, counted := range []struct {
		locale string
		total  int
		want   string
	}{
		{locale: "en", total: 1, want: "one photo"},
		{locale: "en", total: 1234, want: "1,234 photos"},
		{locale: "zh-Hans", total: 1234, want: "1,234 张照片"},
		{locale: "zh-Hant-TW", total: 2, want: "2 張相片"},
		{locale: "zh-CN", total: 3, want: "3 张照片"},
		{locale: "ja", total: 12, want: "写真12枚"},
		{locale: "ru", total: 5, want: "5 фото"},
		{locale: "", total: 7, want: "7 photos"},
		{locale: "de", total: 7, want: "7 photos"},
	} {
		if written := photoCount(counted.locale, counted.total); written != counted.want {
			t.Errorf("photoCount(%q, %d) = %q, want %q", counted.locale, counted.total, written, counted.want)
		}
	}
}

// A shared photo arrives with the photo on it.
func TestThePhotoCardCarriesThePhoto(t *testing.T) {
	app := shellTestApp(t)
	app.setSetting("app", "title", "Memora Test")
	insertTestPhoto(t, app, "sunset", "Sunset over the bay", "A calm evening")
	plain := getCard(t, app, photoCardAddress("sunset")).Body.Bytes()

	writeTestPicture(t, app, "photos/thumbnails/sunset.webp", color.RGBA{0xff, 0x88, 0x22, 0xff})
	pictured := getCard(t, app, photoCardAddress("sunset")).Body.Bytes()
	if bytes.Equal(plain, pictured) {
		t.Fatal("the card of a photo with a picture is the card of a photo without one")
	}
}

// A preview is built for whoever the link is pasted in front of, so a photo an
// album keeps hidden or behind a password has no card to give away: the address
// says the photo exists.
func TestAPhotoTheVisitorCannotSeeHasNoCard(t *testing.T) {
	for _, protected := range []struct {
		name    string
		columns string
		values  string
	}{
		{name: "a hidden album", columns: "is_hidden", values: "1"},
		{name: "a locked album", columns: "password_hash", values: "'$2a$10$locked'"},
	} {
		app := shellTestApp(t)
		if _, err := app.db.Exec(`INSERT INTO albums(id,title,` + protected.columns + `) VALUES(1,'Private',` + protected.values + `)`); err != nil {
			t.Fatal(err)
		}
		insertTestPhoto(t, app, "secret", "At the party", "Nobody else should see this")
		if _, err := app.db.Exec(`INSERT INTO album_photos(album_id,photo_id) VALUES(1,'secret')`); err != nil {
			t.Fatal(err)
		}
		response := getCard(t, app, photoCardAddress("secret"))
		if response.Code != http.StatusNotFound {
			t.Fatalf("the card of a photo in %s = %d, want it to be absent", protected.name, response.Code)
		}
	}
}

// Everything a card is not is refused rather than guessed at.
func TestTheCardAddressHasToNameSomething(t *testing.T) {
	app := shellTestApp(t)
	for _, target := range []string{"/_og/", "/_og/photo/nothing.jpg", "/_og/photo/", "/_og/site.png"} {
		if response := getCard(t, app, target); response.Code != http.StatusNotFound {
			t.Fatalf("%s = %d, want it to be absent", target, response.Code)
		}
	}
	posted := httptest.NewRecorder()
	app.ServeHTTP(posted, httptest.NewRequest(http.MethodPost, siteCardAddress, nil))
	if posted.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST %s = %d, want the card to be read only", siteCardAddress, posted.Code)
	}
}

// The type the Go library ships has no Chinese, Japanese or Korean letters, and
// a font the machine keeps may have them: a card writes such a name in that
// font, and a name neither of them can write is left out rather than drawn as
// boxes.
func TestTextTheCardCannotWriteIsLeftOut(t *testing.T) {
	fonts := shellTestApp(t).cardFonts()
	if !covers(fonts.regular, "Every frame counts 2026") {
		t.Fatalf("the Go font cannot write Latin: %v", fonts.err)
	}
	if covers(fonts.regular, "メモラ") {
		t.Fatal("the Go font claims to have letters it has none of")
	}
	card := newOGCard(fonts)
	if !card.says("Memora") {
		t.Fatal("the card would leave out a name it can draw")
	}
	// Whatever the machine keeps: the name is either written in that font or,
	// where there is none, left off the card. It is never drawn as boxes.
	other := fonts.other()
	switch {
	case other == nil && card.says("メモラ"):
		t.Fatal("the card would draw a name it has no font for")
	case other != nil && !card.says("メモラ"):
		t.Fatal("the card has a font for this name and would not use it")
	case other != nil && fonts.fontFor("メモラ") != other:
		t.Fatal("a name the Go font cannot write is not written in the machine's font")
	}
}

// A card the machine has a font for draws a site named in Chinese, and it is
// drawn rather than left blank.
func TestASiteNamedInChineseIsWrittenOnItsCard(t *testing.T) {
	app := shellTestApp(t)
	if other := app.cardFonts().other(); other == nil {
		t.Skip("this machine keeps no font the card can read outside Latin")
	}
	app.setSetting("app", "title", "陳年影格")
	plain := getCard(t, app, siteCardAddress).Body.Bytes()
	app.setSetting("app", "title", "")
	if defaulted := getCard(t, app, siteCardAddress).Body.Bytes(); bytes.Equal(plain, defaulted) {
		t.Fatal("the card of a site named in Chinese is the card of one that is not")
	}
}

// A font the operator named that cannot be read is no reason for links to
// arrive without a card: the card is drawn in the font that ships instead.
func TestABrokenCardFontStillLeavesACard(t *testing.T) {
	app := shellTestApp(t)
	app.cfg.OGFont = filepath.Join(t.TempDir(), "not-here.ttf")
	app.setSetting("app", "title", "Memora Test")
	decodeCard(t, getCard(t, app, siteCardAddress))
	fonts := app.cardFonts()
	if fonts.err != nil {
		t.Fatalf("a font that could not be read left the card without one: %v", fonts.err)
	}
	if !covers(fonts.regular, "Memora") {
		t.Fatal("the card cannot write even in the font that ships with it")
	}
}

// A font the operator names is the one the card is set in.
func TestTheCardFontCanBeNamedByTheOperator(t *testing.T) {
	app := shellTestApp(t)
	path := filepath.Join(t.TempDir(), "card.ttf")
	if err := os.WriteFile(path, goregular.TTF, 0o644); err != nil {
		t.Fatal(err)
	}
	app.cfg.OGFont = path
	fonts := app.cardFonts()
	if fonts.err != nil {
		t.Fatalf("naming a font the card could read: %v", fonts.err)
	}
	if !fonts.single || fonts.fontFor("Memora") != fonts.regular {
		t.Fatal("the named font is not the one the card is drawn in")
	}
	decodeCard(t, getCard(t, app, siteCardAddress))
}

func getCard(t *testing.T, app *App, target string) *httptest.ResponseRecorder {
	t.Helper()
	response := httptest.NewRecorder()
	app.ServeHTTP(response, httptest.NewRequest(http.MethodGet, target, nil))
	return response
}

func decodeCard(t *testing.T, response *httptest.ResponseRecorder) image.Image {
	t.Helper()
	if response.Code != http.StatusOK {
		t.Fatalf("the card = %d, %s", response.Code, strings.TrimSpace(response.Body.String()))
	}
	if contentType := response.Header().Get("Content-Type"); contentType != "image/jpeg" {
		t.Fatalf("the card answered as %q, want a JPEG", contentType)
	}
	picture, format, err := image.Decode(bytes.NewReader(response.Body.Bytes()))
	if err != nil {
		t.Fatalf("decoding the card: %v", err)
	}
	if format != "jpeg" {
		t.Fatalf("the card is a %s, want a JPEG", format)
	}
	return picture
}

// writeTestPicture puts a picture in the site's storage, the way an upload
// would, so the card has something of the site's to draw.
func writeTestPicture(t *testing.T, app *App, key string, fill color.RGBA) {
	t.Helper()
	picture := image.NewRGBA(image.Rect(0, 0, 64, 40))
	draw.Draw(picture, picture.Bounds(), &image.Uniform{fill}, image.Point{}, draw.Src)
	var buffer bytes.Buffer
	if err := png.Encode(&buffer, picture); err != nil {
		t.Fatal(err)
	}
	if _, err := app.storage.Create(context.Background(), key, buffer.Bytes(), "image/png"); err != nil {
		t.Fatal(err)
	}
}
