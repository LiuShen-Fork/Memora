package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"golang.org/x/crypto/bcrypt"
	"golang.org/x/crypto/scrypt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// HTTP and JSON
func (a *App) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "same-origin")
	if r.Method == "OPTIONS" {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if strings.HasPrefix(r.URL.Path, "/api/") {
		a.api(w, r)
		return
	}
	if strings.HasPrefix(r.URL.Path, "/storage/") {
		a.serveStorage(w, r)
		return
	}
	if strings.HasPrefix(r.URL.Path, "/image/") {
		a.serveImage(w, r)
		return
	}
	if strings.HasPrefix(r.URL.Path, "/thumb/") {
		a.serveThumb(w, r)
		return
	}
	a.serveWeb(w, r)
}
func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
func errorJSON(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]any{"statusCode": status, "statusMessage": message, "message": message})
}
func decodeJSON(r *http.Request, v any) error {
	return json.NewDecoder(io.LimitReader(r.Body, 16<<20)).Decode(v)
}
func (a *App) api(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimSuffix(r.URL.Path, "/")
	switch {
	case path == "/api/health" && r.Method == "GET":
		a.health(w, r)
	case path == "/api/login" && r.Method == "POST":
		a.login(w, r)
	case path == "/api/logout":
		a.logout(w, r)
	case path == "/api/profile":
		a.profile(w, r)
	case path == "/api/photos" && r.Method == "GET":
		a.photos(w, r, false)
	case path == "/api/photos/visible" && r.Method == "GET":
		a.photos(w, r, true)
	case path == "/api/photos" && r.Method == "POST":
		a.prepareUpload(w, r)
	case path == "/api/photos/upload" && r.Method == "PUT":
		a.upload(w, r)
	case path == "/api/photos/exif/reindex" && r.Method == "POST":
		a.reindexExif(w, r)
	case path == "/api/photos/livephoto/manage" && r.Method == "POST":
		a.manageLivePhoto(w, r)
	case path == "/api/photos/check-duplicate" && r.Method == "POST":
		a.checkDuplicate(w, r)
	case path == "/api/photos/reactions" && r.Method == "GET":
		a.reactions(w, r)
	case path == "/api/photos/status" && r.Method == "GET":
		a.photoStatus(w, r)
	case strings.HasPrefix(path, "/api/photos/"):
		a.photoRoute(w, r, strings.TrimPrefix(path, "/api/photos/"))
	case path == "/api/albums" && r.Method == "GET":
		a.albums(w, r)
	case path == "/api/albums" && r.Method == "POST":
		a.createAlbum(w, r)
	case strings.HasPrefix(path, "/api/albums/"):
		a.albumRoute(w, r, strings.TrimPrefix(path, "/api/albums/"))
	case strings.HasPrefix(path, "/api/queue/"):
		a.queueRoute(w, r, strings.TrimPrefix(path, "/api/queue/"))
	case strings.HasPrefix(path, "/api/system/settings"):
		a.settingsRoute(w, r, strings.TrimPrefix(path, "/api/system/settings"))
	case path == "/api/system/stats":
		a.systemStats(w, r)
	case path == "/api/system/logs":
		a.systemLogs(w, r)
	case strings.HasPrefix(path, "/api/wizard/"):
		a.wizardRoute(w, r, strings.TrimPrefix(path, "/api/wizard/"))
	case path == "/api/auth/github":
		a.githubAuth(w, r)
	default:
		errorJSON(w, http.StatusNotFound, "Not Found")
	}
}

func (a *App) health(w http.ResponseWriter, r *http.Request) {
	if err := a.db.PingContext(r.Context()); err != nil {
		errorJSON(w, http.StatusServiceUnavailable, "Database unavailable")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (a *App) user(r *http.Request) (map[string]any, bool) {
	c, err := r.Cookie(cookieName)
	if err != nil {
		return nil, false
	}
	parts := strings.Split(c.Value, ".")
	if len(parts) != 2 {
		return nil, false
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return nil, false
	}
	mac := hmac.New(sha256.New, a.cfg.SessionKey)
	mac.Write([]byte(parts[0]))
	if subtle.ConstantTimeCompare(mac.Sum(nil), mustDecode(parts[1])) != 1 {
		return nil, false
	}
	var id int64
	var exp int64
	fmt.Sscanf(string(payload), "%d:%d", &id, &exp)
	if id == 0 || exp < time.Now().Unix() {
		return nil, false
	}
	var name, email, avatar string
	var admin int
	if a.db.QueryRow(`SELECT name,email,COALESCE(avatar,''),is_admin FROM users WHERE id=?`, id).Scan(&name, &email, &avatar, &admin) != nil {
		return nil, false
	}
	return map[string]any{"id": id, "username": name, "email": email, "avatar": avatar, "isAdmin": admin}, true
}
func mustDecode(s string) []byte                              { b, _ := base64.RawURLEncoding.DecodeString(s); return b }
func (a *App) require(r *http.Request) (map[string]any, bool) { return a.user(r) }
func (a *App) requireAdmin(w http.ResponseWriter, r *http.Request) bool {
	u, ok := a.require(r)
	if !ok {
		errorJSON(w, 401, "Unauthorized")
		return false
	}
	v, _ := u["isAdmin"].(int)
	if v != 1 {
		errorJSON(w, 403, "Forbidden")
		return false
	}
	return true
}
func (a *App) setSession(w http.ResponseWriter, id int64) {
	payload := fmt.Sprintf("%d:%d", id, time.Now().Add(30*24*time.Hour).Unix())
	p := base64.RawURLEncoding.EncodeToString([]byte(payload))
	mac := hmac.New(sha256.New, a.cfg.SessionKey)
	mac.Write([]byte(p))
	sig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	http.SetCookie(w, &http.Cookie{Name: cookieName, Value: p + "." + sig, Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode, MaxAge: 30 * 24 * 3600})
}
func (a *App) login(w http.ResponseWriter, r *http.Request) {
	var body struct{ Email, Password string }
	if decodeJSON(r, &body) != nil {
		errorJSON(w, 400, "Invalid request")
		return
	}
	var id int64
	var hash string
	if a.db.QueryRow(`SELECT id,password FROM users WHERE email=?`, body.Email).Scan(&id, &hash) != nil || !verifyPassword(hash, body.Password) {
		errorJSON(w, 401, "Invalid credentials")
		return
	}
	a.setSession(w, id)
	w.WriteHeader(http.StatusCreated)
}
func verifyPassword(hash, password string) bool {
	if strings.HasPrefix(hash, "$2") {
		return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
	}
	if strings.HasPrefix(hash, "$scrypt$") {
		parts := strings.Split(hash, "$")
		if len(parts) != 5 {
			return false
		}
		params := map[string]int{}
		for _, pair := range strings.Split(parts[2], ",") {
			kv := strings.SplitN(pair, "=", 2)
			if len(kv) == 2 {
				params[kv[0]], _ = strconv.Atoi(kv[1])
			}
		}
		salt, err := base64.RawStdEncoding.DecodeString(parts[3])
		if err != nil {
			salt, err = base64.StdEncoding.DecodeString(parts[3])
		}
		expected, hashErr := base64.RawStdEncoding.DecodeString(parts[4])
		if hashErr != nil {
			expected, hashErr = base64.StdEncoding.DecodeString(parts[4])
		}
		if err != nil || hashErr != nil || params["n"] == 0 || params["r"] == 0 || params["p"] == 0 {
			return false
		}
		derived, deriveErr := scrypt.Key([]byte(password), salt, params["n"], params["r"], params["p"], len(expected))
		return deriveErr == nil && subtle.ConstantTimeCompare(derived, expected) == 1
	}
	return subtle.ConstantTimeCompare([]byte(hash), []byte(password)) == 1
}
func (a *App) logout(w http.ResponseWriter, _ *http.Request) {
	http.SetCookie(w, &http.Cookie{Name: cookieName, Value: "", Path: "/", MaxAge: -1, HttpOnly: true})
	w.WriteHeader(http.StatusNoContent)
}
func (a *App) profile(w http.ResponseWriter, r *http.Request) {
	u, ok := a.user(r)
	if !ok {
		writeJSON(w, 200, nil)
		return
	}
	writeJSON(w, 200, u)
}
