package main

import (
	"context"
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
		VideoKey string   `json:"videoKey"`
		PhotoID  string   `json:"photoId"`
		PhotoIDs []string `json:"photoIds"`
	}
	if decodeJSON(r, &body) != nil || body.Action == "" {
		errorJSON(w, http.StatusBadRequest, "Action is required")
		return
	}
	if body.Action == "scan" {
		rows, err := a.db.Query(`SELECT id FROM photos WHERE storage_key IS NOT NULL`)
		if err != nil {
			errorJSON(w, http.StatusInternalServerError, "Unable to scan photos")
			return
		}
		defer rows.Close()
		results := []map[string]any{}
		for rows.Next() {
			var id string
			if rows.Scan(&id) != nil {
				continue
			}
			success, key, err := a.detectLivePhoto(r, id)
			result := map[string]any{"photoId": id, "success": success, "videoKey": key}
			if err != nil {
				result["error"] = err.Error()
			}
			results = append(results, result)
		}
		writeJSON(w, http.StatusOK, map[string]any{"message": "Scan completed", "results": results})
		return
	}
	if body.Action == "process" {
		if body.VideoKey == "" {
			errorJSON(w, http.StatusBadRequest, "videoKey is required for process action")
			return
		}
		success, err := a.processSpecificLivePhotoVideo(r.Context(), body.VideoKey)
		if err != nil {
			errorJSON(w, http.StatusInternalServerError, "Failed to process LivePhoto")
			return
		}
		message := "Failed to process LivePhoto"
		if success {
			message = "LivePhoto processed successfully"
		}
		writeJSON(w, http.StatusOK, map[string]any{"message": message, "success": success, "videoKey": body.VideoKey})
		return
	}
	if body.Action == "update-photo" {
		if body.PhotoID == "" {
			errorJSON(w, http.StatusBadRequest, "photoId is required for update-photo action")
			return
		}
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
		writeJSON(w, http.StatusOK, map[string]any{"message": "Batch LivePhoto detection completed", "success": true, "results": results})
		return
	}
	errorJSON(w, http.StatusBadRequest, "Supported actions are scan, detect, process, and update-photo")
}

// processSpecificLivePhotoVideo mirrors the Nuxt management action: only
// small MOV files are treated as Live Photos and the matching image record is
// updated with the video's public URL and storage key.
func (a *App) processSpecificLivePhotoVideo(ctx context.Context, videoKey string) (bool, error) {
	object, err := a.storage.Meta(ctx, videoKey)
	if err != nil {
		return false, nil
	}
	size := object.Size
	if size <= 0 {
		data, getErr := a.storage.Get(ctx, videoKey)
		if getErr != nil {
			return false, nil
		}
		size = int64(len(data))
	}
	ext := strings.ToLower(filepath.Ext(videoKey))
	if ext != ".mov" || size > 12*1024*1024 {
		return false, nil
	}
	base := strings.TrimSuffix(videoKey, filepath.Ext(videoKey))
	for _, imageExt := range []string{".HEIC", ".heic", ".JPG", ".jpg", ".JPEG", ".jpeg"} {
		candidate := base + imageExt
		var photoID string
		if err := a.db.QueryRow(`SELECT id FROM photos WHERE storage_key=? OR id=? LIMIT 1`, candidate, safePhotoID(candidate)).Scan(&photoID); err != nil {
			continue
		}
		url := a.storage.PublicURL(videoKey)
		if url == "" {
			url = "/image/" + strings.TrimLeft(videoKey, "/")
		}
		_, err = a.db.Exec(`UPDATE photos SET is_live_photo=1,live_photo_video_url=?,live_photo_video_key=? WHERE id=?`, url, videoKey, photoID)
		return err == nil, err
	}
	return false, nil
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
