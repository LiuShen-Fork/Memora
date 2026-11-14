package main

import (
	"database/sql"
	"net/http"
	"time"
)

func (a *App) photoAlbums(w http.ResponseWriter, photoID string) {
	rows, err := a.db.Query(`SELECT al.id,al.title,al.slug,al.description,al.cover_photo_id,al.created_at,al.updated_at FROM albums al INNER JOIN album_photos ap ON al.id=ap.album_id WHERE ap.photo_id=? ORDER BY al.created_at DESC`, photoID)
	if err != nil {
		errorJSON(w, http.StatusInternalServerError, "Unable to load photo albums")
		return
	}
	albums := []map[string]any{}
	for rows.Next() {
		var id, createdAt, updatedAt int64
		var title string
		var description, coverPhotoID, slug sql.NullString
		if err := rows.Scan(&id, &title, &slug, &description, &coverPhotoID, &createdAt, &updatedAt); err != nil {
			_ = rows.Close()
			errorJSON(w, http.StatusInternalServerError, "Unable to read photo album")
			return
		}
		albums = append(albums, map[string]any{
			"id":           id,
			"title":        title,
			"slug":         slug.String,
			"description":  nullString(description),
			"coverPhotoId": nullString(coverPhotoID),
			"createdAt":    time.Unix(createdAt, 0),
			"updatedAt":    time.Unix(updatedAt, 0),
		})
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		errorJSON(w, http.StatusInternalServerError, "Unable to read photo albums")
		return
	}
	if err := rows.Close(); err != nil {
		errorJSON(w, http.StatusInternalServerError, "Unable to read photo albums")
		return
	}
	writeJSON(w, http.StatusOK, albums)
}
