package main

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"regexp"
	"strings"
	"time"

	"golang.org/x/crypto/scrypt"
)

// An album can be hidden, which keeps it for signed-in sessions only, and it
// can carry a password, which keeps it for whoever knows the password. The two
// are separate: a hidden album is not a protected one, and unlocking a
// protected album does not reveal a hidden one.
//
// Unlocking hands the browser a signed grant rather than a session, so a
// visitor without an account can open a protected album, and so the grant can
// be tied to the specific password it was given for. Changing an album's
// password therefore ends every grant made with the old one.

const (
	albumAccessCookie  = "album_access"
	albumGrantLifetime = 7 * 24 * time.Hour
	albumGrantMax      = 30
	albumGrantTokenMax = 4000

	albumPasswordMinLength = 6
	albumPasswordMaxLength = 128

	// The cost parameters match the defaults of Node's crypto.scrypt, which is
	// what an album password set before this fork was hashed with.
	albumScryptN         = 16384
	albumScryptR         = 8
	albumScryptP         = 1
	albumScryptKeyLength = 64
)

var (
	albumPasswordHashPattern = regexp.MustCompile(`^[a-f0-9]{128}$`)
	albumGrantHashPattern    = regexp.MustCompile(`^[a-f0-9]{32}$`)
)

// albumPasswordProblem reports why a password offered for an album is not
// acceptable, or "" when it is. The same rule guards creating and updating so
// the two cannot disagree.
func albumPasswordProblem(password string) string {
	if len(password) < albumPasswordMinLength || len(password) > albumPasswordMaxLength {
		return "password must be between 6 and 128 characters"
	}
	return ""
}

func hashAlbumPassword(password string) (string, error) {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	encoded := hex.EncodeToString(salt)
	derived, err := scrypt.Key([]byte(password), []byte(encoded), albumScryptN, albumScryptR, albumScryptP, albumScryptKeyLength)
	if err != nil {
		return "", err
	}
	return encoded + ":" + hex.EncodeToString(derived), nil
}

func verifyAlbumPassword(password, stored string) bool {
	salt, expected, found := strings.Cut(stored, ":")
	if !found || salt == "" || !albumPasswordHashPattern.MatchString(expected) {
		return false
	}
	derived, err := scrypt.Key([]byte(password), []byte(salt), albumScryptN, albumScryptR, albumScryptP, albumScryptKeyLength)
	if err != nil {
		return false
	}
	want, err := hex.DecodeString(expected)
	if err != nil {
		return false
	}
	return subtle.ConstantTimeCompare(derived, want) == 1
}

// albumGrant is one unlocked album, remembered in the visitor's cookie. The
// hash is derived from the album's password hash, so it stops matching as soon
// as the password changes, without the server having to track anything.
type albumGrant struct {
	ID        int64  `json:"id"`
	Hash      string `json:"hash"`
	ExpiresAt int64  `json:"expiresAt"`
}

func (a *App) albumPasswordVersion(passwordHash string) string {
	mac := hmac.New(sha256.New, a.cfg.SessionKey)
	mac.Write([]byte("album-password-version\x00"))
	mac.Write([]byte(passwordHash))
	return hex.EncodeToString(mac.Sum(nil))[:32]
}

func (a *App) albumGrantSignature(payload string) string {
	mac := hmac.New(sha256.New, a.cfg.SessionKey)
	mac.Write([]byte("album-access-cookie\x00"))
	mac.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

// readAlbumGrants returns the unexpired grants in the visitor's cookie. A
// cookie that does not verify, or that holds anything unexpected, is treated as
// holding nothing so a forged one buys no access even if it is not rejected.
func (a *App) readAlbumGrants(r *http.Request) []albumGrant {
	cookie, err := r.Cookie(albumAccessCookie)
	if err != nil || cookie.Value == "" || len(cookie.Value) > albumGrantTokenMax {
		return nil
	}
	payload, signature, found := strings.Cut(cookie.Value, ".")
	if !found || payload == "" || len(signature) != 43 {
		return nil
	}
	if subtle.ConstantTimeCompare([]byte(signature), []byte(a.albumGrantSignature(payload))) != 1 {
		return nil
	}
	decoded, err := base64.RawURLEncoding.DecodeString(payload)
	if err != nil {
		return nil
	}
	var grants []albumGrant
	if err := json.Unmarshal(decoded, &grants); err != nil || len(grants) > albumGrantMax {
		return nil
	}
	now := time.Now()
	live := []albumGrant{}
	for _, grant := range grants {
		if grant.ID <= 0 || !albumGrantHashPattern.MatchString(grant.Hash) {
			return nil
		}
		if time.UnixMilli(grant.ExpiresAt).After(now) {
			live = append(live, grant)
		}
	}
	return live
}

// grantAlbumAccess adds an album to the visitor's cookie. The cookie is
// deliberately a session cookie: the expiry that matters is the one inside the
// payload, and leaving it to the browser to also expire would let a stale
// cookie outlive a password change.
func (a *App) grantAlbumAccess(w http.ResponseWriter, r *http.Request, id int64, passwordHash string) {
	grants := []albumGrant{}
	for _, grant := range a.readAlbumGrants(r) {
		if grant.ID != id {
			grants = append(grants, grant)
		}
	}
	grants = append(grants, albumGrant{
		ID:        id,
		Hash:      a.albumPasswordVersion(passwordHash),
		ExpiresAt: time.Now().Add(albumGrantLifetime).UnixMilli(),
	})
	if len(grants) > albumGrantMax {
		grants = grants[len(grants)-albumGrantMax:]
	}
	payload, err := json.Marshal(grants)
	if err != nil {
		return
	}
	encoded := base64.RawURLEncoding.EncodeToString(payload)
	http.SetCookie(w, &http.Cookie{
		Name:     albumAccessCookie,
		Value:    encoded + "." + a.albumGrantSignature(encoded),
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   requestIsHTTPS(r),
	})
}

func requestIsHTTPS(r *http.Request) bool {
	return r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
}

// albumAccessTarget is the part of an album that decides who may see it.
type albumAccessTarget struct {
	id       int64
	hidden   bool
	password sql.NullString
}

func (album albumAccessTarget) locked() bool {
	return album.password.Valid && album.password.String != ""
}

// albumAccess is what one request is allowed to see, resolved once so that a
// list of albums does not verify the session or the cookie once per album.
type albumAccess struct {
	admin  bool
	grants []albumGrant
}

func (a *App) albumAccessFor(r *http.Request) albumAccess {
	_, admin := a.require(r)
	return albumAccess{admin: admin, grants: a.readAlbumGrants(r)}
}

func (a *App) canViewAlbum(access albumAccess, album albumAccessTarget) bool {
	if access.admin {
		return true
	}
	if album.hidden {
		return false
	}
	if !album.locked() {
		return true
	}
	version := a.albumPasswordVersion(album.password.String)
	for _, grant := range access.grants {
		if grant.ID == album.id && grant.Hash == version {
			return true
		}
	}
	return false
}

// photoAccessDenied decides the fate of a photo from the albums it belongs to.
// A photo kept out of sight is out of reach; otherwise it is reachable as soon
// as one of its albums is, so a photo shared between a public album and a
// protected one stays visible through the public album. A photo in no album at
// all is not album business and is left alone.
func (a *App) photoAccessDenied(access albumAccess, memberships []albumAccessTarget) bool {
	if access.admin || len(memberships) == 0 {
		return false
	}
	// A hidden album takes the photo out of reach whatever else holds it, so
	// the whole list is searched for one before any album is allowed to decide.
	for _, album := range memberships {
		if album.hidden {
			return true
		}
	}
	for _, album := range memberships {
		if a.canViewAlbum(access, album) {
			return false
		}
	}
	return true
}

// albumMembershipsForPhotos groups the albums that hold each photo.
func (a *App) albumMembershipsForPhotos(ctx context.Context, photoIDs []string) (map[string][]albumAccessTarget, error) {
	memberships := map[string][]albumAccessTarget{}
	if len(photoIDs) == 0 {
		return memberships, nil
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(photoIDs)), ",")
	args := make([]any, len(photoIDs))
	for i, id := range photoIDs {
		args[i] = id
	}
	rows, err := a.db.QueryContext(ctx, `
		SELECT ap.photo_id,al.id,al.is_hidden,al.password_hash
		FROM album_photos ap
		INNER JOIN albums al ON al.id=ap.album_id
		WHERE ap.photo_id IN (`+placeholders+`)`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var photoID string
		var album albumAccessTarget
		var hidden int
		if err := rows.Scan(&photoID, &album.id, &hidden, &album.password); err != nil {
			return nil, err
		}
		album.hidden = hidden == 1
		memberships[photoID] = append(memberships[photoID], album)
	}
	return memberships, rows.Err()
}

// restrictedPhotoIDs lists every photo that a hidden album holds. The public
// album list uses it to leave those photos out of the albums that do show, so a
// photo kept out of the gallery cannot be brought back in by adding it to a
// public album.
//
// A locked album restricts nothing on its own. A photo is reachable exactly
// when some album the visitor may open holds it, so an album that is on show
// shows everything it holds, whichever other albums also hold it — which is the
// same answer the media routes give.
func (a *App) restrictedPhotoIDs(ctx context.Context) (map[string]bool, error) {
	rows, err := a.db.QueryContext(ctx, `
		SELECT DISTINCT ap.photo_id
		FROM album_photos ap
		INNER JOIN albums al ON al.id=ap.album_id
		WHERE al.is_hidden=1`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := map[string]bool{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids[id] = true
	}
	return ids, rows.Err()
}

// mediaAccess answers whether a media request may be served, and whether the
// answer depended on who is asking.
//
// The second value is what decides the cache headers. A photo that anybody may
// see is immutable and safe for a shared cache to keep; a photo that is held
// back for a password must not be, because the next visitor behind the same
// cache would otherwise be handed it without ever proving anything.
func (a *App) mediaAccess(r *http.Request, key string) (allowed, restricted bool) {
	access := a.albumAccessFor(r)
	if access.admin {
		return true, false
	}
	ids, err := a.photoIDsForAccessKey(r.Context(), normalizeAccessKey(key))
	if err != nil || len(ids) == 0 {
		// No indexed photo answers to this key, so no album rule applies to it.
		return true, false
	}
	memberships, err := a.albumMembershipsForPhotos(r.Context(), ids)
	if err != nil {
		return false, false
	}
	for _, id := range ids {
		albums := memberships[id]
		for _, album := range albums {
			if album.hidden || album.locked() {
				restricted = true
			}
		}
		if a.photoAccessDenied(access, albums) {
			return false, restricted
		}
	}
	return true, restricted
}

// mediaCacheControl is applied to every media response. See mediaAccess for why
// a restricted answer is not cacheable.
func mediaCacheControl(restricted bool) string {
	if restricted {
		return "private, no-store"
	}
	return "public, max-age=31536000, immutable"
}

// unlockAlbum is the only way a visitor without an account can gain access to a
// protected album. Everything that would say whether an album exists happens
// before the password is checked, so a wrong album id, a hidden album and one
// with no password all look the same from the outside.
func (a *App) unlockAlbum(w http.ResponseWriter, r *http.Request, id int64) {
	var body struct {
		Password string `json:"password"`
	}
	if decodeJSON(r, &body) != nil {
		errorJSON(w, http.StatusBadRequest, "Invalid request")
		return
	}
	if body.Password == "" || len(body.Password) > albumPasswordMaxLength {
		errorJSON(w, http.StatusBadRequest, "Invalid password")
		return
	}
	var album albumAccessTarget
	var hidden int
	if err := a.db.QueryRow(`SELECT id,is_hidden,password_hash FROM albums WHERE id=?`, id).Scan(&album.id, &hidden, &album.password); err != nil {
		errorJSON(w, http.StatusNotFound, "Album not found")
		return
	}
	album.hidden = hidden == 1
	if album.hidden || !album.locked() {
		errorJSON(w, http.StatusNotFound, "Album not found")
		return
	}
	client := remoteIP(r)
	if !albumUnlockAllowed(id, client) {
		errorJSON(w, http.StatusTooManyRequests, "Too many unlock attempts")
		return
	}
	if !verifyAlbumPassword(body.Password, album.password.String) {
		errorJSON(w, http.StatusUnauthorized, "Incorrect album password")
		return
	}
	albumUnlockSucceeded(id, client)
	a.grantAlbumAccess(w, r, album.id, album.password.String)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
