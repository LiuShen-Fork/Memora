package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"math"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"unicode"

	xdraw "golang.org/x/image/draw"
	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/font/sfnt"
	"golang.org/x/image/math/fixed"
)

// A link pasted into a chat client or a timeline is drawn by that client from
// the tags the page carries. The tags name one picture, and this is it: a card
// the server draws for the page being shared, so that a shared site looks like
// the site and a shared photo arrives with what the site knows about it.
//
// 1200x630 is the size every client agrees on for the large card.
const (
	ogCardWidth  = 1200
	ogCardHeight = 630
)

// The card is drawn in the colours the site is drawn in.
var (
	ogBackground = color.RGBA{0x0a, 0x0a, 0x0a, 0xff} // neutral-950
	ogPrimary    = color.RGBA{0xec, 0x48, 0x99, 0xff} // pink-500, the app's primary
	ogAccent     = color.RGBA{0xa8, 0x55, 0xf7, 0xff} // purple-500
	ogHeading    = color.RGBA{0xfa, 0xfa, 0xfa, 0xff} // neutral-50
	ogBody       = color.RGBA{0xd4, 0xd4, 0xd4, 0xff} // neutral-300
	ogMuted      = color.RGBA{0x73, 0x73, 0x73, 0xff} // neutral-500
)

// errNoCard says the address names nothing a card may describe.
var errNoCard = errors.New("no card for this address")

// siteCardAddress is where the card the site itself is shared with is served
// from, and photoCardAddress the card for one photo.
const siteCardAddress = "/_og/site.jpg"

func photoCardAddress(id string) string {
	return "/_og/photo/" + url.PathEscape(id) + ".jpg"
}

// ogCardFonts is the type cards are set in: the Go family, which ships inside
// the Go image library rather than as an asset of its own, unless the operator
// names a font file of their own in CFRAME_OG_FONT.
//
// The Go family has no Chinese, Japanese or Korean letters, which is what many
// of the sites this draws are named in. A card therefore also looks for a font
// the machine already has, and sets any text its own type cannot write in that
// — see fontFor. Text neither of them can write is left out rather than drawn
// as empty boxes.
type ogCardFonts struct {
	regular *sfnt.Font
	bold    *sfnt.Font
	// single says the card was given one font to write everything in, and a
	// font file usually carries one weight.
	single bool
	// elsewhere is the font the machine keeps, read the first time a card is
	// asked for a letter the card's own type has none of.
	elsewhere     *sfnt.Font
	elsewhereOnce sync.Once
	err           error
}

// cardFonts loads the type once per process. The paths are read at the same
// time, so a font given in the environment is in use for every card after it.
func (a *App) cardFonts() *ogCardFonts {
	a.ogFontOnce.Do(func() { loadCardFonts(&a.ogFonts, a.cfg.OGFont, a.logs) })
	return &a.ogFonts
}

// loadCardFonts fills in the type a card is set in, and says so in the log: a
// card that arrives without the site's name on it is worth explaining.
func loadCardFonts(fonts *ogCardFonts, named string, logs *LogBuffer) {
	regular, bold := goregular.TTF, gobold.TTF
	if path := strings.TrimSpace(named); path != "" {
		font, err := readCardFont(path)
		if err == nil {
			// One file sets the whole card, and a file usually carries one
			// weight, so its heavy lines are drawn twice.
			fonts.regular, fonts.bold, fonts.single = font, font, true
			logs.Add("og", "link preview cards are drawn in "+path)
			return
		}
		// A font that cannot be read is no reason for links to arrive without a
		// card at all.
		logs.Add("og", fmt.Sprintf("drawing link preview cards in the Go font: reading %s: %v", path, err))
	}
	var err error
	if fonts.regular, err = sfnt.Parse(regular); err != nil {
		fonts.err = err
		return
	}
	if fonts.bold, err = sfnt.Parse(bold); err != nil {
		fonts.err = err
		return
	}
	logs.Add("og", "link preview cards are drawn in the Go font, which has no Chinese, Japanese or Korean letters: a font this machine keeps stands in for those, and CFRAME_OG_FONT names a font of your own")
}

// cardFallbackFonts are the fonts a card looks for a font the machine already
// has, for the letters its own type has none of. They are the usual names in
// the usual places.
var cardFallbackFonts = []string{
	// Windows
	`C:\Windows\Fonts\msyh.ttc`,   // Microsoft YaHei
	`C:\Windows\Fonts\msyh.ttf`,   // Microsoft YaHei, before Windows 8
	`C:\Windows\Fonts\simhei.ttf`, // SimHei
	`C:\Windows\Fonts\meiryo.ttc`, // Meiryo
	`C:\Windows\Fonts\malgun.ttf`, // Malgun Gothic
	// macOS
	`/System/Library/Fonts/PingFang.ttc`,
	`/System/Library/Fonts/Hiragino Sans GB.ttc`,
	`/System/Library/Fonts/AppleSDGothicNeo.ttc`,
	// Linux
	`/usr/share/fonts/opentype/noto/NotoSansCJK-Regular.ttc`,
	`/usr/share/fonts/noto-cjk/NotoSansCJK-Regular.ttc`,
	`/usr/share/fonts/opentype/noto/NotoSansCJKsc-Regular.otf`,
	`/usr/share/fonts/truetype/noto/NotoSansCJK-Regular.ttc`,
	`/usr/share/fonts/truetype/wqy/wqy-microhei.ttc`,
	`/usr/share/fonts/truetype/droid/DroidSansFallbackFull.ttf`,
	// Android
	`/system/fonts/NotoSansCJK-Regular.ttc`,
}

// other is a font the machine keeps, or nil where it keeps none the card can
// read. It is looked for only when a card needs it, so a site written entirely
// in Latin never reads it at all.
func (f *ogCardFonts) other() *sfnt.Font {
	f.elsewhereOnce.Do(func() {
		for _, path := range cardFallbackFonts {
			if font, err := readCardFont(path); err == nil {
				f.elsewhere = font
				return
			}
		}
	})
	return f.elsewhere
}

// readCardFont reads one font file. A file may hold a family of faces rather
// than one; the first is the one drawn.
func readCardFont(path string) (*sfnt.Font, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	font, parseErr := sfnt.Parse(data)
	if parseErr == nil {
		return font, nil
	}
	collection, collectionErr := sfnt.ParseCollection(data)
	if collectionErr != nil {
		return nil, parseErr
	}
	return collection.Font(0)
}

// fontFor is the font that writes text: the card's own type where it has the
// letters, the machine's font where only it has them, and nothing where neither
// does.
func (f *ogCardFonts) fontFor(text string) *sfnt.Font {
	if f.err != nil || f.regular == nil {
		return nil
	}
	if covers(f.regular, text) {
		return f.regular
	}
	if other := f.other(); other != nil && covers(other, text) {
		return other
	}
	return nil
}

// covers reports whether one font has every letter of text.
func covers(font *sfnt.Font, text string) bool {
	var buffer sfnt.Buffer
	for _, r := range text {
		if unicode.IsSpace(r) {
			continue
		}
		index, err := font.GlyphIndex(&buffer, r)
		if err != nil || index == 0 {
			return false
		}
	}
	return true
}

// face is a font at one size. Faces are made per card rather than shared: a
// face carries the buffer its font is read through.
func (f *ogCardFonts) face(chosen *sfnt.Font, size float64, bold bool) (font.Face, error) {
	if f.err != nil {
		return nil, f.err
	}
	if bold && chosen == f.regular && f.bold != nil {
		chosen = f.bold
	}
	if chosen == nil {
		return nil, errors.New("no card font is loaded")
	}
	return opentype.NewFace(chosen, &opentype.FaceOptions{Size: size, DPI: 72, Hinting: font.HintingFull})
}

// strokes is where a line is drawn to look heavy. The Go family carries a
// weight of its own for that; a font that came as a single file has none, so a
// heavy line is drawn once more a pixel along.
func (f *ogCardFonts) strokes(chosen *sfnt.Font, bold bool) []int {
	if !bold {
		return []int{0}
	}
	if f.single || (chosen != nil && chosen == f.elsewhere) {
		return []int{0, 1}
	}
	return []int{0}
}

// ogCard is one card being drawn.
type ogCard struct {
	img     *image.RGBA
	fonts   *ogCardFonts
	faces   map[ogFaceKey]font.Face
	missing bool
}

type ogFaceKey struct {
	font *sfnt.Font
	size float64
	bold bool
}

func newOGCard(fonts *ogCardFonts) *ogCard {
	img := image.NewRGBA(image.Rect(0, 0, ogCardWidth, ogCardHeight))
	draw.Draw(img, img.Bounds(), &image.Uniform{ogBackground}, image.Point{}, draw.Src)
	return &ogCard{img: img, fonts: fonts, faces: map[ogFaceKey]font.Face{}}
}

// face is text's font at one size, made once per card and kept: a face carries
// the buffer its font is read through.
func (c *ogCard) face(text string, size float64, bold bool) font.Face {
	chosen := c.fonts.fontFor(text)
	if chosen == nil {
		return nil
	}
	key := ogFaceKey{font: chosen, size: size, bold: bold}
	if face, ok := c.faces[key]; ok {
		return face
	}
	face, err := c.fonts.face(chosen, size, bold)
	if err != nil {
		c.missing = true
		return nil
	}
	c.faces[key] = face
	return face
}

// says reports whether the card has a font that can print text. Text it cannot
// write is left out rather than printed as empty boxes.
func (c *ogCard) says(text string) bool {
	return strings.TrimSpace(text) != "" && c.fonts.fontFor(text) != nil
}

// glow lights the card the way the setup screens light their background: one
// soft circle of light whose strength falls away with the distance from its
// centre. The drawing package has no radial gradient, so the falloff is built
// as a mask and the colour is laid through it once, which is also what keeps
// the light even rather than piling up where the circles would overlap.
func (c *ogCard) glow(cx, cy, radius int, tint color.RGBA, strength float64) {
	if radius <= 0 || strength <= 0 {
		return
	}
	mask := image.NewAlpha(image.Rect(cx-radius, cy-radius, cx+radius, cy+radius))
	for y := mask.Rect.Min.Y; y < mask.Rect.Max.Y; y++ {
		for x := mask.Rect.Min.X; x < mask.Rect.Max.X; x++ {
			dx, dy := float64(x-cx), float64(y-cy)
			distance := math.Sqrt(dx*dx+dy*dy) / float64(radius)
			if distance >= 1 {
				continue
			}
			falloff := 1 - distance
			mask.SetAlpha(x, y, color.Alpha{A: uint8(strength * falloff * falloff * 255)})
		}
	}
	draw.DrawMask(c.img, mask.Rect, &image.Uniform{tint}, image.Point{}, mask, mask.Rect.Min, draw.Over)
}

// shade is a colour at part strength. The drawing package reads colour channels
// as already multiplied by the alpha channel, so a colour that is faded has to
// have its channels faded with it: a colour that kept them would be laid on at
// full strength however small its alpha, and one brighter than the alpha would
// overflow the channel it is drawn into.
func shade(tint color.RGBA, alpha float64) color.RGBA {
	if alpha < 0 {
		alpha = 0
	}
	if alpha > 1 {
		alpha = 1
	}
	return color.RGBA{
		R: uint8(float64(tint.R) * alpha),
		G: uint8(float64(tint.G) * alpha),
		B: uint8(float64(tint.B) * alpha),
		A: uint8(alpha * 255),
	}
}

// cover draws picture into rect the way object-fit: cover does: it fills the
// rect, cropping the overflow rather than leaving a gap. A radius rounds the
// corners off.
func (c *ogCard) cover(picture image.Image, rect image.Rectangle, radius int) {
	if picture == nil || rect.Dx() <= 0 || rect.Dy() <= 0 {
		return
	}
	bounds := picture.Bounds()
	if bounds.Dx() <= 0 || bounds.Dy() <= 0 {
		return
	}
	scale := maxFloat(float64(rect.Dx())/float64(bounds.Dx()), float64(rect.Dy())/float64(bounds.Dy()))
	cropW := minFloat(float64(bounds.Dx()), float64(rect.Dx())/scale)
	cropH := minFloat(float64(bounds.Dy()), float64(rect.Dy())/scale)
	cropX := float64(bounds.Min.X) + (float64(bounds.Dx())-cropW)/2
	cropY := float64(bounds.Min.Y) + (float64(bounds.Dy())-cropH)/2
	source := image.Rect(roundInt(cropX), roundInt(cropY), roundInt(cropX+cropW), roundInt(cropY+cropH))
	if radius <= 0 {
		xdraw.CatmullRom.Scale(c.img, rect, picture, source, draw.Src, nil)
		return
	}
	// A rounded picture is drawn on a canvas of its own and stamped through a
	// mask, so its corners are cut out rather than painted over.
	stamp := image.NewRGBA(image.Rect(0, 0, rect.Dx(), rect.Dy()))
	xdraw.CatmullRom.Scale(stamp, stamp.Bounds(), picture, source, draw.Src, nil)
	mask := image.NewAlpha(stamp.Bounds())
	draw.Draw(mask, mask.Bounds(), &image.Uniform{color.Alpha{A: 0xff}}, image.Point{}, draw.Src)
	cutCorners(mask, radius)
	draw.DrawMask(c.img, rect, stamp, image.Point{}, mask, image.Point{}, draw.Over)
}

// cutCorners clears the pixels outside the rounded corners of a mask: the four
// quarter circles are cut away from the corner each one sits in, outwards.
func cutCorners(mask *image.Alpha, radius int) {
	bounds := mask.Bounds()
	for _, corner := range [...]struct{ x, y, across, down int }{
		{bounds.Min.X + radius, bounds.Min.Y + radius, -1, -1},
		{bounds.Max.X - 1 - radius, bounds.Min.Y + radius, 1, -1},
		{bounds.Min.X + radius, bounds.Max.Y - 1 - radius, -1, 1},
		{bounds.Max.X - 1 - radius, bounds.Max.Y - 1 - radius, 1, 1},
	} {
		for down := 0; down <= radius; down++ {
			for across := 0; across <= radius; across++ {
				if across*across+down*down <= radius*radius {
					continue
				}
				mask.SetAlpha(corner.x+corner.across*across, corner.y+corner.down*down, color.Alpha{})
			}
		}
	}
}

// fade darkens a strip across the card, from solid at one end to clear at the
// other, so a picture and the panel beside it read as one card.
func (c *ogCard) fade(rect image.Rectangle, from, to float64) {
	if rect.Dx() <= 1 {
		return
	}
	span := rect.Max.X - rect.Min.X
	for i := 0; i < span; i++ {
		alpha := from + (to-from)*float64(i)/float64(span-1)
		if alpha <= 0 {
			continue
		}
		strip := image.Rect(rect.Min.X+i, rect.Min.Y, rect.Min.X+i+1, rect.Max.Y)
		draw.Draw(c.img, strip, &image.Uniform{shade(ogBackground, alpha)}, image.Point{}, draw.Over)
	}
}

// lines breaks text into the lines that fit width.
func (c *ogCard) lines(text string, size float64, bold bool, width int) []string {
	face := c.face(text, size, bold)
	words := strings.Fields(text)
	if face == nil || len(words) == 0 {
		return nil
	}
	var lines []string
	current := ""
	for _, word := range words {
		candidate := word
		if current != "" {
			candidate = current + " " + word
		}
		if font.MeasureString(face, candidate).Round() <= width {
			current = candidate
			continue
		}
		if current != "" {
			lines = append(lines, current)
		}
		// A single word too long for the line is cut where it stops fitting.
		if font.MeasureString(face, word).Round() > width {
			lines = append(lines, c.truncate(word, size, bold, width))
			current = ""
			continue
		}
		current = word
	}
	if current != "" {
		lines = append(lines, current)
	}
	return lines
}

// fit keeps at most limit of the lines and marks the last one when the text had
// more to say than the lines show.
func (c *ogCard) fit(lines []string, size float64, bold bool, width, limit int) []string {
	if limit <= 0 || len(lines) <= limit {
		return lines
	}
	kept := lines[:limit]
	kept[limit-1] = c.truncate(strings.TrimRight(kept[limit-1], " ")+" …", size, bold, width)
	return kept
}

// truncate shortens text to what fits in width, ending it with an ellipsis.
func (c *ogCard) truncate(text string, size float64, bold bool, width int) string {
	face := c.face(text, size, bold)
	if face == nil || width <= 0 {
		return ""
	}
	if font.MeasureString(face, text).Round() <= width {
		return text
	}
	runes := []rune(text)
	kept := 0
	for length := len(runes) - 1; length > 0; length-- {
		if font.MeasureString(face, strings.TrimRight(string(runes[:length]), " ")+"…").Round() <= width {
			kept = length
			break
		}
	}
	if kept == 0 {
		return ""
	}
	line := strings.TrimRight(string(runes[:kept]), " ")
	// Cutting a word in half reads worse than stopping a word early, so a cut
	// inside a word goes back to the word before it — unless there is no word
	// before it, as in a single word too long for the line.
	if kept < len(runes) && !unicode.IsSpace(runes[kept]) {
		if space := strings.LastIndex(line, " "); space > 0 {
			line = line[:space]
		}
	}
	return line + "…"
}

func (c *ogCard) width(text string, size float64, bold bool) int {
	face := c.face(text, size, bold)
	if face == nil {
		return 0
	}
	return font.MeasureString(face, text).Round()
}

// draw puts one line of text on the card, with its baseline at y.
func (c *ogCard) draw(text string, x, y int, size float64, bold bool, colour color.RGBA) {
	face := c.face(text, size, bold)
	if face == nil || text == "" {
		return
	}
	drawer := &font.Drawer{Dst: c.img, Src: &image.Uniform{colour}, Face: face}
	for _, offset := range c.fonts.strokes(c.fonts.fontFor(text), bold) {
		drawer.Dot = fixed.P(x+offset, y)
		drawer.DrawString(text)
	}
}

// dot draws the small round mark that separates the pieces of a metadata line.
func (c *ogCard) dot(cx, cy, radius int, colour color.RGBA) {
	for y := -radius; y <= radius; y++ {
		for x := -radius; x <= radius; x++ {
			if x*x+y*y <= radius*radius {
				c.img.SetRGBA(cx+x, cy+y, colour)
			}
		}
	}
}

// encode writes the card out as a JPEG: a card carrying a photograph is far
// smaller this way than as a PNG, and every client that reads a preview reads
// JPEG.
func (c *ogCard) encode() ([]byte, error) {
	if c.missing {
		return nil, errors.New("no card font is loaded")
	}
	var buffer bytes.Buffer
	if err := jpeg.Encode(&buffer, c.img, &jpeg.Options{Quality: 90}); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

// ogCard serves the card addresses. What a card shows follows from the address
// and from whether the page it names is one an anonymous visitor may see, so a
// card can never describe a photo behind a password or in a hidden album.
func (a *App) ogCard(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		errorJSON(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}
	var (
		picture []byte
		err     error
	)
	switch {
	case r.URL.Path == siteCardAddress:
		picture, err = a.siteCard(r.Context())
	case strings.HasPrefix(r.URL.Path, "/_og/photo/"):
		id, unescapeErr := url.PathUnescape(strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/_og/photo/"), ".jpg"))
		if unescapeErr != nil {
			errorJSON(w, http.StatusNotFound, "Not found")
			return
		}
		picture, err = a.photoCard(r.Context(), id)
	default:
		errorJSON(w, http.StatusNotFound, "Not found")
		return
	}
	if err != nil {
		if errors.Is(err, errNoCard) {
			errorJSON(w, http.StatusNotFound, "Not found")
			return
		}
		a.logs.Add("og", fmt.Sprintf("drawing a link preview card: %v", err))
		errorJSON(w, http.StatusInternalServerError, "Unable to draw the preview")
		return
	}
	w.Header().Set("Content-Type", "image/jpeg")
	// A card is a picture of the site as it is now: worth keeping for a while,
	// and worth asking about after that.
	w.Header().Set("Cache-Control", "public, max-age=600")
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(picture)))
	if r.Method == http.MethodHead {
		return
	}
	_, _ = w.Write(picture)
}

// siteCard draws the card the site itself is shared with: what the site is
// called, what it says about itself, and the picture it goes by.
func (a *App) siteCard(ctx context.Context) ([]byte, error) {
	site, slogan, avatar := a.siteIdentity(ctx)
	card := newOGCard(a.cardFonts())
	card.glow(180, 60, 780, ogPrimary, 0.32)
	card.glow(1160, 660, 640, ogAccent, 0.24)

	const (
		margin = 96
		side   = 168
		gap    = 52
	)
	pictured := false
	if picture := a.cardImage(ctx, avatar); picture != nil {
		top := (ogCardHeight - side) / 2
		card.cover(picture, image.Rect(margin, top, margin+side, top+side), 44)
		pictured = true
	}

	x, width := margin, ogCardWidth-2*margin
	if pictured {
		x, width = margin+side+gap, ogCardWidth-(margin+side+gap)-margin
	}

	// The blocks are stacked and then centred as a group, so a site with a
	// slogan and a site without both sit in the middle of the card.
	type block struct {
		lines []string
		size  float64
		lead  int
		bold  bool
		ink   color.RGBA
	}
	var blocks []block
	if card.says(site) {
		title := card.truncate(site, 68, true, width)
		blocks = append(blocks, block{[]string{title}, 68, 84, true, ogHeading})
	}
	if card.says(slogan) {
		blocks = append(blocks, block{card.fit(card.lines(slogan, 32, false, width), 32, false, width, 2), 32, 44, false, ogBody})
	}
	height := 0
	for _, block := range blocks {
		height += block.lead * len(block.lines)
	}
	y := (ogCardHeight+height)/2 - height + 24
	for index, block := range blocks {
		if index > 0 {
			y += 12
		}
		for _, line := range block.lines {
			card.draw(line, x, y, block.size, block.bold, block.ink)
			y += block.lead
		}
	}
	return card.encode()
}

// photoCard draws the card one photo is shared with: the photo itself, and what
// the site knows about it.
func (a *App) photoCard(ctx context.Context, id string) ([]byte, error) {
	photo, ok := a.publicPhoto(ctx, strings.TrimSpace(id))
	if !ok {
		return nil, errNoCard
	}
	site, slogan, _ := a.siteIdentity(ctx)
	card := newOGCard(a.cardFonts())

	// The picture takes the right of the card and is faded into the panel
	// beside it, so the two read as one picture rather than two halves.
	const panel = ogCardWidth * 3 / 5
	if picture := a.cardImage(ctx, firstNonEmpty(stringValue(photo.ThumbnailKey), stringValue(photo.ThumbnailURL), stringValue(photo.OriginalURL))); picture != nil {
		card.cover(picture, image.Rect(panel-140, 0, ogCardWidth, ogCardHeight), 0)
		card.fade(image.Rect(panel-140, 0, panel+200, ogCardHeight), 1, 0)
	} else {
		card.glow(1060, 320, 660, ogPrimary, 0.26)
	}

	const (
		margin = 96
		width  = panel - margin - 44
	)
	// The site is the eyebrow over the photo's own name. A photo with no name of
	// its own is named by the site, and then the eyebrow would only repeat it.
	title := firstNonEmpty(stringValue(photo.Title), site)
	if card.says(title) {
		if title != site && card.says(site) {
			card.draw(strings.ToUpper(card.truncate(site, 24, false, width)), margin, 150, 24, false, ogPrimary)
		}
		y := 250
		for _, line := range card.fit(card.lines(title, 56, true, width), 56, true, width, 2) {
			card.draw(line, margin, y, 56, true, ogHeading)
			y += 70
		}
	}
	// What the site says about itself stands in for a photo nobody described,
	// which is also what the page's own description says.
	if description := firstNonEmpty(stringValue(photo.Description), slogan); card.says(description) {
		y := 430
		for _, line := range card.fit(card.lines(description, 28, false, width), 28, false, width, 2) {
			card.draw(line, margin, y, 28, false, ogBody)
			y += 38
		}
	}
	card.metaLine(photoFacts(photo), margin, 552, width)
	return card.encode()
}

// metaLine draws the facts about a photo along one line, separated by dots,
// dropping whatever does not fit.
func (c *ogCard) metaLine(pieces []string, x, y, width int) {
	const size = 26
	pen := x
	for index, piece := range pieces {
		if !c.says(piece) {
			continue
		}
		marker := 0
		if index > 0 {
			marker = c.width("   ", size, false) + 16
		}
		text := c.truncate(piece, size, false, x+width-pen-marker)
		if text == "" {
			break
		}
		if marker > 0 {
			pen += c.width("   ", size, false)
			c.dot(pen+4, y-9, 3, ogMuted)
			pen += 16
		}
		c.draw(text, pen, y, size, false, ogBody)
		pen += c.width(text, size, false)
	}
}

// photoFacts is what a photo card prints along the bottom: where it was taken,
// what took it, and how it was set.
func photoFacts(photo Photo) []string {
	var pieces []string
	for _, place := range []string{stringValue(photo.City), stringValue(photo.LocationName), stringValue(photo.Country)} {
		if strings.TrimSpace(place) != "" {
			pieces = append(pieces, place)
			break
		}
	}
	exif, _ := photo.Exif.(map[string]any)
	if camera := cameraName(exif); camera != "" {
		pieces = append(pieces, camera)
	}
	for _, value := range []string{
		focalLength(exifValue(exif, "FocalLengthIn35mmFormat", "FocalLength")),
		prefix(exifValue(exif, "FNumber"), "f/"),
		suffix(exifValue(exif, "ExposureTime"), "s"),
		prefix(exifValue(exif, "ISO"), "ISO "),
	} {
		if value != "" {
			pieces = append(pieces, value)
		}
	}
	return pieces
}

// exifValue reads a fact from a photo's metadata, which is stored as it came
// out of the file. Keys are matched without regard to their case because the
// spelling differs between the tools that write them.
func exifValue(exif map[string]any, names ...string) string {
	for _, name := range names {
		for key, raw := range exif {
			if !strings.EqualFold(key, name) {
				continue
			}
			if value := strings.TrimSpace(stringValue(raw)); value != "" && value != "—" {
				return value
			}
		}
	}
	return ""
}

// focalLength writes a focal length the way it is said out loud, in
// millimetres. A file that already says the unit is left as it is.
func focalLength(value string) string {
	if value == "" || strings.HasSuffix(value, "mm") || strings.Trim(value, "0123456789.") != "" {
		return value
	}
	return value + "mm"
}

func prefix(value, lead string) string {
	if value == "" || strings.HasPrefix(value, lead) {
		return value
	}
	return lead + value
}

func suffix(value, tail string) string {
	if value == "" || strings.HasSuffix(value, tail) {
		return value
	}
	return value + tail
}

// cameraName names a camera the way it is written on its body, without
// repeating the make when the model already says it.
func cameraName(exif map[string]any) string {
	make := exifValue(exif, "Make")
	model := exifValue(exif, "Model")
	if model == "" {
		return make
	}
	if make == "" {
		return model
	}
	if first := strings.Fields(make)[0]; strings.Contains(strings.ToLower(model), strings.ToLower(first)) {
		return model
	}
	return make + " " + model
}

// cardPictureArea is the largest picture a card reads, in pixels. A card is a
// small picture drawn for anyone who is sent the link, so an address that turns
// out to hold more than this is one the server does not hold in memory for it.
const cardPictureArea = 64 << 20

// cardImage reads one picture for a card. A picture that cannot be read leaves
// the card without one rather than leaving it undrawn: the card is still worth
// sharing.
func (a *App) cardImage(ctx context.Context, address string) image.Image {
	data, err := a.cardImageBytes(ctx, strings.TrimSpace(address))
	if err != nil || len(data) == 0 {
		return nil
	}
	// The size is read before the picture is, so a file that says it is far
	// larger than a card needs is left unread rather than decoded in full.
	if config, _, err := image.DecodeConfig(bytes.NewReader(data)); err != nil || config.Width*config.Height > cardPictureArea {
		return nil
	}
	picture, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil
	}
	return picture
}

// cardImageBytes reads the bytes behind a picture address: a key in the site's
// storage, an address under the site itself, a file that ships with the client,
// or somewhere else on the network.
func (a *App) cardImageBytes(ctx context.Context, address string) ([]byte, error) {
	if address == "" {
		return nil, nil
	}
	if strings.HasPrefix(address, "http://") || strings.HasPrefix(address, "https://") {
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
		if err != nil {
			return nil, err
		}
		response, err := externalHTTPClient.Do(request)
		if err != nil {
			return nil, err
		}
		defer response.Body.Close()
		if response.StatusCode < 200 || response.StatusCode >= 300 {
			return nil, fmt.Errorf("reading %s: %s", address, response.Status)
		}
		return readLimited(response.Body, a.mediaLimit())
	}
	if strings.HasPrefix(address, "/storage/") || strings.HasPrefix(address, "/image/") {
		key := strings.TrimPrefix(strings.TrimPrefix(address, "/storage/"), "/image/")
		return a.readStorageBytes(ctx, key)
	}
	if strings.HasPrefix(address, "/") {
		// An address under this server names a file the client is served, or a
		// picture the server reads from its own storage.
		path := filepath.Join(a.cfg.WebDir, filepath.FromSlash(strings.TrimLeft(address, "/")))
		if file, err := os.Open(path); err == nil {
			defer file.Close()
			if info, statErr := file.Stat(); statErr == nil && info.Mode().IsRegular() {
				return readLimited(file, a.mediaLimit())
			}
		}
		return a.readStorageBytes(ctx, strings.TrimLeft(address, "/"))
	}
	return a.readStorageBytes(ctx, address)
}

func maxFloat(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

func minFloat(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

func roundInt(value float64) int {
	return int(value + 0.5)
}
