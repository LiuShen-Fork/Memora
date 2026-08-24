package main

import (
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"
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

func (a *App) photoByID(id string) (Photo, error) {
	rows, err := a.db.Query(photoSelect+` WHERE id=?`, id)
	if err != nil {
		return Photo{}, err
	}
	defer rows.Close()
	if !rows.Next() {
		return Photo{}, sql.ErrNoRows
	}
	return scanPhoto(rows)
}

func (a *App) photos(w http.ResponseWriter, r *http.Request, visible bool) {
	// The Nuxt gallery exposes the complete collection to both anonymous and
	// authenticated readers. The visible endpoint only adds hidden-album
	// filtering for anonymous browsing.
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
	if _, ok := a.require(r); !ok {
		errorJSON(w, http.StatusUnauthorized, "Unauthorized")
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
	if found && isVideoUpload(b.FileName, b.ContentType) {
		if storageKeyText, ok := existing.StorageKey.(string); ok && isImageStorageKey(storageKeyText) {
			// A Live Photo video may share the sanitized ID of its image pair.
			// Nuxt only treats an existing video as a duplicate in this case.
			found = false
		}
	}
	if found && !b.SkipDuplicateCheck && duplicateCheckEnabled(a) {
		mode := duplicateMode(a)
		response := map[string]any{"duplicate": true, "existingPhoto": existing, "fileKey": key, "title": "Duplicate file", "message": "File already exists"}
		if mode == "block" {
			writeJSON(w, http.StatusConflict, response)
			return
		}
		if mode == "skip" {
			response["skipped"] = true
			writeJSON(w, http.StatusOK, response)
			return
		}
		response["warningInfo"] = map[string]any{"title": "Duplicate file", "message": "File already exists"}
		signed, err := a.storage.SignedURL(r.Context(), key, b.ContentType)
		if err != nil {
			signed = "/api/photos/upload?key=" + urlQueryEscape(key)
		}
		response["signedUrl"] = signed
		response["expiresIn"] = 3600
		writeJSON(w, http.StatusOK, response)
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

func isVideoUpload(fileName, contentType string) bool {
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(contentType)), "video/") {
		return true
	}
	switch strings.ToLower(filepath.Ext(fileName)) {
	case ".mov", ".mp4":
		return true
	default:
		return false
	}
}

func isImageStorageKey(key string) bool {
	switch strings.ToLower(filepath.Ext(key)) {
	case ".avif", ".bmp", ".gif", ".heic", ".heif", ".jpeg", ".jpg", ".png", ".tif", ".tiff", ".webp":
		return true
	default:
		return false
	}
}
func (a *App) upload(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.user(r); !ok {
		errorJSON(w, 401, "Unauthorized")
		return
	}
	contentType := r.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	if !uploadMIMEAllowed(contentType) {
		errorJSON(w, http.StatusUnsupportedMediaType, "Unsupported media type")
		return
	}
	key := r.URL.Query().Get("key")
	if key == "" {
		errorJSON(w, 400, "key is required")
		return
	}
	max := a.maxUploadBytes()
	r.Body = http.MaxBytesReader(w, r.Body, max)
	data, err := io.ReadAll(r.Body)
	if err != nil {
		errorJSON(w, 413, "Upload too large")
		return
	}
	if _, err = a.storage.Create(r.Context(), key, data, contentType); err != nil {
		errorJSON(w, 500, "Upload failed")
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true, "key": key})
}
func (a *App) checkDuplicate(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.require(r); !ok {
		errorJSON(w, http.StatusUnauthorized, "Unauthorized")
		return
	}
	var body struct {
		FileNames   []string `json:"fileNames"`
		StorageKeys []string `json:"storageKeys"`
	}
	if decodeJSON(r, &body) != nil || (body.FileNames == nil && body.StorageKeys == nil) {
		errorJSON(w, http.StatusBadRequest, "fileNames or storageKeys is required")
		return
	}
	results := []map[string]any{}
	check := func(key, fileName string) {
		id := safePhotoID(key)
		var photo Photo
		var tags, exif sql.NullString
		err := a.db.QueryRow(photoSelect+` WHERE id=?`, id).Scan(&photo.ID, &photo.Title, &photo.Description, &photo.Width, &photo.Height, &photo.AspectRatio, &photo.DateTaken, &photo.StorageKey, &photo.ThumbnailKey, &photo.FileSize, &photo.LastModified, &photo.OriginalURL, &photo.ThumbnailURL, &photo.ThumbnailHash, &tags, &exif, &photo.Latitude, &photo.Longitude, &photo.Country, &photo.City, &photo.LocationName, &photo.IsLivePhoto, &photo.LivePhotoVideoURL, &photo.LivePhotoVideoKey)
		result := map[string]any{"storageKey": key, "photoId": id, "exists": err == nil, "photo": nil}
		if fileName != "" {
			result["fileName"] = fileName
		}
		if err == nil {
			result["photo"] = photo
		}
		results = append(results, result)
	}
	for _, fileName := range body.FileNames {
		check(storageKey(a.storage.Prefix(), fileName), fileName)
	}
	for _, key := range body.StorageKeys {
		check(key, "")
	}
	duplicates := 0
	for _, result := range results {
		if result["exists"] == true {
			duplicates++
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "results": results, "duplicatesFound": duplicates, "summary": map[string]any{"title": "Duplicate check completed", "message": "Duplicate check completed"}})
}
func (a *App) photoStatus(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.require(r); !ok {
		errorJSON(w, http.StatusUnauthorized, "Unauthorized")
		return
	}
	rows, err := a.db.Query(photoSelect + ` ORDER BY last_modified LIMIT 10`)
	if err != nil {
		errorJSON(w, http.StatusInternalServerError, "Unable to read photo status")
		return
	}
	defer rows.Close()
	recentPhotos := []Photo{}
	for rows.Next() {
		photo, scanErr := scanPhoto(rows)
		if scanErr != nil {
			errorJSON(w, http.StatusInternalServerError, "Unable to read photo status")
			return
		}
		recentPhotos = append(recentPhotos, photo)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"recentPhotos": recentPhotos,
		"timestamp":    time.Now().UTC().Format(time.RFC3339Nano),
	})
}
func (a *App) reactions(w http.ResponseWriter, r *http.Request) {
	ids := r.URL.Query()["ids"]
	if len(ids) == 0 || (len(ids) == 1 && strings.TrimSpace(ids[0]) == "") {
		errorJSON(w, http.StatusBadRequest, "ids is required")
		return
	}
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
	if len(parts) >= 2 && parts[1] == "livephoto" && r.Method == http.MethodGet {
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
	if _, ok := a.require(r); !ok {
		errorJSON(w, http.StatusUnauthorized, "Unauthorized")
		return
	}
	var body struct {
		Title       *string          `json:"title"`
		Description *string          `json:"description"`
		Tags        *[]string        `json:"tags"`
		Location    json.RawMessage  `json:"location"`
		Rating      *json.RawMessage `json:"rating"`
	}
	if decodeJSON(r, &body) != nil {
		errorJSON(w, http.StatusBadRequest, "Invalid request")
		return
	}
	if body.Title != nil && utf8.RuneCountInString(*body.Title) > 512 {
		errorJSON(w, http.StatusBadRequest, "title is too long")
		return
	}
	if body.Description != nil && utf8.RuneCountInString(*body.Description) > 2000 {
		errorJSON(w, http.StatusBadRequest, "description is too long")
		return
	}
	if body.Tags != nil {
		if len(*body.Tags) > 64 {
			errorJSON(w, http.StatusBadRequest, "tags must contain at most 64 items")
			return
		}
		for _, tag := range *body.Tags {
			if utf8.RuneCountInString(tag) > 128 {
				errorJSON(w, http.StatusBadRequest, "tag is too long")
				return
			}
		}
	}
	if body.Title == nil && body.Description == nil && body.Tags == nil && body.Location == nil && body.Rating == nil {
		errorJSON(w, http.StatusBadRequest, "No changes provided")
		return
	}
	var key string
	if err := a.db.QueryRow(`SELECT storage_key FROM photos WHERE id=?`, id).Scan(&key); err != nil {
		errorJSON(w, http.StatusNotFound, "Photo not found")
		return
	}
	data, err := a.storage.Get(r.Context(), key)
	if err != nil {
		errorJSON(w, http.StatusNotFound, "Photo file missing")
		return
	}
	updates := map[string]any{}
	sets := []string{}
	args := []any{}
	var location map[string]any
	if body.Title != nil {
		title := strings.TrimSpace(*body.Title)
		updates["Title"] = title
		updates["XPTitle"] = title
		sets = append(sets, "title=?")
		args = append(args, nullIfEmpty(title))
	}
	if body.Description != nil {
		description := strings.TrimSpace(*body.Description)
		updates["Description"] = description
		updates["ImageDescription"] = description
		updates["CaptionAbstract"] = description
		updates["XPComment"] = description
		updates["UserComment"] = description
		sets = append(sets, "description=?")
		args = append(args, nullIfEmpty(description))
	}
	if body.Tags != nil {
		tags := normalizeTags(*body.Tags)
		updates["Subject"] = tags
		updates["Keywords"] = tags
		updates["XPKeywords"] = strings.Join(tags, "; ")
		sets = append(sets, "tags=?")
		args = append(args, jsonValue(tags))
	}
	if body.Location != nil {
		if string(body.Location) != "null" && json.Unmarshal(body.Location, &location) != nil {
			errorJSON(w, http.StatusBadRequest, "Invalid location")
			return
		}
		if location != nil {
			latitude, lok := location["latitude"].(float64)
			longitude, ook := location["longitude"].(float64)
			if !lok || !ook || latitude < -90 || latitude > 90 || longitude < -180 || longitude > 180 {
				errorJSON(w, http.StatusBadRequest, "Invalid location")
				return
			}
			sets = append(sets, "latitude=?", "longitude=?", "country=NULL", "city=NULL", "location_name=NULL")
			args = append(args, latitude, longitude)
		} else {
			sets = append(sets, "latitude=NULL", "longitude=NULL", "country=NULL", "city=NULL", "location_name=NULL")
		}
		updatesMap := locationExifUpdates(location)
		for field, value := range updatesMap {
			updates[field] = value
		}
	}
	if body.Rating != nil {
		var rating any
		if json.Unmarshal(*body.Rating, &rating) != nil {
			errorJSON(w, http.StatusBadRequest, "Invalid rating")
			return
		}
		if rating != nil {
			value, ok := rating.(float64)
			if !ok || value < 0 || value > 5 || value != float64(int(value)) {
				errorJSON(w, http.StatusBadRequest, "Invalid rating")
				return
			}
		}
		updates["Rating"] = rating
	}
	updatedData, err := a.rewritePhotoMetadata(r.Context(), key, data, updates)
	if err != nil {
		errorJSON(w, http.StatusInternalServerError, "Failed to write photo metadata")
		return
	}
	if _, err := a.storage.Create(r.Context(), key, updatedData, "application/octet-stream"); err != nil {
		errorJSON(w, http.StatusInternalServerError, "Failed to save photo")
		return
	}
	exif, _ := extractExif(a.cfg.ExifTool, updatedData, filepath.Ext(key))
	sets = append(sets, "exif=?", "file_size=?", "last_modified=?")
	args = append(args, jsonValue(exif), len(updatedData), metadataTimestamp(), id)
	if _, err := a.db.Exec(`UPDATE photos SET `+strings.Join(sets, ",")+` WHERE id=?`, args...); err != nil {
		errorJSON(w, http.StatusInternalServerError, "Failed to update photo")
		return
	}
	if body.Location != nil && location != nil {
		payload, _ := json.Marshal(map[string]any{
			"type":      "photo-reverse-geocoding",
			"photoId":   id,
			"latitude":  location["latitude"],
			"longitude": location["longitude"],
		})
		if _, queueErr := a.db.Exec(`INSERT INTO pipeline_queue(payload,priority,max_attempts,status) VALUES(?,?,?,'pending')`, string(payload), 1, 3); queueErr != nil {
			a.logs.Add("queue", "failed to enqueue reverse geocoding for "+id+": "+queueErr.Error())
		} else {
			a.wakeQueue()
		}
	}
	updated, err := a.photoByID(id)
	if err != nil {
		errorJSON(w, http.StatusInternalServerError, "Failed to load updated photo")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "photo": updated})
}

func normalizeTags(tags []string) []string {
	seen := map[string]bool{}
	result := []string{}
	for _, tag := range tags {
		tag = strings.TrimSpace(tag)
		key := strings.ToLower(tag)
		if tag != "" && !seen[key] {
			seen[key] = true
			result = append(result, tag)
		}
	}
	return result
}
func (a *App) deletePhoto(w http.ResponseWriter, r *http.Request, id string) {
	if _, ok := a.require(r); !ok {
		errorJSON(w, http.StatusUnauthorized, "Unauthorized")
		return
	}
	var key, thumb, liveVideo string
	if a.db.QueryRow(`SELECT storage_key,thumbnail_key,COALESCE(live_photo_video_key,'') FROM photos WHERE id=?`, id).Scan(&key, &thumb, &liveVideo) != nil {
		errorJSON(w, 404, "Photo not found")
		return
	}
	_ = a.storage.Delete(r.Context(), key)
	if thumb != "" {
		_ = a.storage.Delete(r.Context(), thumb)
	}
	if liveVideo != "" {
		_ = a.storage.Delete(r.Context(), liveVideo)
	}
	// HEIC processing may leave a converted JPEG alongside the original.
	lowerKey := strings.ToLower(key)
	for _, ext := range []string{".heic", ".heif", ".hif"} {
		if strings.HasSuffix(lowerKey, ext) {
			jpegKey := key[:len(key)-len(ext)] + ".jpeg"
			if jpegKey != key {
				_ = a.storage.Delete(r.Context(), jpegKey)
			}
			break
		}
	}
	_, _ = a.db.Exec(`DELETE FROM photos WHERE id=?`, id)
	writeJSON(w, http.StatusOK, map[string]any{"statusCode": 200, "statusMessage": "Photo deleted successfully"})
}
func (a *App) photoReaction(w http.ResponseWriter, r *http.Request, id string) {
	fingerprint := reactionFingerprint(r)
	if r.Method == http.MethodGet {
		rows, err := a.db.Query(`SELECT reaction_type,count(*) FROM photo_reactions WHERE photo_id=? GROUP BY reaction_type`, id)
		if err != nil {
			errorJSON(w, http.StatusInternalServerError, "Unable to load reactions")
			return
		}
		defer rows.Close()
		counts := newReactionCounts()
		for rows.Next() {
			var typ string
			var count int
			if rows.Scan(&typ, &count) == nil && counts[typ] >= 0 {
				counts[typ] = count
			}
		}
		var current sql.NullString
		_ = a.db.QueryRow(`SELECT reaction_type FROM photo_reactions WHERE photo_id=? AND fingerprint=? ORDER BY id DESC LIMIT 1`, id, fingerprint).Scan(&current)
		var userReaction any
		if current.Valid {
			userReaction = current.String
		}
		writeJSON(w, http.StatusOK, map[string]any{"photoId": id, "reactions": counts, "userReaction": userReaction})
		return
	}
	if r.Method == http.MethodDelete {
		var reactionID int64
		if err := a.db.QueryRow(`SELECT id FROM photo_reactions WHERE photo_id=? AND fingerprint=? ORDER BY id DESC LIMIT 1`, id, fingerprint).Scan(&reactionID); err != nil {
			errorJSON(w, http.StatusNotFound, "Reaction not found")
			return
		}
		_, _ = a.db.Exec(`DELETE FROM photo_reactions WHERE id=?`, reactionID)
		writeJSON(w, http.StatusOK, map[string]any{"success": true, "action": "deleted"})
		return
	}
	if r.Method != http.MethodPost {
		errorJSON(w, http.StatusMethodNotAllowed, "Method Not Allowed")
		return
	}
	var body struct {
		ReactionType string `json:"reactionType"`
	}
	if decodeJSON(r, &body) != nil || !validReactionType(body.ReactionType) {
		errorJSON(w, http.StatusBadRequest, "Invalid reaction type")
		return
	}
	var photoExists int
	if err := a.db.QueryRow(`SELECT 1 FROM photos WHERE id=?`, id).Scan(&photoExists); err != nil {
		errorJSON(w, http.StatusNotFound, "Photo not found")
		return
	}
	var recent int
	_ = a.db.QueryRow(`SELECT count(*) FROM photo_reactions WHERE fingerprint=? AND created_at>?`, fingerprint, time.Now().Add(-time.Minute).Unix()).Scan(&recent)
	if recent >= 10 {
		errorJSON(w, http.StatusTooManyRequests, "Too many reactions. Please try again later.")
		return
	}
	var existingID int64
	var saveErr error
	if err := a.db.QueryRow(`SELECT id FROM photo_reactions WHERE photo_id=? AND fingerprint=? ORDER BY id DESC LIMIT 1`, id, fingerprint).Scan(&existingID); err == nil {
		_, saveErr = a.db.Exec(`UPDATE photo_reactions SET reaction_type=?,updated_at=unixepoch(),ip_address=?,user_agent=? WHERE id=?`, body.ReactionType, remoteIP(r), r.UserAgent(), existingID)
	} else {
		_, saveErr = a.db.Exec(`INSERT INTO photo_reactions(photo_id,reaction_type,fingerprint,ip_address,user_agent,created_at,updated_at) VALUES(?,?,?,?,?,unixepoch(),unixepoch())`, id, body.ReactionType, fingerprint, remoteIP(r), r.UserAgent())
	}
	if saveErr != nil {
		errorJSON(w, http.StatusInternalServerError, "Unable to save reaction")
		return
	}
	action := "created"
	if existingID != 0 {
		action = "updated"
	}
	writeJSON(w, http.StatusOK, map[string]any{"photoId": id, "reactionType": body.ReactionType, "success": true, "action": action})
}

func reactionFingerprint(r *http.Request) string {
	value := remoteIP(r) + "|" + valueOrUnknown(r.UserAgent()) + "|" + valueOrUnknown(r.Header.Get("Accept-Language")) + "|" + valueOrUnknown(r.Header.Get("Accept-Encoding"))
	return base64.StdEncoding.EncodeToString([]byte(value))
}
func valueOrUnknown(value string) string {
	if strings.TrimSpace(value) == "" {
		return "unknown"
	}
	return value
}
func remoteIP(r *http.Request) string {
	if forwarded := strings.TrimSpace(strings.Split(r.Header.Get("X-Forwarded-For"), ",")[0]); forwarded != "" {
		return forwarded
	}
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}
	return r.RemoteAddr
}
func validReactionType(value string) bool {
	switch value {
	case "like", "love", "amazing", "funny", "wow", "sad", "fire", "sparkle":
		return true
	default:
		return false
	}
}
