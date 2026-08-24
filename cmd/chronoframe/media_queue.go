package main

import (
	"bytes"
	"context"
	"crypto/md5"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"mime"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

func (a *App) startWorkers() {
	// Tasks left in an in-progress state after a process crash must be retried.
	_, _ = a.db.Exec(`UPDATE pipeline_queue SET status='pending',status_stage=NULL WHERE status='in-stages'`)
	for i := 0; i < a.cfg.WorkerCount; i++ {
		a.wg.Add(1)
		go a.worker(i + 1)
	}
}
func (a *App) stopWorkers() { close(a.stop); a.wg.Wait() }
func (a *App) worker(id int) {
	defer a.wg.Done()
	for {
		select {
		case <-a.stop:
			return
		default:
		}
		task, err := a.claimTask()
		if err != nil {
			a.logs.Add("queue", fmt.Sprintf("worker %d: %v", id, err))
			time.Sleep(time.Second)
			continue
		}
		if task == nil {
			select {
			case <-a.stop:
				return
			case <-a.queueWake:
			case <-time.After(2 * time.Second):
			}
			continue
		}
		if err := a.processTask(task); err != nil {
			a.failTask(task.ID, err.Error())
		} else {
			a.completeTask(task.ID)
		}
	}
}

type Task struct {
	ID                    int64
	Payload               map[string]any
	Attempts, MaxAttempts int
	Status                string
	Stage                 sql.NullString
}

func (a *App) claimTask() (*Task, error) {
	// Only one in-process worker should try to claim at a time. This keeps
	// SQLite's single-writer path short while workers still process media in
	// parallel after the claim commits.
	a.queueClaimMu.Lock()
	defer a.queueClaimMu.Unlock()
	tx, err := a.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var t Task
	var payload string
	err = tx.QueryRow(`SELECT id,payload,attempts,max_attempts,status,status_stage FROM pipeline_queue WHERE status='pending' AND (created_at <= unixepoch()) ORDER BY priority DESC, created_at ASC, id ASC LIMIT 1`).Scan(&t.ID, &payload, &t.Attempts, &t.MaxAttempts, &t.Status, &t.Stage)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if err = json.Unmarshal([]byte(payload), &t.Payload); err != nil {
		message := "invalid task payload: " + err.Error()
		if _, updateErr := tx.Exec(`UPDATE pipeline_queue SET status='failed', attempts=max_attempts, error_message=? WHERE id=?`, message, t.ID); updateErr != nil {
			return nil, updateErr
		}
		if commitErr := tx.Commit(); commitErr != nil {
			return nil, commitErr
		}
		a.logs.Add("queue", fmt.Sprintf("task %d moved to failed: %s", t.ID, message))
		return nil, nil
	}
	result, err := tx.Exec(`UPDATE pipeline_queue SET status='in-stages' WHERE id=? AND status='pending'`, t.ID)
	if err != nil {
		return nil, err
	}
	changed, _ := result.RowsAffected()
	if changed != 1 {
		return nil, nil
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &t, nil
}
func (a *App) completeTask(id int64) {
	_, _ = a.db.Exec(`UPDATE pipeline_queue SET status='completed', completed_at=unixepoch() WHERE id=?`, id)
}
func (a *App) failTask(id int64, message string) {
	var attempts, max int
	if a.db.QueryRow(`SELECT attempts,max_attempts FROM pipeline_queue WHERE id=?`, id).Scan(&attempts, &max) != nil {
		return
	}
	attempts++
	if attempts < max {
		_, _ = a.db.Exec(`UPDATE pipeline_queue SET status='pending', attempts=?, error_message=?, created_at=unixepoch()+? WHERE id=?`, attempts, message, min(30, 1<<min(attempts-1, 5)), id)
	} else {
		_, _ = a.db.Exec(`UPDATE pipeline_queue SET status='failed', attempts=?, error_message=? WHERE id=?`, attempts, message, id)
	}
}
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
func (a *App) wakeQueue() {
	select {
	case a.queueWake <- struct{}{}:
	default:
	}
}

func (a *App) processTask(t *Task) error {
	typeName, _ := t.Payload["type"].(string)
	switch typeName {
	case "photo":
		return a.processPhoto(t)
	case "live-photo-video":
		return a.processLivePhoto(t)
	case "photo-reverse-geocoding":
		return a.processReverseGeocoding(t)
	case "photo-erase-location":
		return a.eraseLocation(t)
	default:
		return fmt.Errorf("unknown task type: %s", typeName)
	}
}

func safePhotoID(key string) string {
	name := filepath.Base(key)
	name = strings.TrimSuffix(name, filepath.Ext(name))
	original := name
	name = regexp.MustCompile(`[^\w\-_.]+`).ReplaceAllString(name, "_")
	name = regexp.MustCompile(`_{2,}`).ReplaceAllString(name, "_")
	name = strings.Trim(name, "_")
	if len(name) < 3 {
		h := md5.Sum([]byte(original))
		return "photo_" + hex.EncodeToString(h[:])[:8]
	}
	if len(name) > 32 {
		h := md5.Sum([]byte(original))
		return name[:23] + "_" + hex.EncodeToString(h[:])[:8]
	}
	return name
}
func storageKey(prefix, key string) string {
	prefix = strings.Trim(prefix, "/")
	key = strings.TrimLeft(strings.ReplaceAll(key, "\\", "/"), "/")
	if prefix == "" || strings.HasPrefix(key, prefix+"/") || key == prefix {
		return key
	}
	return prefix + "/" + key
}
func jsonValue(value any) string { b, _ := json.Marshal(value); return string(b) }

func (a *App) processPhoto(t *Task) error {
	key, _ := t.Payload["storageKey"].(string)
	if key == "" {
		return errors.New("missing storageKey")
	}
	id := safePhotoID(key)
	ctx := context.Background()
	a.setStage(t.ID, "preprocessing")
	raw, err := a.storage.Get(ctx, key)
	if err != nil {
		return err
	}
	if len(raw) == 0 {
		return errors.New("empty photo")
	}
	erase := false
	if value, explicit := t.Payload["eraseLocation"].(bool); explicit {
		erase = value
	} else {
		var setting any
		if a.readSetting("privacy", "upload.autoEraseLocation", &setting) {
			erase, _ = setting.(bool)
		}
	}
	if erase {
		updated, rewriteErr := a.rewritePhotoMetadata(ctx, key, raw, locationExifUpdates(nil))
		if rewriteErr != nil {
			return rewriteErr
		}
		if _, rewriteErr = a.storage.Create(ctx, key, updated, mime.TypeByExtension(filepath.Ext(key))); rewriteErr != nil {
			return rewriteErr
		}
		raw = updated
	}
	processed, err := a.preprocessPhoto(ctx, key, raw)
	if err != nil {
		return err
	}
	a.setStage(t.ID, "metadata")
	a.setStage(t.ID, "thumbnail")
	thumbnail, err := ffmpegThumbnail(a.cfg.FFmpeg, processed.image)
	if err != nil {
		return fmt.Errorf("thumbnail: %w", err)
	}
	thumbKey := storageKey(a.storage.Prefix(), "thumbnails/"+id+".webp")
	if _, err := a.storage.Create(ctx, thumbKey, thumbnail, "image/webp"); err != nil {
		return err
	}
	thumbnailHash := thumbnailPlaceholder(thumbnail)
	a.setStage(t.ID, "exif")
	exif, dateTaken := extractExif(a.cfg.ExifTool, raw, filepath.Ext(key))
	title, description, tags := photoInfoFromExif(key, exif)
	if erase {
		exif = stripGPS(exif)
	}
	latitude, longitude, hasGPS := gpsCoordinates(exif)
	if erase {
		hasGPS = false
	}
	a.setStage(t.ID, "motion-photo")
	motionVideoKey, motionErr := a.extractMotionPhoto(ctx, id, key, raw, exif)
	if motionErr != nil {
		a.logs.Add("queue", motionError(key, motionErr).Error())
	}
	var liveVideoURL any
	var isLivePhoto int
	if motionVideoKey == "" {
		motionVideoKey = a.pairedLiveVideo(ctx, key)
	}
	if motionVideoKey != "" {
		isLivePhoto = 1
		liveVideoURL = publicMediaURL(a.storage, motionVideoKey)
	}
	originalKey := key
	if processed.jpegKey != "" {
		originalKey = processed.jpegKey
	}
	original := publicMediaURL(a.storage, originalKey)
	thumbURL := publicMediaURL(a.storage, thumbKey)
	last := processed.object.ModTime.UTC().Format(time.RFC3339)
	if processed.object.ModTime.IsZero() {
		last = time.Now().UTC().Format(time.RFC3339)
	}
	var latValue, lonValue any
	if hasGPS {
		latValue, lonValue = latitude, longitude
	}
	var fileSize any = processed.object.Size
	if processed.object.Size <= 0 {
		fileSize = len(raw)
	}
	_, err = a.db.Exec(`INSERT INTO photos (id,title,description,width,height,aspect_ratio,date_taken,storage_key,thumbnail_key,file_size,last_modified,original_url,thumbnail_url,thumbnail_hash,tags,exif,latitude,longitude,country,city,location_name,is_live_photo,live_photo_video_url,live_photo_video_key) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?) ON CONFLICT(id) DO UPDATE SET title=excluded.title,description=excluded.description,width=excluded.width,height=excluded.height,aspect_ratio=excluded.aspect_ratio,date_taken=excluded.date_taken,storage_key=excluded.storage_key,thumbnail_key=excluded.thumbnail_key,file_size=excluded.file_size,last_modified=excluded.last_modified,original_url=excluded.original_url,thumbnail_url=excluded.thumbnail_url,thumbnail_hash=excluded.thumbnail_hash,tags=excluded.tags,exif=excluded.exif,latitude=excluded.latitude,longitude=excluded.longitude,is_live_photo=CASE WHEN excluded.is_live_photo=1 THEN 1 ELSE photos.is_live_photo END,live_photo_video_url=CASE WHEN excluded.live_photo_video_key IS NOT NULL THEN excluded.live_photo_video_url ELSE photos.live_photo_video_url END,live_photo_video_key=CASE WHEN excluded.live_photo_video_key IS NOT NULL THEN excluded.live_photo_video_key ELSE photos.live_photo_video_key END`, id, title, description, processed.width, processed.height, float64(processed.width)/float64(processed.height), nullableString(dateTaken), key, thumbKey, fileSize, last, original, thumbURL, thumbnailHash, jsonValue(tags), jsonValue(exif), latValue, lonValue, nil, nil, nil, isLivePhoto, liveVideoURL, nullableString(motionVideoKey))
	if err != nil {
		return err
	}
	if hasGPS {
		a.setStage(t.ID, "reverse-geocoding")
		if err := a.enqueueReverseGeocoding(id, latitude, longitude); err != nil {
			a.logs.Add("queue", fmt.Sprintf("failed to enqueue reverse geocoding %s: %v", id, err))
		}
	}
	a.logs.Add("queue", "processed photo "+id)
	return nil
}
func (a *App) processLivePhoto(t *Task) error {
	key, _ := t.Payload["storageKey"].(string)
	if key == "" {
		return errors.New("missing storageKey")
	}
	base := strings.TrimSuffix(key, filepath.Ext(key))
	imageKey := ""
	for _, ext := range []string{".jpg", ".JPG", ".jpeg", ".JPEG", ".heic", ".HEIC", ".heif", ".HEIF"} {
		candidate := base + ext
		var found string
		if a.db.QueryRow(`SELECT storage_key FROM photos WHERE storage_key=? OR id=? LIMIT 1`, candidate, safePhotoID(candidate)).Scan(&found) == nil {
			imageKey = found
			break
		}
	}
	if imageKey == "" {
		return errors.New("paired photo not found")
	}
	var id string
	if err := a.db.QueryRow(`SELECT id FROM photos WHERE storage_key=? OR id=? LIMIT 1`, imageKey, safePhotoID(imageKey)).Scan(&id); err != nil {
		return err
	}
	url := a.storage.PublicURL(key)
	if url == "" {
		url = "/image/" + key
	}
	_, err := a.db.Exec(`UPDATE photos SET is_live_photo=1,live_photo_video_url=?,live_photo_video_key=? WHERE id=?`, url, key, id)
	return err
}
func (a *App) processReverseGeocoding(t *Task) error {
	photoID, _ := t.Payload["photoId"].(string)
	latitude, lok := numberValue(t.Payload["latitude"])
	longitude, ook := numberValue(t.Payload["longitude"])
	if photoID == "" || !lok || !ook {
		return errors.New("invalid reverse geocoding task")
	}
	return a.reverseGeocode(context.Background(), photoID, latitude, longitude)
}
func numberValue(value any) (float64, bool) {
	switch v := value.(type) {
	case float64:
		return v, true
	case int:
		return float64(v), true
	case int64:
		return float64(v), true
	default:
		return 0, false
	}
}
func (a *App) eraseLocation(t *Task) error {
	id, _ := t.Payload["photoId"].(string)
	var key string
	if err := a.db.QueryRow(`SELECT storage_key FROM photos WHERE id=?`, id).Scan(&key); err != nil {
		return err
	}
	data, err := a.storage.Get(context.Background(), key)
	if err != nil {
		return err
	}
	return a.erasePhotoLocation(context.Background(), id, key, data)
}
func nilIf(err error) error { return err }
func (a *App) setStage(id int64, stage string) {
	_, _ = a.db.Exec(`UPDATE pipeline_queue SET status_stage=? WHERE id=?`, stage, id)
}

func (a *App) enqueueReverseGeocoding(photoID string, latitude, longitude float64) error {
	payload, err := json.Marshal(map[string]any{
		"type":      "photo-reverse-geocoding",
		"photoId":   photoID,
		"latitude":  latitude,
		"longitude": longitude,
	})
	if err != nil {
		return err
	}
	if _, err := a.db.Exec(`INSERT INTO pipeline_queue(payload,priority,max_attempts,status) VALUES(?,?,?,'pending')`, string(payload), 1, 3); err != nil {
		return err
	}
	a.wakeQueue()
	return nil
}

func imageSize(data []byte) (int, int) {
	config, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return 0, 0
	}
	return config.Width, config.Height
}

func probeSize(ffprobe string, data []byte) (int, int) {
	cmd := exec.Command(ffprobe, "-v", "error", "-select_streams", "v:0", "-show_entries", "stream=width,height", "-of", "json", "pipe:0")
	cmd.Stdin = bytes.NewReader(data)
	out, err := cmd.Output()
	if err != nil {
		return 0, 0
	}
	var result struct {
		Streams []struct {
			Width  int `json:"width"`
			Height int `json:"height"`
		} `json:"streams"`
	}
	if json.Unmarshal(out, &result) != nil || len(result.Streams) == 0 {
		return 0, 0
	}
	return result.Streams[0].Width, result.Streams[0].Height
}
func ffmpegThumbnail(ffmpeg string, data []byte) ([]byte, error) {
	cmd := exec.Command(ffmpeg, "-hide_banner", "-loglevel", "error", "-i", "pipe:0", "-frames:v", "1", "-vf", "scale='min(600,iw)':-2", "-c:v", "libwebp", "-quality", "85", "-f", "webp", "pipe:1")
	cmd.Stdin = bytes.NewReader(data)
	return cmd.Output()
}

func extractExif(tool string, data []byte, ext string) (map[string]any, string) {
	result := map[string]any{}
	dir, err := os.MkdirTemp("", "cframe-exif-")
	if err != nil {
		return result, ""
	}
	defer os.RemoveAll(dir)
	file := filepath.Join(dir, "input"+ext)
	if os.WriteFile(file, data, 0600) != nil {
		return result, ""
	}
	out, err := exec.Command(tool, "-j", "-n", "-s", file).Output()
	if err == nil {
		var rows []map[string]any
		if json.Unmarshal(out, &rows) == nil && len(rows) > 0 {
			result = rows[0]
		}
	}
	date := ""
	for _, key := range []string{"DateTimeOriginal", "CreateDate", "DateTimeDigitized"} {
		if value, ok := result[key]; ok {
			date = normalizeDate(fmt.Sprint(value))
			if date != "" {
				break
			}
		}
	}
	return result, date
}
func normalizeDate(value string) string {
	value = strings.TrimSpace(strings.ReplaceAll(value, " ", "T"))
	if t, err := time.Parse(time.RFC3339, value); err == nil {
		return t.UTC().Format(time.RFC3339)
	}
	if t, err := time.Parse("2006:01:02T15:04:05", value); err == nil {
		return t.UTC().Format(time.RFC3339)
	}
	return value
}
func stripGPS(m map[string]any) map[string]any {
	for _, key := range []string{"GPSAltitude", "GPSCoordinates", "GPSAltitudeRef", "GPSLatitude", "GPSLatitudeRef", "GPSLongitude", "GPSLongitudeRef", "GPSPosition", "GPSDateStamp", "GPSTimeStamp"} {
		delete(m, key)
	}
	return m
}

// Storage implementations
