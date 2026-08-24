package main

import (
	"database/sql"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

func (a *App) albums(w http.ResponseWriter, r *http.Request) {
	rows, err := a.db.QueryContext(r.Context(), `
		SELECT al.id,al.title,al.description,al.cover_photo_id,al.is_hidden,al.created_at,al.updated_at,ap.photo_id
		FROM albums al
		LEFT JOIN album_photos ap ON ap.album_id=al.id
		ORDER BY al.created_at DESC,al.id DESC,ap.position ASC,ap.id ASC`)
	if err != nil {
		errorJSON(w, 500, err.Error())
		return
	}
	type albumRow struct {
		id                 int64
		title              string
		description, cover sql.NullString
		hidden             int
		created, updated   int64
		photoIDs           []string
	}
	albumRows := []albumRow{}
	for rows.Next() {
		var album albumRow
		var photoID sql.NullString
		if err := rows.Scan(&album.id, &album.title, &album.description, &album.cover, &album.hidden, &album.created, &album.updated, &photoID); err != nil {
			_ = rows.Close()
			errorJSON(w, http.StatusInternalServerError, "Unable to read albums")
			return
		}
		if len(albumRows) == 0 || albumRows[len(albumRows)-1].id != album.id {
			album.photoIDs = make([]string, 0, 8)
			albumRows = append(albumRows, album)
		}
		if photoID.Valid {
			albumRows[len(albumRows)-1].photoIDs = append(albumRows[len(albumRows)-1].photoIDs, photoID.String)
		}
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		errorJSON(w, http.StatusInternalServerError, "Unable to read albums")
		return
	}
	_ = rows.Close()
	out := []map[string]any{}
	for _, album := range albumRows {
		out = append(out, map[string]any{"id": album.id, "title": album.title, "description": nullString(album.description), "coverPhotoId": nullString(album.cover), "isHidden": album.hidden == 1, "createdAt": time.Unix(album.created, 0), "updatedAt": time.Unix(album.updated, 0), "photoIds": album.photoIDs})
	}
	writeJSON(w, 200, out)
}
func nullString(v sql.NullString) any {
	if v.Valid {
		return v.String
	}
	return nil
}
func (a *App) createAlbum(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.require(r); !ok {
		errorJSON(w, http.StatusUnauthorized, "Unauthorized")
		return
	}
	b, err := decodeAlbumPayload(r)
	if err != nil {
		errorJSON(w, http.StatusBadRequest, err.Error())
		return
	}
	title, ok := b["title"].(string)
	if !ok || title == "" {
		errorJSON(w, http.StatusBadRequest, "title is required")
		return
	}
	description := nullableAlbumString(b, "description")
	coverPhotoID := nullableAlbumString(b, "coverPhotoId")
	photoIDs := albumPhotoIDs(b)
	if cover, ok := coverPhotoID.(string); ok {
		photoIDs = append(photoIDs, cover)
	}
	isHidden, _ := b["isHidden"].(bool)
	tx, err := a.db.Begin()
	if err != nil {
		errorJSON(w, 500, err.Error())
		return
	}
	res, err := tx.Exec(`INSERT INTO albums(title,description,cover_photo_id,is_hidden) VALUES(?,?,?,?)`, title, description, coverPhotoID, boolInt(isHidden))
	if err != nil {
		tx.Rollback()
		errorJSON(w, 500, err.Error())
		return
	}
	id, _ := res.LastInsertId()
	if _, err := insertAlbumPhotos(tx, id, photoIDs, 1000010); err != nil {
		_ = tx.Rollback()
		errorJSON(w, http.StatusBadRequest, "Unable to add photos to album")
		return
	}
	if err := tx.Commit(); err != nil {
		errorJSON(w, 500, err.Error())
		return
	}
	var created, updated int64
	if err := a.db.QueryRow(`SELECT created_at,updated_at FROM albums WHERE id=?`, id).Scan(&created, &updated); err != nil {
		errorJSON(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": id, "title": title, "description": description, "coverPhotoId": coverPhotoID, "isHidden": isHidden, "createdAt": time.Unix(created, 0), "updatedAt": time.Unix(updated, 0)})
}

func decodeAlbumPayload(r *http.Request) (map[string]any, error) {
	var body map[string]any
	if err := decodeJSON(r, &body); err != nil || body == nil {
		return nil, fmt.Errorf("Invalid request")
	}
	if value, ok := body["title"]; ok {
		title, valid := value.(string)
		if !valid || len(title) < 1 || len(title) > 255 {
			return nil, fmt.Errorf("Invalid title")
		}
	}
	if value, ok := body["description"]; ok {
		description, valid := value.(string)
		if !valid || len(description) > 1000 {
			return nil, fmt.Errorf("Invalid description")
		}
	}
	if value, ok := body["coverPhotoId"]; ok {
		if _, valid := value.(string); !valid {
			return nil, fmt.Errorf("Invalid coverPhotoId")
		}
	}
	if value, ok := body["photoIds"]; ok {
		items, valid := value.([]any)
		if !valid || len(items) > 1000 {
			return nil, fmt.Errorf("Invalid photoIds")
		}
		for _, item := range items {
			if _, valid := item.(string); !valid {
				return nil, fmt.Errorf("Invalid photoIds")
			}
		}
	}
	if value, ok := body["isHidden"]; ok {
		if _, valid := value.(bool); !valid {
			return nil, fmt.Errorf("Invalid isHidden")
		}
	}
	return body, nil
}

func nullableAlbumString(body map[string]any, key string) any {
	value, ok := body[key].(string)
	if !ok || value == "" {
		return nil
	}
	return value
}

func albumPhotoIDs(body map[string]any) []string {
	items, _ := body["photoIds"].([]any)
	ids := make([]string, 0, len(items))
	for _, item := range items {
		ids = append(ids, item.(string))
	}
	return ids
}
func boolInt(v bool) int {
	if v {
		return 1
	}
	return 0
}
func uniqueStrings(items []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, v := range items {
		if v != "" && !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	return out
}

func insertAlbumPhotos(tx *sql.Tx, albumID int64, photoIDs []string, startPosition float64) (int64, error) {
	ids := uniqueStrings(photoIDs)
	if len(ids) == 0 {
		return 0, nil
	}
	values := make([]string, len(ids))
	args := make([]any, 0, len(ids)*3)
	for i, photoID := range ids {
		values[i] = "(?,?,?)"
		args = append(args, albumID, photoID, startPosition+float64(i*10))
	}
	result, err := tx.Exec(`INSERT OR IGNORE INTO album_photos(album_id,photo_id,position) VALUES `+strings.Join(values, ","), args...)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}
func (a *App) albumRoute(w http.ResponseWriter, r *http.Request, rest string) {
	parts := strings.Split(strings.Trim(rest, "/"), "/")
	if len(parts) == 0 {
		return
	}
	if parts[0] == "" || strings.Trim(parts[0], "0123456789") != "" {
		errorJSON(w, http.StatusBadRequest, "Invalid album ID")
		return
	}
	id, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		errorJSON(w, http.StatusBadRequest, "Invalid album ID")
		return
	}
	if len(parts) == 2 && parts[1] == "photos" && r.Method == http.MethodPut {
		a.updateAlbumPhotos(w, r, id)
		return
	}
	if len(parts) == 1 && r.Method == "GET" {
		a.albumDetail(w, r, id)
		return
	}
	if len(parts) == 1 && r.Method == "PUT" {
		a.updateAlbum(w, r, id)
		return
	}
	if len(parts) == 1 && r.Method == "DELETE" {
		if _, ok := a.require(r); !ok {
			errorJSON(w, http.StatusUnauthorized, "Unauthorized")
			return
		}
		var exists int
		if a.db.QueryRow(`SELECT 1 FROM albums WHERE id=?`, id).Scan(&exists) != nil {
			errorJSON(w, http.StatusNotFound, "Album not found")
			return
		}
		_, _ = a.db.Exec(`DELETE FROM albums WHERE id=?`, id)
		writeJSON(w, http.StatusOK, map[string]bool{"success": true})
		return
	}
	if len(parts) >= 3 && parts[1] == "photos" && r.Method == "DELETE" {
		if _, ok := a.require(r); !ok {
			errorJSON(w, http.StatusUnauthorized, "Unauthorized")
			return
		}
		var exists int
		if a.db.QueryRow(`SELECT 1 FROM album_photos WHERE album_id=? AND photo_id=?`, id, parts[2]).Scan(&exists) != nil {
			errorJSON(w, http.StatusNotFound, "Photo not found in album")
			return
		}
		_, _ = a.db.Exec(`DELETE FROM album_photos WHERE album_id=? AND photo_id=?`, id, parts[2])
		_, _ = a.db.Exec(`UPDATE albums SET cover_photo_id=NULL,updated_at=unixepoch() WHERE id=? AND cover_photo_id=?`, id, parts[2])
		writeJSON(w, http.StatusOK, map[string]bool{"success": true})
		return
	}
	errorJSON(w, 404, "Not Found")
}

// updateAlbumPhotos applies a batch membership change in one transaction.
// Keeping this server-side avoids one read/write round trip per selected photo.
func (a *App) updateAlbumPhotos(w http.ResponseWriter, r *http.Request, albumID int64) {
	if _, ok := a.require(r); !ok {
		errorJSON(w, http.StatusUnauthorized, "Unauthorized")
		return
	}
	var body struct {
		Action   string   `json:"action"`
		PhotoIDs []string `json:"photoIds"`
	}
	if decodeJSON(r, &body) != nil {
		errorJSON(w, http.StatusBadRequest, "Invalid request")
		return
	}
	body.Action = strings.ToLower(strings.TrimSpace(body.Action))
	if body.Action != "add" && body.Action != "remove" {
		errorJSON(w, http.StatusBadRequest, "action must be add or remove")
		return
	}
	photoIDs := uniqueStrings(body.PhotoIDs)
	if len(photoIDs) == 0 || len(photoIDs) > 1000 {
		errorJSON(w, http.StatusBadRequest, "photoIds must contain between 1 and 1000 items")
		return
	}

	tx, err := a.db.BeginTx(r.Context(), nil)
	if err != nil {
		errorJSON(w, http.StatusInternalServerError, "Failed to update album photos")
		return
	}
	defer tx.Rollback()
	var exists int
	if err := tx.QueryRow(`SELECT 1 FROM albums WHERE id=?`, albumID).Scan(&exists); err != nil {
		errorJSON(w, http.StatusNotFound, "Album not found")
		return
	}

	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(photoIDs)), ",")
	args := make([]any, 0, len(photoIDs)+1)
	args = append(args, albumID)
	for _, photoID := range photoIDs {
		args = append(args, photoID)
	}
	changed := int64(0)
	if body.Action == "add" {
		var position float64
		_ = tx.QueryRow(`SELECT COALESCE(MAX(position),1000000) FROM album_photos WHERE album_id=?`, albumID).Scan(&position)
		var insertErr error
		changed, insertErr = insertAlbumPhotos(tx, albumID, photoIDs, position+10)
		if insertErr != nil {
			errorJSON(w, http.StatusBadRequest, "Unable to add photo to album")
			return
		}
	} else {
		result, deleteErr := tx.Exec(`DELETE FROM album_photos WHERE album_id=? AND photo_id IN (`+placeholders+`)`, args...)
		if deleteErr != nil {
			errorJSON(w, http.StatusInternalServerError, "Unable to remove photos from album")
			return
		}
		changed, _ = result.RowsAffected()
		args = []any{albumID}
		for _, photoID := range photoIDs {
			args = append(args, photoID)
		}
		if _, err := tx.Exec(`UPDATE albums SET cover_photo_id=NULL WHERE id=? AND cover_photo_id IN (`+placeholders+")", args...); err != nil {
			errorJSON(w, http.StatusInternalServerError, "Unable to update album cover")
			return
		}
	}
	if _, err := tx.Exec(`UPDATE albums SET updated_at=unixepoch() WHERE id=?`, albumID); err != nil {
		errorJSON(w, http.StatusInternalServerError, "Unable to update album")
		return
	}
	if err := tx.Commit(); err != nil {
		errorJSON(w, http.StatusInternalServerError, "Failed to update album photos")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "action": body.Action, "changed": changed})
}

func (a *App) albumDetail(w http.ResponseWriter, r *http.Request, id int64) {
	var title string
	var description, cover sql.NullString
	var hidden int
	var created, updated int64
	if a.db.QueryRow(`SELECT title,description,cover_photo_id,is_hidden,created_at,updated_at FROM albums WHERE id=?`, id).Scan(&title, &description, &cover, &hidden, &created, &updated) != nil {
		errorJSON(w, 404, "Album not found")
		return
	}
	if hidden == 1 {
		if _, ok := a.require(r); !ok {
			errorJSON(w, http.StatusNotFound, "Album not found")
			return
		}
	}
	rows, err := a.db.QueryContext(r.Context(), photoSelect+` INNER JOIN album_photos ap ON ap.photo_id=photos.id WHERE ap.album_id=? ORDER BY ap.position ASC,ap.id ASC`, id)
	if err != nil {
		errorJSON(w, http.StatusInternalServerError, "Unable to read album photos")
		return
	}
	photos := []Photo{}
	for rows.Next() {
		p, e := scanPhoto(rows)
		if e != nil {
			_ = rows.Close()
			errorJSON(w, http.StatusInternalServerError, "Unable to read album photos")
			return
		}
		photos = append(photos, p)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		errorJSON(w, http.StatusInternalServerError, "Unable to read album photos")
		return
	}
	if err := rows.Close(); err != nil {
		errorJSON(w, http.StatusInternalServerError, "Unable to read album photos")
		return
	}
	writeJSON(w, 200, map[string]any{"id": id, "title": title, "description": nullString(description), "coverPhotoId": nullString(cover), "isHidden": hidden == 1, "createdAt": time.Unix(created, 0), "updatedAt": time.Unix(updated, 0), "photos": photos})
}
func (a *App) updateAlbum(w http.ResponseWriter, r *http.Request, id int64) {
	if _, ok := a.require(r); !ok {
		errorJSON(w, http.StatusUnauthorized, "Unauthorized")
		return
	}
	b, err := decodeAlbumPayload(r)
	if err != nil {
		errorJSON(w, http.StatusBadRequest, err.Error())
		return
	}
	var exists int
	if a.db.QueryRow(`SELECT 1 FROM albums WHERE id=?`, id).Scan(&exists) != nil {
		errorJSON(w, http.StatusNotFound, "Album not found")
		return
	}
	tx, err := a.db.Begin()
	if err != nil {
		errorJSON(w, http.StatusInternalServerError, "Failed to update album")
		return
	}
	defer tx.Rollback()
	sets := []string{}
	args := []any{}
	for _, key := range []string{"title", "description", "coverPhotoId"} {
		if v, ok := b[key]; ok {
			sets = append(sets, keyToColumn(key)+"=?")
			if key == "description" || key == "coverPhotoId" {
				args = append(args, nullableAlbumString(b, key))
			} else {
				args = append(args, v)
			}
		}
	}
	if v, ok := b["isHidden"].(bool); ok {
		sets = append(sets, "is_hidden=?")
		args = append(args, boolInt(v))
	}
	if _, ok := b["photoIds"]; ok {
		_, _ = tx.Exec(`DELETE FROM album_photos WHERE album_id=?`, id)
		photoIDs := albumPhotoIDs(b)
		if cover, ok := b["coverPhotoId"].(string); ok && cover != "" {
			photoIDs = append(photoIDs, cover)
		}
		if _, err := insertAlbumPhotos(tx, id, photoIDs, 1000010); err != nil {
			errorJSON(w, http.StatusBadRequest, "Unable to add photos to album")
			return
		}
	}
	sets = append(sets, "updated_at=unixepoch()")
	args = append(args, id)
	if _, err := tx.Exec(`UPDATE albums SET `+strings.Join(sets, ",")+` WHERE id=?`, args...); err != nil {
		errorJSON(w, http.StatusInternalServerError, "Failed to update album")
		return
	}
	if err := tx.Commit(); err != nil {
		errorJSON(w, http.StatusInternalServerError, "Failed to update album")
		return
	}
	var title string
	var description, cover sql.NullString
	var hidden int
	var created, updated int64
	if err := a.db.QueryRow(`SELECT title,description,cover_photo_id,is_hidden,created_at,updated_at FROM albums WHERE id=?`, id).Scan(&title, &description, &cover, &hidden, &created, &updated); err != nil {
		errorJSON(w, http.StatusNotFound, "Album not found")
		return
	}
	writeJSON(w, 200, map[string]any{"id": id, "title": title, "description": nullString(description), "coverPhotoId": nullString(cover), "isHidden": hidden == 1, "createdAt": time.Unix(created, 0), "updatedAt": time.Unix(updated, 0)})
}
func keyToColumn(key string) string {
	if key == "coverPhotoId" {
		return "cover_photo_id"
	}
	return key
}
