package main

import (
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

// An album's slug is the readable half of its address: /albums/kyoto-2024
// rather than /albums/7. The number stays a valid address for good, so a link
// written before slugs existed keeps working; a slug an album has stopped using
// is kept too, and answers with the album it now points at so a shared link
// does not rot.
//
// A slug may be written in any script. A gallery of Chinese titles would
// otherwise have nothing but album-3, album-4 to offer, and a browser shows the
// characters back to the reader even though the URL carries them percent
// encoded. Letters, digits and single hyphens between them are what is allowed;
// an address made only of digits is left to album IDs.
const albumSlugMaxLength = 80

var (
	errAlbumSlugTaken   = errors.New("Album URL slug is in use")
	errAlbumSlugInvalid = errors.New("Invalid album URL slug")
)

// albumSlugStatements create the storage the slugs need. The unique index is
// here rather than in ensureSchema because the column it indexes is added by
// migrateSchema, which runs first; a database that predates slugs would fail to
// create the index before its column existed.
var albumSlugStatements = []string{
	`CREATE TABLE IF NOT EXISTS album_slug_history (slug TEXT PRIMARY KEY, album_id INTEGER NOT NULL REFERENCES albums(id) ON DELETE CASCADE, created_at INTEGER NOT NULL DEFAULT (unixepoch()))`,
	`CREATE INDEX IF NOT EXISTS album_slug_history_album_idx ON album_slug_history(album_id)`,
	`CREATE UNIQUE INDEX IF NOT EXISTS albums_slug_idx ON albums(slug) WHERE slug IS NOT NULL AND slug <> ''`,
}

// albumSlugStep adds the column an album's current slug lives in. An install
// that predates slugs gets it here and is then given a slug per album by
// backfillAlbumSlugs; a fresh install creates albums with the column already.
var albumSlugStep = schemaStep{
	table:      "albums",
	column:     "slug",
	definition: "TEXT",
}

// writeAlbumSlugError answers a refused slug. A slug another album already
// answers to is a conflict rather than a malformed request, and the dashboard
// tells the two apart so it can say which one happened.
func writeAlbumSlugError(w http.ResponseWriter, err error) {
	if errors.Is(err, errAlbumSlugTaken) {
		errorJSON(w, http.StatusConflict, err.Error())
		return
	}
	errorJSON(w, http.StatusBadRequest, err.Error())
}

// isNumericAlbumPathSegment reports whether an address is an album ID rather
// than a slug. Only ASCII digits count, which is exactly what the route parses.
func isNumericAlbumPathSegment(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// canonicalAlbumSlug folds what someone typed into the form a slug is stored
// in: compatibility forms are folded to their plain characters (so full-width
// letters in a title are letters, not punctuation) and case is dropped, because
// a URL that differs only by case invites two links to the same album.
func canonicalAlbumSlug(input string) string {
	return strings.ToLower(strings.TrimSpace(norm.NFKC.String(input)))
}

// validAlbumSlug reports whether a canonical slug may be stored. The rules are
// deliberately narrow: letters and digits of any script, joined by single
// hyphens, short enough to stay readable, and never a number, which is how an
// album ID is written.
func validAlbumSlug(slug string) bool {
	if slug == "" || isNumericAlbumPathSegment(slug) {
		return false
	}
	count := 0
	previousDash := true
	for _, r := range slug {
		count++
		if count > albumSlugMaxLength {
			return false
		}
		if r == '-' {
			if previousDash {
				return false
			}
			previousDash = true
			continue
		}
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) {
			return false
		}
		previousDash = false
	}
	return !previousDash
}

// generatedAlbumSlug turns a title into a slug. Everything that is not a letter
// or a digit becomes a hyphen, so the result reads the way the title does; a
// title with nothing usable in it (emoji, punctuation) falls back to the name
// the caller knows the album by.
func generatedAlbumSlug(title, fallback string) string {
	var out strings.Builder
	count := 0
	previousDash := true
	for _, r := range canonicalAlbumSlug(title) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			if count >= albumSlugMaxLength {
				break
			}
			out.WriteRune(r)
			count++
			previousDash = false
			continue
		}
		if !previousDash && count < albumSlugMaxLength {
			out.WriteByte('-')
			count++
			previousDash = true
		}
	}
	slug := strings.TrimRight(out.String(), "-")
	if slug == "" || isNumericAlbumPathSegment(slug) {
		return fallback
	}
	return slug
}

// suffixedAlbumSlug is the nth candidate for an automatically chosen slug. The
// base is shortened when it has to be, so the number always fits inside the
// limit rather than being cut off with the rest.
func suffixedAlbumSlug(base string, suffix int) string {
	if suffix == 0 {
		return base
	}
	ending := "-" + strconv.Itoa(suffix)
	room := albumSlugMaxLength - len([]rune(ending))
	trimmed := []rune(strings.TrimRight(base, "-"))
	if room < 1 {
		room = 1
	}
	if len(trimmed) > room {
		trimmed = trimmed[:room]
	}
	return strings.TrimRight(string(trimmed), "-") + ending
}

// claimAlbumSlug decides which slug an album is addressed by and stores it. An
// empty request means "make one from the title"; a request with something in it
// is taken as chosen and is refused, rather than quietly reworded, when another
// album already answers to it. The album's previous slug moves to the history
// table in the same transaction, so the two can never disagree.
//
// Must run inside the caller's transaction.
func (a *App) claimAlbumSlug(tx *sql.Tx, albumID int64, title, requested string) (string, error) {
	automatic := strings.TrimSpace(requested) == ""
	base := ""
	if automatic {
		base = generatedAlbumSlug(title, fmt.Sprintf("album-%d", albumID))
	} else {
		base = canonicalAlbumSlug(requested)
		if !validAlbumSlug(base) {
			return "", errAlbumSlugInvalid
		}
	}
	for suffix := 0; suffix < 10000; suffix++ {
		candidate := base
		if automatic {
			candidate = suffixedAlbumSlug(base, suffix)
		}
		var owner int64
		err := tx.QueryRow(`SELECT id FROM albums WHERE slug=?`, candidate).Scan(&owner)
		switch {
		case err == nil && owner == albumID:
			return candidate, nil
		case err == nil:
			if !automatic {
				return "", errAlbumSlugTaken
			}
			continue
		case !errors.Is(err, sql.ErrNoRows):
			return "", err
		}
		// A slug no album answers to may still be retired by one. Taking it back
		// removes the history row rather than leaving two records claiming it; an
		// automatic name that another album retired steps over it, because an old
		// link to that album should keep arriving there.
		var retiredOwner int64
		err = tx.QueryRow(`SELECT album_id FROM album_slug_history WHERE slug=?`, candidate).Scan(&retiredOwner)
		switch {
		case err == nil && automatic && retiredOwner != albumID:
			continue
		case err == nil:
			if _, err := tx.Exec(`DELETE FROM album_slug_history WHERE slug=?`, candidate); err != nil {
				return "", err
			}
		case !errors.Is(err, sql.ErrNoRows):
			return "", err
		}
		var previous sql.NullString
		if err := tx.QueryRow(`SELECT slug FROM albums WHERE id=?`, albumID).Scan(&previous); err != nil {
			return "", err
		}
		if previous.Valid && previous.String != "" && previous.String != candidate {
			if _, err := tx.Exec(`INSERT INTO album_slug_history(slug,album_id) VALUES(?,?) ON CONFLICT(slug) DO UPDATE SET album_id=excluded.album_id`, previous.String, albumID); err != nil {
				return "", err
			}
		}
		if _, err := tx.Exec(`UPDATE albums SET slug=? WHERE id=?`, candidate, albumID); err != nil {
			return "", err
		}
		return candidate, nil
	}
	return "", errAlbumSlugTaken
}

// resolveAlbumSegment maps what comes after /albums/ to an album. A number is
// an album ID and anything else is a slug; a slug the album has retired still
// resolves, and canonical says the address should be rewritten to the slug the
// album uses now.
func (a *App) resolveAlbumSegment(segment string) (albumID int64, slug string, canonical bool, ok bool) {
	segment = strings.TrimSpace(segment)
	if segment == "" {
		return 0, "", false, false
	}
	if isNumericAlbumPathSegment(segment) {
		id, err := strconv.ParseInt(segment, 10, 64)
		if err != nil || id < 1 {
			return 0, "", false, false
		}
		if err := a.db.QueryRow(`SELECT COALESCE(slug,'') FROM albums WHERE id=?`, id).Scan(&slug); err != nil {
			return 0, "", false, false
		}
		return id, slug, slug == "", true
	}
	segment = canonicalAlbumSlug(segment)
	if !validAlbumSlug(segment) {
		return 0, "", false, false
	}
	if err := a.db.QueryRow(`SELECT id FROM albums WHERE slug=?`, segment).Scan(&albumID); err == nil {
		return albumID, segment, true, true
	}
	if err := a.db.QueryRow(`SELECT h.album_id,COALESCE(al.slug,'') FROM album_slug_history h INNER JOIN albums al ON al.id=h.album_id WHERE h.slug=?`, segment).Scan(&albumID, &slug); err == nil {
		return albumID, slug, false, true
	}
	return 0, "", false, false
}

// backfillAlbumSlugs gives every album an install created before slugs a slug
// of its own, once. Album addresses already handed out keep working through
// their ID, so there is nothing to break by inventing the slugs here.
func (a *App) backfillAlbumSlugs() error {
	var applied int
	err := a.db.QueryRow(`SELECT 1 FROM schema_migrations WHERE name=?`, "album_slug_backfill").Scan(&applied)
	if err == nil {
		return nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	tx, err := a.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	rows, err := tx.Query(`SELECT id,title FROM albums WHERE slug IS NULL OR slug='' ORDER BY id ASC`)
	if err != nil {
		return err
	}
	type titledAlbum struct {
		id    int64
		title string
	}
	pending := []titledAlbum{}
	for rows.Next() {
		var album titledAlbum
		if err := rows.Scan(&album.id, &album.title); err != nil {
			_ = rows.Close()
			return err
		}
		pending = append(pending, album)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}
	for _, album := range pending {
		if _, err := a.claimAlbumSlug(tx, album.id, album.title, ""); err != nil {
			return fmt.Errorf("backfill albums.slug for %d: %w", album.id, err)
		}
	}
	if _, err := tx.Exec(`INSERT INTO schema_migrations(name) VALUES(?)`, "album_slug_backfill"); err != nil {
		return fmt.Errorf("record migration album_slug_backfill: %w", err)
	}
	return tx.Commit()
}
