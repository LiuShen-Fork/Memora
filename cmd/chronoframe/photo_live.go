package main

import "net/http"

func (a *App) photoLive(w http.ResponseWriter, r *http.Request, id string) {
	if _, ok := a.require(r); !ok {
		errorJSON(w, http.StatusUnauthorized, "Unauthorized")
		return
	}
	rows, err := a.db.Query(photoSelect+` WHERE id=?`, id)
	if err != nil {
		errorJSON(w, http.StatusInternalServerError, "Unable to load photo")
		return
	}
	defer rows.Close()
	if !rows.Next() {
		errorJSON(w, http.StatusNotFound, "Photo not found")
		return
	}
	photo, err := scanPhoto(rows)
	if err != nil {
		errorJSON(w, http.StatusInternalServerError, "Unable to load photo")
		return
	}
	isLivePhoto, _ := photo.IsLivePhoto.(int64)
	writeJSON(w, http.StatusOK, map[string]any{
		"id":                photo.ID,
		"title":             photo.Title,
		"isLivePhoto":       isLivePhoto != 0,
		"livePhotoVideoUrl": photo.LivePhotoVideoURL,
		"originalUrl":       photo.OriginalURL,
		"thumbnailUrl":      photo.ThumbnailURL,
	})
}
