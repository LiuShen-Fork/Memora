package main

import (
	"net/http"
	"path/filepath"
	"strings"
)

func (a *App) manageLivePhoto(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.require(r); !ok {
		errorJSON(w, http.StatusUnauthorized, "Unauthorized")
		return
	}
	var body struct {
		Action   string   `json:"action"`
		PhotoID  string   `json:"photoId"`
		PhotoIDs []string `json:"photoIds"`
	}
	if decodeJSON(r, &body) != nil || body.Action == "" {
		errorJSON(w, http.StatusBadRequest, "Action is required")
		return
	}
	if body.Action == "update-photo" {
		success, key, err := a.detectLivePhoto(r, body.PhotoID)
		if err != nil {
			errorJSON(w, http.StatusNotFound, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"success": success, "photoId": body.PhotoID, "videoKey": key})
		return
	}
	if body.Action == "detect" {
		if len(body.PhotoIDs) == 0 {
			errorJSON(w, http.StatusBadRequest, "photoIds is required for batch detection")
			return
		}
		results := make([]map[string]any, 0, len(body.PhotoIDs))
		for _, id := range uniqueStrings(body.PhotoIDs) {
			success, key, err := a.detectLivePhoto(r, id)
			result := map[string]any{"photoId": id, "success": success, "videoKey": key}
			if err != nil {
				result["error"] = err.Error()
			}
			results = append(results, result)
		}
		writeJSON(w, http.StatusOK, map[string]any{"success": true, "results": results})
		return
	}
	errorJSON(w, http.StatusBadRequest, "Supported actions are update-photo and detect")
}

func (a *App) detectLivePhoto(r *http.Request, photoID string) (bool, string, error) {
	if photoID == "" {
		return false, "", errPhotoIDRequired
	}
	var imageKey string
	if err := a.db.QueryRow(`SELECT storage_key FROM photos WHERE id=?`, photoID).Scan(&imageKey); err != nil {
		return false, "", errPhotoNotFound
	}
	base := strings.TrimSuffix(imageKey, filepath.Ext(imageKey))
	for _, extension := range []string{".mov", ".mp4"} {
		candidate := base + extension
		if _, err := a.storage.Meta(r.Context(), candidate); err != nil {
			continue
		}
		url := a.storage.PublicURL(candidate)
		if url == "" {
			url = "/image/" + candidate
		}
		_, err := a.db.Exec(`UPDATE photos SET is_live_photo=1,live_photo_video_key=?,live_photo_video_url=? WHERE id=?`, candidate, url, photoID)
		return err == nil, candidate, err
	}
	_, err := a.db.Exec(`UPDATE photos SET is_live_photo=0,live_photo_video_key=NULL,live_photo_video_url=NULL WHERE id=?`, photoID)
	return false, "", err
}

var (
	errPhotoIDRequired = &livePhotoError{"Photo ID is required"}
	errPhotoNotFound   = &livePhotoError{"Photo not found"}
)

type livePhotoError struct{ message string }

func (e *livePhotoError) Error() string { return e.message }
