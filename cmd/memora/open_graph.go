package main

import (
	"bytes"
	"context"
	"fmt"
	"html"
	"net/http"
	"path/filepath"
	"strings"
)

// defaultSocialImage is what a link preview falls back to when the site has no
// avatar of its own. It ships with the client bundle.
const defaultSocialImage = "/web-app-manifest-512x512.png"

// defaultSiteTitle stands in when the settings cannot be read at all. It is the
// same title the settings table ships with.
const defaultSiteTitle = "Memora"

// clientRoutes are the addresses the client answers with a page of its own
// before it reads the first path segment as a photo id. A photo whose id
// happens to match one of them is opened through its own address, so those
// segments describe the page rather than the photo.
var clientRoutes = map[string]bool{
	"albums":     true,
	"dashboard":  true,
	"globe":      true,
	"onboarding": true,
	"signin":     true,
}

// socialPreview is what a link shared outside the browser shows: the tags a
// chat client, a search engine or a timeline reads out of the page.
type socialPreview struct {
	title       string
	site        string
	description string
	image       string
	kind        string
	card        string
}

// annotatePage puts the link preview into the client shell. The shell is built
// without a title of its own, and the client that would fill one in never runs
// for the crawler that builds a preview, so a shared link would otherwise show
// a bare address. The tags are added per request, which is also what keeps a
// photo's own title and picture out of the page when the album it lives in is
// not public.
func (a *App) annotatePage(page []byte, r *http.Request) []byte {
	index := bytes.Index(page, []byte("</head>"))
	if index < 0 {
		return page
	}
	head := a.socialHead(r)
	annotated := make([]byte, 0, len(page)+len(head))
	annotated = append(annotated, page[:index]...)
	annotated = append(annotated, head...)
	return append(annotated, page[index:]...)
}

// socialHead renders the tags a preview reads.
func (a *App) socialHead(r *http.Request) string {
	preview := a.socialPreviewFor(r.Context(), r)
	origin := requestOrigin(r)
	var head strings.Builder
	head.WriteString("<title>")
	head.WriteString(html.EscapeString(preview.title))
	head.WriteString("</title>")
	writeMetaTag(&head, "name", "description", preview.description)
	writeMetaTag(&head, "property", "og:site_name", preview.site)
	writeMetaTag(&head, "property", "og:title", preview.title)
	writeMetaTag(&head, "property", "og:description", preview.description)
	writeMetaTag(&head, "property", "og:type", preview.kind)
	writeMetaTag(&head, "property", "og:url", absoluteURL(origin, r.URL.Path))
	writeMetaTag(&head, "property", "og:image", absoluteURL(origin, preview.image))
	writeMetaTag(&head, "name", "twitter:card", preview.card)
	return head.String()
}

// socialPreviewFor describes the page a shared link points at: one photo when
// the address names a photo an anonymous visitor may see, the site itself
// otherwise.
func (a *App) socialPreviewFor(ctx context.Context, r *http.Request) socialPreview {
	site, slogan, avatar := a.siteIdentity(ctx)
	preview := socialPreview{
		title:       site,
		site:        site,
		description: slogan,
		image:       avatar,
		kind:        "website",
		// A site avatar is square, which is the shape the small card shows.
		card: "summary",
	}
	if preview.image == "" {
		preview.image = defaultSocialImage
	}
	id := photoIDFromPath(r.URL.Path)
	if id == "" {
		return preview
	}
	photo, ok := a.publicPhoto(ctx, id)
	if !ok {
		return preview
	}
	if title := stringValue(photo.Title); title != "" {
		preview.title = title
	}
	if description := stringValue(photo.Description); description != "" {
		preview.description = description
	}
	if image := firstNonEmpty(stringValue(photo.ThumbnailURL), stringValue(photo.OriginalURL)); image != "" {
		preview.image = image
	}
	preview.kind = "article"
	preview.card = "summary_large_image"
	return preview
}

// siteIdentity reads what the site calls itself. It comes from the public
// snapshot the client is served, which is warmed once and kept current.
func (a *App) siteIdentity(ctx context.Context) (site, slogan, avatar string) {
	site = defaultSiteTitle
	snapshot, err := a.publicSettingsSnapshot(ctx)
	if err != nil {
		return site, "", ""
	}
	values := snapshot["app"]
	return firstNonEmpty(stringValue(values["title"]), site), stringValue(values["slogan"]), stringValue(values["avatarUrl"])
}

// publicPhoto reads one photo an anonymous visitor is allowed to see. A preview
// is built for whoever the link is pasted in front of, so a photo an album
// keeps hidden or behind a password is not described at all.
func (a *App) publicPhoto(ctx context.Context, id string) (Photo, bool) {
	rows, err := a.db.QueryContext(ctx, photoSelect+` WHERE photos.id=? AND `+publicPhotoClause, id)
	if err != nil {
		return Photo{}, false
	}
	defer rows.Close()
	if !rows.Next() {
		return Photo{}, false
	}
	photo, err := scanPhoto(rows)
	if err != nil {
		return Photo{}, false
	}
	return photo, true
}

// photoIDFromPath returns the photo a request names, or "" for every other
// route. A shared photo is addressed by its id alone, which is what the client
// reads the first path segment as.
func photoIDFromPath(path string) string {
	trimmed := strings.Trim(path, "/")
	if trimmed == "" || strings.Contains(trimmed, "/") || filepath.Ext(trimmed) != "" || clientRoutes[trimmed] {
		return ""
	}
	return trimmed
}

// requestOrigin is the scheme and host the page was reached with. A preview
// image has to be an absolute address: the crawler that fetches it is not on
// this page and has nothing to resolve a relative one against.
func requestOrigin(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	if forwarded := forwardedValue(r, "X-Forwarded-Proto"); forwarded == "http" || forwarded == "https" {
		scheme = forwarded
	}
	host := firstNonEmpty(forwardedValue(r, "X-Forwarded-Host"), strings.TrimSpace(r.Host))
	if host == "" {
		return ""
	}
	return scheme + "://" + host
}

func forwardedValue(r *http.Request, header string) string {
	value, _, _ := strings.Cut(r.Header.Get(header), ",")
	return strings.TrimSpace(value)
}

// absoluteURL makes a stored address absolute, and leaves one that already is
// alone. With no origin — a request without a host — the address stays relative
// rather than naming a server that was never reached.
func absoluteURL(origin, address string) string {
	if address == "" || origin == "" || strings.HasPrefix(address, "//") ||
		strings.HasPrefix(address, "http://") || strings.HasPrefix(address, "https://") {
		return address
	}
	return origin + "/" + strings.TrimLeft(address, "/")
}

func writeMetaTag(head *strings.Builder, attribute, name, value string) {
	if value == "" {
		return
	}
	fmt.Fprintf(head, `<meta %s="%s" content="%s">`, attribute, name, html.EscapeString(value))
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
