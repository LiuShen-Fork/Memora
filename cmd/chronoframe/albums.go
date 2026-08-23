package main

import (
	"database/sql"
	"net/http"
	"strconv"
	"strings"
	"time"
)

func (a *App) albums(w http.ResponseWriter, r *http.Request) {
	rows, err := a.db.Query(`SELECT id,title,description,cover_photo_id,is_hidden,created_at,updated_at FROM albums ORDER BY created_at DESC`)
	if err != nil {
		errorJSON(w, 500, err.Error())
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id int64
		var title string
		var description, cover sql.NullString
		var hidden int
		var created, updated int64
		_ = rows.Scan(&id, &title, &description, &cover, &hidden, &created, &updated)
		ids := []string{}
		pRows, _ := a.db.Query(`SELECT photo_id FROM album_photos WHERE album_id=? ORDER BY position`, id)
		for pRows != nil && pRows.Next() {
			var photoID string
			_ = pRows.Scan(&photoID)
			ids = append(ids, photoID)
		}
		if pRows != nil {
			pRows.Close()
		}
		out = append(out, map[string]any{"id": id, "title": title, "description": nullString(description), "coverPhotoId": nullString(cover), "isHidden": hidden == 1, "createdAt": time.Unix(created, 0), "updatedAt": time.Unix(updated, 0), "photoIds": ids})
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
	if !a.requireAdmin(w, r) {
		return
	}
	var b struct {
		Title        string
		Description  string
		CoverPhotoID string   `json:"coverPhotoId"`
		PhotoIDs     []string `json:"photoIds"`
		IsHidden     bool     `json:"isHidden"`
	}
	if decodeJSON(r, &b) != nil || strings.TrimSpace(b.Title) == "" {
		errorJSON(w, 400, "title is required")
		return
	}
	tx, err := a.db.Begin()
	if err != nil {
		errorJSON(w, 500, err.Error())
		return
	}
	res, err := tx.Exec(`INSERT INTO albums(title,description,cover_photo_id,is_hidden) VALUES(?,?,?,?)`, b.Title, b.Description, b.CoverPhotoID, boolInt(b.IsHidden))
	if err != nil {
		tx.Rollback()
		errorJSON(w, 500, err.Error())
		return
	}
	id, _ := res.LastInsertId()
	for i, pid := range uniqueStrings(append(b.PhotoIDs, b.CoverPhotoID)) {
		if pid != "" {
			_, _ = tx.Exec(`INSERT INTO album_photos(album_id,photo_id,position) VALUES(?,?,?)`, id, pid, float64(i+1)*10)
		}
	}
	if err := tx.Commit(); err != nil {
		errorJSON(w, 500, err.Error())
		return
	}
	writeJSON(w, 201, map[string]any{"id": id, "title": b.Title, "description": b.Description, "coverPhotoId": b.CoverPhotoID, "isHidden": b.IsHidden, "photoIds": b.PhotoIDs})
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
func (a *App) albumRoute(w http.ResponseWriter, r *http.Request, rest string) {
	parts := strings.Split(strings.Trim(rest, "/"), "/")
	if len(parts) == 0 {
		return
	}
	id, _ := strconv.ParseInt(parts[0], 10, 64)
	if len(parts) == 1 && r.Method == "GET" {
		a.albumDetail(w, r, id)
		return
	}
	if len(parts) == 1 && r.Method == "PUT" {
		a.updateAlbum(w, r, id)
		return
	}
	if len(parts) == 1 && r.Method == "DELETE" {
		if !a.requireAdmin(w, r) {
			return
		}
		_, _ = a.db.Exec(`DELETE FROM albums WHERE id=?`, id)
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if len(parts) >= 3 && parts[1] == "photos" && r.Method == "DELETE" {
		if !a.requireAdmin(w, r) {
			return
		}
		_, _ = a.db.Exec(`DELETE FROM album_photos WHERE album_id=? AND photo_id=?`, id, parts[2])
		w.WriteHeader(http.StatusNoContent)
		return
	}
	errorJSON(w, 404, "Not Found")
}
func (a *App) albumDetail(w http.ResponseWriter, _ *http.Request, id int64) {
	var title string
	var description, cover sql.NullString
	var hidden int
	if a.db.QueryRow(`SELECT title,description,cover_photo_id,is_hidden FROM albums WHERE id=?`, id).Scan(&title, &description, &cover, &hidden) != nil {
		errorJSON(w, 404, "Album not found")
		return
	}
	rows, _ := a.db.Query(photoSelect+` WHERE id IN (SELECT photo_id FROM album_photos WHERE album_id=?) ORDER BY date_taken DESC`, id)
	photos := []Photo{}
	for rows != nil && rows.Next() {
		p, e := scanPhoto(rows)
		if e == nil {
			photos = append(photos, p)
		}
	}
	if rows != nil {
		rows.Close()
	}
	writeJSON(w, 200, map[string]any{"id": id, "title": title, "description": nullString(description), "coverPhotoId": nullString(cover), "isHidden": hidden == 1, "photos": photos})
}
func (a *App) updateAlbum(w http.ResponseWriter, r *http.Request, id int64) {
	if !a.requireAdmin(w, r) {
		return
	}
	var b map[string]any
	if decodeJSON(r, &b) != nil {
		errorJSON(w, 400, "Invalid request")
		return
	}
	sets := []string{}
	args := []any{}
	for _, key := range []string{"title", "description", "coverPhotoId"} {
		if v, ok := b[key]; ok {
			sets = append(sets, keyToColumn(key)+"=?")
			args = append(args, v)
		}
	}
	if v, ok := b["isHidden"].(bool); ok {
		sets = append(sets, "is_hidden=?")
		args = append(args, boolInt(v))
	}
	if len(sets) > 0 {
		args = append(args, id)
		_, _ = a.db.Exec(`UPDATE albums SET `+strings.Join(sets, ",")+`,updated_at=unixepoch() WHERE id=?`, args...)
	}
	writeJSON(w, 200, map[string]any{"success": true})
}
func keyToColumn(key string) string {
	if key == "coverPhotoId" {
		return "cover_photo_id"
	}
	return key
}
