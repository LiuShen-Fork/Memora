package main

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"mime"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

func (a *App) startWorkers() {
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
		return nil, err
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
		return nil
	case "photo-erase-location":
		return a.eraseLocation(t)
	default:
		return fmt.Errorf("unknown task type: %s", typeName)
	}
}

func safePhotoID(key string) string {
	name := filepath.Base(key)
	name = strings.TrimSuffix(name, filepath.Ext(name))
	name = regexp.MustCompile(`[^A-Za-z0-9_.-]+`).ReplaceAllString(name, "_")
	name = strings.Trim(name, "_")
	if len(name) < 3 {
		h := sha256.Sum256([]byte(key))
		return "photo_" + hex.EncodeToString(h[:])[:8]
	}
	if len(name) > 32 {
		h := sha256.Sum256([]byte(key))
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
	a.setStage(t.ID, "preprocessing")
	data, err := a.storage.Get(context.Background(), key)
	if err != nil {
		return err
	}
	if len(data) == 0 {
		return errors.New("empty photo")
	}
	a.setStage(t.ID, "metadata")
	width, height := imageSize(data)
	if width == 0 || height == 0 {
		width, height = probeSize(a.cfg.FFmpeg, data)
	}
	if width == 0 || height == 0 {
		return errors.New("unable to read image dimensions")
	}
	a.setStage(t.ID, "thumbnail")
	thumbnail, err := ffmpegThumbnail(a.cfg.FFmpeg, data)
	if err != nil {
		return fmt.Errorf("thumbnail: %w", err)
	}
	thumbKey := storageKey(a.storage.Prefix(), "thumbnails/"+id+".webp")
	if _, err := a.storage.Create(context.Background(), thumbKey, thumbnail, "image/webp"); err != nil {
		return err
	}
	a.setStage(t.ID, "exif")
	exif, dateTaken := extractExif(a.cfg.ExifTool, data, filepath.Ext(key))
	last := time.Now().UTC().Format(time.RFC3339)
	original := a.storage.PublicURL(key)
	thumbURL := a.storage.PublicURL(thumbKey)
	erase, _ := t.Payload["eraseLocation"].(bool)
	if erase {
		exif = stripGPS(exif)
	}
	_, err = a.db.Exec(`INSERT INTO photos (id,title,description,width,height,aspect_ratio,date_taken,storage_key,thumbnail_key,file_size,last_modified,original_url,thumbnail_url,thumbnail_hash,tags,exif,latitude,longitude,country,city,location_name,is_live_photo,live_photo_video_url,live_photo_video_key) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?) ON CONFLICT(id) DO UPDATE SET title=excluded.title,width=excluded.width,height=excluded.height,aspect_ratio=excluded.aspect_ratio,date_taken=excluded.date_taken,storage_key=excluded.storage_key,thumbnail_key=excluded.thumbnail_key,file_size=excluded.file_size,last_modified=excluded.last_modified,original_url=excluded.original_url,thumbnail_url=excluded.thumbnail_url,exif=excluded.exif`, id, strings.TrimSuffix(filepath.Base(key), filepath.Ext(key)), nil, width, height, float64(width)/float64(height), dateTaken, key, thumbKey, len(data), last, original, thumbURL, nil, "[]", jsonValue(exif), nil, nil, nil, nil, nil, 0, nil, nil)
	if err != nil {
		return err
	}
	if erase {
		_, _ = a.db.Exec(`UPDATE photos SET latitude=NULL,longitude=NULL,country=NULL,city=NULL,location_name=NULL WHERE id=?`, id)
	}
	a.logs.Add("queue", "processed photo "+id)
	return nil
}
func (a *App) processLivePhoto(t *Task) error { return nil }
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
	_, err = a.storage.Create(context.Background(), key, data, mime.TypeByExtension(filepath.Ext(key)))
	return err
}
func nilIf(err error) error { return err }
func (a *App) setStage(id int64, stage string) {
	_, _ = a.db.Exec(`UPDATE pipeline_queue SET status_stage=? WHERE id=?`, stage, id)
}

func imageSize(data []byte) (int, int) {
	config, _, err := image.DecodeConfig(strings.NewReader(string(data)))
	if err != nil {
		config, _, err = image.DecodeConfig(bytesReader(data))
	}
	if err != nil {
		return 0, 0
	}
	return config.Width, config.Height
}

type byteReader struct {
	data []byte
	pos  int
}

func (r *byteReader) Read(p []byte) (int, error) {
	if r.pos >= len(r.data) {
		return 0, io.EOF
	}
	n := copy(p, r.data[r.pos:])
	r.pos += n
	return n, nil
}
func bytesReader(b []byte) io.Reader { return &byteReader{data: b} }
func probeSize(ffmpeg string, data []byte) (int, int) {
	cmd := exec.Command(ffmpeg, "-hide_banner", "-loglevel", "error", "-i", "pipe:0", "-f", "null", "-")
	cmd.Stdin = bytesReader(data)
	_ = cmd.Run()
	return 0, 0
}
func ffmpegThumbnail(ffmpeg string, data []byte) ([]byte, error) {
	cmd := exec.Command(ffmpeg, "-hide_banner", "-loglevel", "error", "-i", "pipe:0", "-frames:v", "1", "-vf", "scale='min(600,iw)':-2", "-c:v", "libwebp", "-quality", "85", "-f", "webp", "pipe:1")
	cmd.Stdin = bytesReader(data)
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
	out, err := exec.Command(tool, "-j", "-n", "-G", "-s", file).Output()
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
