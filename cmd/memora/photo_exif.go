package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"time"
)

type exifReindexResult struct {
	PhotoID string `json:"photoId"`
	Error   string `json:"error,omitempty"`
}

func (a *App) reindexExif(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.require(r); !ok {
		errorJSON(w, http.StatusUnauthorized, "Unauthorized")
		return
	}
	var body struct {
		Action   string   `json:"action"`
		PhotoID  string   `json:"photoId"`
		PhotoIDs []string `json:"photoIds"`
	}
	if decodeJSON(r, &body) != nil {
		errorJSON(w, http.StatusBadRequest, "Invalid request")
		return
	}

	ids := body.PhotoIDs
	if body.Action == "single-reindex" {
		ids = []string{body.PhotoID}
	} else if body.Action != "batch-reindex" {
		errorJSON(w, http.StatusBadRequest, "Invalid reindex action")
		return
	}
	if len(ids) == 0 {
		rows, err := a.db.Query(`SELECT id FROM photos WHERE storage_key IS NOT NULL`)
		if err != nil {
			errorJSON(w, http.StatusInternalServerError, "Unable to list photos")
			return
		}
		for rows.Next() {
			var id string
			if rows.Scan(&id) == nil {
				ids = append(ids, id)
			}
		}
		_ = rows.Close()
	}
	if body.Action == "single-reindex" {
		if body.PhotoID == "" {
			errorJSON(w, http.StatusBadRequest, "photoId is required")
			return
		}
		if err := a.reindexOneExif(r.Context(), body.PhotoID); err != nil {
			if strings.Contains(err.Error(), "Photo not found") {
				errorJSON(w, http.StatusNotFound, "Photo not found")
			} else {
				errorJSON(w, http.StatusInternalServerError, err.Error())
			}
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"success": true,
			"message": "EXIF data reindexed successfully",
			"photoId": body.PhotoID,
		})
		return
	}

	results := make([]exifReindexResult, 0, len(ids))
	updated := 0
	for _, id := range uniqueStrings(ids) {
		if err := a.reindexOneExif(r.Context(), id); err != nil {
			results = append(results, exifReindexResult{PhotoID: id, Error: err.Error()})
			continue
		}
		updated++
		results = append(results, exifReindexResult{PhotoID: id})
	}
	processed := len(results)
	successRate := "0%"
	if processed > 0 {
		successRate = fmt.Sprintf("%.1f%%", float64(processed-len(reindexErrors(results)))/float64(processed)*100)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"message": "EXIF reindex completed",
		"results": map[string]any{
			"total": len(ids), "processed": processed, "updated": updated,
			"errors":     reindexErrors(results),
			"statistics": map[string]any{"successRate": successRate},
		},
	})
}

func (a *App) reindexOneExif(ctx context.Context, id string) error {
	var key string
	if err := a.db.QueryRow(`SELECT storage_key FROM photos WHERE id=?`, id).Scan(&key); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return errors.New("Photo not found")
		}
		return err
	}
	if strings.TrimSpace(key) == "" {
		return errors.New("Photo has no storage key")
	}
	data, err := a.readStorageBytes(ctx, key)
	if err != nil {
		return errors.New("File not found in storage")
	}
	exif, dateTaken := extractExifContext(ctx, a.cfg.ExifTool, data, filepath.Ext(key))
	title := strings.TrimSuffix(filepath.Base(key), filepath.Ext(key))
	tags := exifTags(exif)
	thumbKey := storageKey(a.storage.Prefix(), "thumbnails/"+id+".webp")
	_, err = a.db.Exec(`UPDATE photos SET exif=?,title=?,tags=?,date_taken=?,last_modified=?,thumbnail_key=? WHERE id=?`, jsonValue(exif), title, jsonValue(tags), nullableString(dateTaken), time.Now().UTC().Format(time.RFC3339), thumbKey, id)
	return err
}

func exifTags(exif map[string]any) []string {
	for _, key := range []string{"Subject", "Keywords", "XPKeywords"} {
		value, ok := exif[key]
		if !ok || value == nil {
			continue
		}
		switch v := value.(type) {
		case []any:
			result := make([]string, 0, len(v))
			for _, item := range v {
				if text := strings.TrimSpace(fmt.Sprint(item)); text != "" {
					result = append(result, text)
				}
			}
			return normalizeTags(result)
		case string:
			parts := strings.FieldsFunc(v, func(r rune) bool { return r == ';' || r == ',' })
			return normalizeTags(parts)
		}
	}
	return []string{}
}

func nullableString(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func reindexErrors(results []exifReindexResult) []exifReindexResult {
	errors := []exifReindexResult{}
	for _, result := range results {
		if result.Error != "" {
			errors = append(errors, result)
		}
	}
	return errors
}
