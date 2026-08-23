package main

import (
	"context"
	"database/sql"
	"errors"
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
		defer rows.Close()
		for rows.Next() {
			var id string
			if rows.Scan(&id) == nil {
				ids = append(ids, id)
			}
		}
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
	writeJSON(w, http.StatusOK, map[string]any{
		"success": true,
		"message": "EXIF reindex completed",
		"photoId": body.PhotoID,
		"results": map[string]any{
			"total": len(ids), "processed": len(results), "updated": updated, "errors": reindexErrors(results),
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
	data, err := a.storage.Get(ctx, key)
	if err != nil {
		return errors.New("File not found in storage")
	}
	exif, dateTaken := extractExif(a.cfg.ExifTool, data, filepath.Ext(key))
	title := strings.TrimSuffix(filepath.Base(key), filepath.Ext(key))
	_, err = a.db.Exec(`UPDATE photos SET exif=?,title=?,date_taken=?,last_modified=? WHERE id=?`, jsonValue(exif), title, nullableString(dateTaken), time.Now().UTC().Format(time.RFC3339), id)
	return err
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
