package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

type Photo struct {
	ID                string `json:"id"`
	Title             any    `json:"title"`
	Description       any    `json:"description"`
	Width             any    `json:"width"`
	Height            any    `json:"height"`
	AspectRatio       any    `json:"aspectRatio"`
	DateTaken         any    `json:"dateTaken"`
	StorageKey        any    `json:"storageKey"`
	ThumbnailKey      any    `json:"thumbnailKey"`
	FileSize          any    `json:"fileSize"`
	LastModified      any    `json:"lastModified"`
	OriginalURL       any    `json:"originalUrl"`
	ThumbnailURL      any    `json:"thumbnailUrl"`
	ThumbnailHash     any    `json:"thumbnailHash"`
	Tags              any    `json:"tags"`
	Exif              any    `json:"exif"`
	Latitude          any    `json:"latitude"`
	Longitude         any    `json:"longitude"`
	Country           any    `json:"country"`
	City              any    `json:"city"`
	LocationName      any    `json:"locationName"`
	IsLivePhoto       any    `json:"isLivePhoto"`
	LivePhotoVideoURL any    `json:"livePhotoVideoUrl"`
	LivePhotoVideoKey any    `json:"livePhotoVideoKey"`
}

func scanPhoto(rows *sql.Rows) (Photo, error) {
	var p Photo
	var tags, exif sql.NullString
	err := rows.Scan(&p.ID, &p.Title, &p.Description, &p.Width, &p.Height, &p.AspectRatio, &p.DateTaken, &p.StorageKey, &p.ThumbnailKey, &p.FileSize, &p.LastModified, &p.OriginalURL, &p.ThumbnailURL, &p.ThumbnailHash, &tags, &exif, &p.Latitude, &p.Longitude, &p.Country, &p.City, &p.LocationName, &p.IsLivePhoto, &p.LivePhotoVideoURL, &p.LivePhotoVideoKey)
	if tags.Valid {
		var v any
		if json.Unmarshal([]byte(tags.String), &v) == nil {
			p.Tags = v
		}
	}
	if exif.Valid {
		var v any
		if json.Unmarshal([]byte(exif.String), &v) == nil {
			p.Exif = v
		}
	}
	return p, err
}

const photoSelect = `SELECT id,title,description,width,height,aspect_ratio,date_taken,storage_key,thumbnail_key,file_size,last_modified,original_url,thumbnail_url,thumbnail_hash,tags,exif,latitude,longitude,country,city,location_name,is_live_photo,live_photo_video_url,live_photo_video_key FROM photos`

func (a *App) photos(w http.ResponseWriter, r *http.Request, visible bool) {
	if !visible {
		if u, ok := a.user(r); !ok || u == nil {
			errorJSON(w, 401, "Unauthorized")
			return
		}
	}
	query := photoSelect
	if visible {
		query += ` WHERE NOT EXISTS (SELECT 1 FROM album_photos ap JOIN albums al ON al.id=ap.album_id WHERE ap.photo_id=photos.id AND al.is_hidden=1)`
	}
	query += ` ORDER BY date_taken DESC`
	rows, err := a.db.Query(query)
	if err != nil {
		errorJSON(w, 500, err.Error())
		return
	}
	defer rows.Close()
	out := []Photo{}
	for rows.Next() {
		p, err := scanPhoto(rows)
		if err == nil {
			out = append(out, p)
		}
	}
	writeJSON(w, 200, out)
}

func (a *App) prepareUpload(w http.ResponseWriter, r *http.Request) {
	if !a.requireAdmin(w, r) {
		return
	}
	var b struct {
		FileName, ContentType string
		SkipDuplicateCheck    bool
	}
	if decodeJSON(r, &b) != nil || b.FileName == "" {
		errorJSON(w, 400, "fileName is required")
		return
	}
	key := storageKey(a.storage.Prefix(), b.FileName)
	id := safePhotoID(key)
	var existing Photo
	var found bool
	row := a.db.QueryRow(photoSelect+` WHERE id=?`, id)
	var rows *sql.Rows
	_ = rows
	var rawTags, rawExif sql.NullString
	err := row.Scan(&existing.ID, &existing.Title, &existing.Description, &existing.Width, &existing.Height, &existing.AspectRatio, &existing.DateTaken, &existing.StorageKey, &existing.ThumbnailKey, &existing.FileSize, &existing.LastModified, &existing.OriginalURL, &existing.ThumbnailURL, &existing.ThumbnailHash, &rawTags, &rawExif, &existing.Latitude, &existing.Longitude, &existing.Country, &existing.City, &existing.LocationName, &existing.IsLivePhoto, &existing.LivePhotoVideoURL, &existing.LivePhotoVideoKey)
	if err == nil {
		found = true
	}
	if found && !b.SkipDuplicateCheck {
		writeJSON(w, 200, map[string]any{"skipped": true, "duplicate": true, "existingPhoto": existing, "fileKey": key, "title": "Duplicate file", "message": "File already exists"})
		return
	}
	signed, err := a.storage.SignedURL(r.Context(), key, b.ContentType)
	if err != nil {
		signed = "/api/photos/upload?key=" + urlQueryEscape(key)
	}
	writeJSON(w, 200, map[string]any{"signedUrl": signed, "fileKey": key, "expiresIn": 3600, "duplicate": found, "existingPhoto": func() any {
		if found {
			return existing
		}
		return nil
	}()})
}
func (a *App) upload(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.user(r); !ok {
		errorJSON(w, 401, "Unauthorized")
		return
	}
	key := r.URL.Query().Get("key")
	if key == "" {
		errorJSON(w, 400, "key is required")
		return
	}
	max := int64(envInt("CFRAME_MAX_UPLOAD_MB", 256)) * 1024 * 1024
	r.Body = http.MaxBytesReader(w, r.Body, max)
	data, err := io.ReadAll(r.Body)
	if err != nil {
		errorJSON(w, 413, "Upload too large")
		return
	}
	if _, err = a.storage.Create(r.Context(), key, data, r.Header.Get("Content-Type")); err != nil {
		errorJSON(w, 500, "Upload failed")
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true, "key": key})
}
func (a *App) checkDuplicate(w http.ResponseWriter, r *http.Request) {
	if !a.requireAdmin(w, r) {
		return
	}
	var b struct{ FileName string }
	if decodeJSON(r, &b) != nil {
		errorJSON(w, 400, "Invalid request")
		return
	}
	id := safePhotoID(storageKey(a.storage.Prefix(), b.FileName))
	var count int
	a.db.QueryRow(`SELECT count(*) FROM photos WHERE id=?`, id).Scan(&count)
	writeJSON(w, 200, map[string]any{"duplicate": count > 0})
}
func (a *App) photoStatus(w http.ResponseWriter, r *http.Request) {
	if !a.requireAdmin(w, r) {
		return
	}
	var total, processed int
	a.db.QueryRow(`SELECT count(*) FROM photos`).Scan(&total)
	a.db.QueryRow(`SELECT count(*) FROM pipeline_queue WHERE status='completed'`).Scan(&processed)
	writeJSON(w, 200, map[string]any{"total": total, "processed": processed})
}
func (a *App) reactions(w http.ResponseWriter, r *http.Request) {
	ids := r.URL.Query()["ids"]
	if len(ids) == 1 {
		ids = strings.Split(ids[0], ",")
	}
	out := map[string]map[string]int{}
	for _, id := range ids {
		rows, _ := a.db.Query(`SELECT reaction_type,count(*) FROM photo_reactions WHERE photo_id=? GROUP BY reaction_type`, id)
		m := newReactionCounts()
		for rows != nil && rows.Next() {
			var typ string
			var n int
			_ = rows.Scan(&typ, &n)
			m[typ] = n
		}
		if rows != nil {
			rows.Close()
		}
		out[id] = m
	}
	writeJSON(w, 200, out)
}

func newReactionCounts() map[string]int {
	return map[string]int{"like": 0, "love": 0, "amazing": 0, "funny": 0, "wow": 0, "sad": 0, "fire": 0, "sparkle": 0}
}

func (a *App) photoRoute(w http.ResponseWriter, r *http.Request, rest string) {
	parts := strings.Split(strings.Trim(rest, "/"), "/")
	if len(parts) == 0 {
		return
	}
	id := parts[0]
	if len(parts) == 1 && r.Method == "PUT" {
		a.updatePhoto(w, r, id)
		return
	}
	if len(parts) == 1 && r.Method == "DELETE" {
		a.deletePhoto(w, r, id)
		return
	}
	if len(parts) >= 2 && parts[1] == "albums" && r.Method == "GET" {
		a.photoAlbums(w, id)
		return
	}
	if len(parts) >= 2 && parts[1] == "livephoto" {
		a.photoLive(w, r, id)
		return
	}
	if len(parts) >= 2 && parts[1] == "reactions" {
		a.photoReaction(w, r, id)
		return
	}
	if len(parts) >= 2 && parts[1] == "exif" {
		writeJSON(w, 200, map[string]any{"success": true})
		return
	}
	errorJSON(w, 404, "Not Found")
}
func (a *App) updatePhoto(w http.ResponseWriter, r *http.Request, id string) {
	if !a.requireAdmin(w, r) {
		return
	}
	var b map[string]any
	if decodeJSON(r, &b) != nil {
		errorJSON(w, 400, "Invalid request")
		return
	}
	allowed := map[string]string{"title": "title", "description": "description", "tags": "tags", "latitude": "latitude", "longitude": "longitude"}
	sets := []string{}
	args := []any{}
	for key, col := range allowed {
		if v, ok := b[key]; ok {
			sets = append(sets, col+"=?")
			if key == "tags" {
				args = append(args, jsonValue(v))
			} else {
				args = append(args, v)
			}
		}
	}
	if len(sets) > 0 {
		args = append(args, id)
		_, _ = a.db.Exec(`UPDATE photos SET `+strings.Join(sets, ",")+` WHERE id=?`, args...)
	}
	writeJSON(w, 200, map[string]any{"success": true})
}
func (a *App) deletePhoto(w http.ResponseWriter, r *http.Request, id string) {
	if !a.requireAdmin(w, r) {
		return
	}
	var key, thumb string
	if a.db.QueryRow(`SELECT storage_key,thumbnail_key FROM photos WHERE id=?`, id).Scan(&key, &thumb) != nil {
		errorJSON(w, 404, "Photo not found")
		return
	}
	_ = a.storage.Delete(r.Context(), key)
	if thumb != "" {
		_ = a.storage.Delete(r.Context(), thumb)
	}
	_, _ = a.db.Exec(`DELETE FROM photos WHERE id=?`, id)
	w.WriteHeader(http.StatusNoContent)
}
func (a *App) photoReaction(w http.ResponseWriter, r *http.Request, id string) {
	u, _ := a.user(r)
	fingerprint := r.RemoteAddr
	if u != nil {
		fingerprint = fmt.Sprint(u["id"])
	}
	if r.Method == "GET" {
		rows, _ := a.db.Query(`SELECT reaction_type,count(*) FROM photo_reactions WHERE photo_id=? GROUP BY reaction_type`, id)
		out := newReactionCounts()
		for rows != nil && rows.Next() {
			var typ string
			var count int
			_ = rows.Scan(&typ, &count)
			out[typ] = count
		}
		if rows != nil {
			rows.Close()
		}
		writeJSON(w, 200, out)
		return
	}
	if r.Method == "DELETE" {
		_, _ = a.db.Exec(`DELETE FROM photo_reactions WHERE photo_id=? AND fingerprint=?`, id, fingerprint)
		w.WriteHeader(http.StatusNoContent)
		return
	}
	var body struct {
		ReactionType string `json:"reactionType"`
	}
	if decodeJSON(r, &body) != nil || body.ReactionType == "" {
		errorJSON(w, 400, "reactionType is required")
		return
	}
	_, err := a.db.Exec(`INSERT INTO photo_reactions(photo_id,reaction_type,fingerprint,ip_address,user_agent) VALUES(?,?,?,?,?)`, id, body.ReactionType, fingerprint, r.RemoteAddr, r.UserAgent())
	if err != nil {
		errorJSON(w, 500, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"success": true})
}
