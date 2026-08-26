package main

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// cleanupInvalidGeneratedVideos removes malformed videos left by previous
// Motion Photo scans. Generated videos use the deterministic <photoID>.mp4
// name; unrelated media is intentionally outside this cleanup path.
func (a *App) cleanupInvalidGeneratedVideos() {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()

	rows, err := a.db.QueryContext(ctx, `SELECT id,COALESCE(live_photo_video_key,''),is_live_photo FROM photos`)
	if err != nil {
		a.logs.Add("queue", fmt.Sprintf("live video cleanup query failed: %v", err))
		return
	}
	defer rows.Close()

	removed := 0
	for rows.Next() {
		var id, videoKey string
		var isLive int
		if err := rows.Scan(&id, &videoKey, &isLive); err != nil {
			continue
		}

		// Check the database reference first. This also handles legacy records
		// whose generated file was stored with a provider prefix.
		if videoKey != "" && !a.validStoredVideo(ctx, a.relativeStorageKey(videoKey)) {
			if a.deleteStorageObject(ctx, videoKey) {
				removed++
			}
			_, _ = a.db.ExecContext(ctx, `UPDATE photos SET is_live_photo=0,live_photo_video_key=NULL,live_photo_video_url=NULL WHERE id=?`, id)
			videoKey = ""
		}

		// Old extraction wrote <photoID>.mp4 even when the source was not a
		// Motion Photo. Remove only malformed files with that exact generated
		// name; valid files are left for normal pairing/detection.
		generatedKey := id + ".mp4"
		if _, err := a.storage.Meta(ctx, generatedKey); err == nil && !a.validStoredVideo(ctx, generatedKey) {
			if a.deleteStorageObject(ctx, generatedKey) {
				removed++
			}
			if videoKey == generatedKey || a.relativeStorageKey(videoKey) == generatedKey {
				_, _ = a.db.ExecContext(ctx, `UPDATE photos SET is_live_photo=0,live_photo_video_key=NULL,live_photo_video_url=NULL WHERE id=?`, id)
			}
		}
	}
	if err := rows.Err(); err != nil {
		a.logs.Add("queue", fmt.Sprintf("live video cleanup iteration failed: %v", err))
	}
	if removed > 0 {
		a.logs.Add("queue", fmt.Sprintf("removed %d invalid generated Live Photo videos", removed))
	}
}

func (a *App) relativeStorageKey(key string) string {
	prefix := strings.Trim(a.storage.Prefix(), "/")
	key = strings.TrimLeft(strings.ReplaceAll(key, "\\", "/"), "/")
	if prefix != "" {
		key = strings.TrimPrefix(key, prefix+"/")
	}
	return key
}

func (a *App) deleteStorageObject(ctx context.Context, key string) bool {
	if err := a.storage.Delete(ctx, a.relativeStorageKey(key)); err != nil {
		a.logs.Add("queue", fmt.Sprintf("failed to remove invalid Live Photo video %s: %v", key, err))
		return false
	}
	return true
}
