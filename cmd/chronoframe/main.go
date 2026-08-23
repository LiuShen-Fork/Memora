package main

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"log"
	"mime"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"golang.org/x/crypto/bcrypt"
	"golang.org/x/crypto/scrypt"
	_ "modernc.org/sqlite"
)

type App struct {
	cfg       Config
	db        *sql.DB
	storage   Storage
	queueWake chan struct{}
	stop      chan struct{}
	wg        sync.WaitGroup
	logs      *LogBuffer
}

func main() {
	cfg := loadConfig()
	if err := os.MkdirAll(filepath.Dir(cfg.DBPath), 0o755); err != nil {
		log.Fatal(err)
	}
	if err := os.MkdirAll(cfg.DataDir, 0o755); err != nil {
		log.Fatal(err)
	}
	db, err := sql.Open("sqlite", cfg.DBPath+"?_pragma=busy_timeout(10000)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)&_pragma=foreign_keys(ON)")
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	app := &App{cfg: cfg, db: db, queueWake: make(chan struct{}, 1), stop: make(chan struct{}), logs: NewLogBuffer()}
	if err := app.ensureSchema(); err != nil {
		log.Fatal(err)
	}
	app.storage = app.loadStorage()
	app.startWorkers()
	defer app.stopWorkers()

	server := &http.Server{Addr: cfg.Addr, Handler: app}
	app.logs.Add("server", "ChronoFrame Go backend listening on "+cfg.Addr)
	log.Printf("ChronoFrame Go backend listening on %s", cfg.Addr)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}

func (a *App) ensureSchema() error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS photos (id TEXT PRIMARY KEY, title TEXT, description TEXT, width INTEGER, height INTEGER, aspect_ratio REAL, date_taken TEXT, storage_key TEXT, file_size INTEGER, last_modified TEXT, original_url TEXT, thumbnail_url TEXT, thumbnail_hash TEXT, tags TEXT, exif TEXT, latitude REAL, longitude REAL, country TEXT, city TEXT, location_name TEXT, is_live_photo INTEGER NOT NULL DEFAULT 0, live_photo_video_url TEXT, live_photo_video_key TEXT, thumbnail_key TEXT)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS photos_id_unique ON photos(id)`,
		`CREATE TABLE IF NOT EXISTS users (id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT NOT NULL UNIQUE, email TEXT NOT NULL UNIQUE, password TEXT, avatar TEXT, created_at INTEGER NOT NULL, is_admin INTEGER NOT NULL DEFAULT 0)`,
		`CREATE TABLE IF NOT EXISTS pipeline_queue (id INTEGER PRIMARY KEY AUTOINCREMENT, payload TEXT NOT NULL, priority INTEGER NOT NULL DEFAULT 0, attempts INTEGER NOT NULL DEFAULT 0, max_attempts INTEGER NOT NULL DEFAULT 3, status TEXT NOT NULL DEFAULT 'pending', status_stage TEXT, error_message TEXT, created_at INTEGER NOT NULL DEFAULT (unixepoch()), completed_at INTEGER)`,
		`CREATE TABLE IF NOT EXISTS photo_reactions (id INTEGER PRIMARY KEY AUTOINCREMENT, photo_id TEXT NOT NULL REFERENCES photos(id) ON DELETE CASCADE, reaction_type TEXT NOT NULL, fingerprint TEXT NOT NULL, ip_address TEXT, user_agent TEXT, created_at INTEGER NOT NULL DEFAULT (unixepoch()), updated_at INTEGER NOT NULL DEFAULT (unixepoch()))`,
		`CREATE INDEX IF NOT EXISTS photo_reactions_photo_idx ON photo_reactions(photo_id)`,
		`CREATE TABLE IF NOT EXISTS albums (id INTEGER PRIMARY KEY AUTOINCREMENT, title TEXT NOT NULL, description TEXT, cover_photo_id TEXT REFERENCES photos(id) ON DELETE SET NULL, is_hidden INTEGER NOT NULL DEFAULT 0, created_at INTEGER NOT NULL DEFAULT (unixepoch()), updated_at INTEGER NOT NULL DEFAULT (unixepoch()))`,
		`CREATE TABLE IF NOT EXISTS album_photos (id INTEGER PRIMARY KEY AUTOINCREMENT, album_id INTEGER NOT NULL REFERENCES albums(id) ON DELETE CASCADE, photo_id TEXT NOT NULL REFERENCES photos(id) ON DELETE CASCADE, position REAL NOT NULL DEFAULT 1000000, added_at INTEGER NOT NULL DEFAULT (unixepoch()))`,
		`CREATE INDEX IF NOT EXISTS album_photos_album_idx ON album_photos(album_id, position)`,
		`CREATE TABLE IF NOT EXISTS settings (id INTEGER PRIMARY KEY AUTOINCREMENT, namespace TEXT NOT NULL DEFAULT 'common', key TEXT NOT NULL, type TEXT NOT NULL, value TEXT, default_value TEXT, label TEXT, description TEXT, is_public INTEGER NOT NULL DEFAULT 0, is_readonly INTEGER NOT NULL DEFAULT 0, is_secret INTEGER NOT NULL DEFAULT 0, enum TEXT, updated_at INTEGER NOT NULL DEFAULT (unixepoch()), updated_by INTEGER REFERENCES users(id) ON DELETE SET NULL)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_namespace_key ON settings(namespace, key)`,
		`CREATE TABLE IF NOT EXISTS settings_storage_providers (id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT NOT NULL, provider TEXT NOT NULL, config TEXT NOT NULL, created_at INTEGER NOT NULL DEFAULT (unixepoch()), updated_at INTEGER NOT NULL DEFAULT (unixepoch()))`,
	}
	for _, statement := range statements {
		if _, err := a.db.Exec(statement); err != nil {
			return fmt.Errorf("schema: %w", err)
		}
	}
	return nil
}

func (a *App) loadStorage() Storage {
	provider := strings.ToLower(a.cfg.Provider)
	if row := a.db.QueryRow(`SELECT value FROM settings WHERE namespace='storage' AND key='provider'`); row != nil {
		var value sql.NullString
		if row.Scan(&value) == nil && value.Valid && value.String != "" {
			provider = strings.Trim(value.String, `"`)
			if id, err := strconv.ParseInt(provider, 10, 64); err == nil {
				var configJSON, providerName string
				if a.db.QueryRow(`SELECT provider,config FROM settings_storage_providers WHERE id=?`, id).Scan(&providerName, &configJSON) == nil {
					provider = providerName
					var config map[string]any
					if json.Unmarshal([]byte(configJSON), &config) == nil {
						a.applyProviderConfig(provider, config)
					}
				}
			}
		}
	}
	if provider == "local" || provider == "" {
		return &LocalStorage{base: a.cfg.LocalPath, baseURL: a.cfg.LocalBaseURL, prefix: a.cfg.LocalPrefix}
	}
	if provider == "s3" {
		if s, err := NewS3Storage(a.cfg); err == nil {
			return s
		}
		log.Printf("S3 provider unavailable, falling back to local storage")
		return &LocalStorage{base: a.cfg.LocalPath, baseURL: a.cfg.LocalBaseURL, prefix: a.cfg.LocalPrefix}
	}
	if provider == "openlist" {
		return &OpenListStorage{baseURL: a.cfg.OpenBaseURL, root: a.cfg.OpenRootPath, token: a.cfg.OpenToken, upload: a.cfg.OpenUpload, download: a.cfg.OpenDownload, list: a.cfg.OpenList, delete: a.cfg.OpenDelete, meta: a.cfg.OpenMeta, pathField: a.cfg.OpenPathField, cdn: a.cfg.OpenCDN}
	}
	return &LocalStorage{base: a.cfg.LocalPath, baseURL: a.cfg.LocalBaseURL, prefix: a.cfg.LocalPrefix}
}
func (a *App) applyProviderConfig(provider string, config map[string]any) {
	str := func(key, fallback string) string {
		if v, ok := config[key].(string); ok && v != "" {
			return v
		}
		return fallback
	}
	if provider == "local" {
		a.cfg.LocalPath = str("basePath", a.cfg.LocalPath)
		a.cfg.LocalBaseURL = str("baseUrl", a.cfg.LocalBaseURL)
		a.cfg.LocalPrefix = str("prefix", a.cfg.LocalPrefix)
	}
	if provider == "s3" {
		a.cfg.S3Endpoint = str("endpoint", a.cfg.S3Endpoint)
		a.cfg.S3Bucket = str("bucket", a.cfg.S3Bucket)
		a.cfg.S3Region = str("region", a.cfg.S3Region)
		a.cfg.S3AccessKey = str("accessKeyId", a.cfg.S3AccessKey)
		a.cfg.S3SecretKey = str("secretAccessKey", a.cfg.S3SecretKey)
		a.cfg.S3Prefix = str("prefix", a.cfg.S3Prefix)
		a.cfg.S3CDN = str("cdnUrl", a.cfg.S3CDN)
	}
	if provider == "openlist" {
		a.cfg.OpenBaseURL = str("baseUrl", a.cfg.OpenBaseURL)
		a.cfg.OpenRootPath = str("rootPath", a.cfg.OpenRootPath)
		a.cfg.OpenToken = str("token", a.cfg.OpenToken)
		a.cfg.OpenUpload = str("uploadEndpoint", a.cfg.OpenUpload)
		a.cfg.OpenDownload = str("downloadEndpoint", a.cfg.OpenDownload)
		a.cfg.OpenList = str("listEndpoint", a.cfg.OpenList)
		a.cfg.OpenDelete = str("deleteEndpoint", a.cfg.OpenDelete)
		a.cfg.OpenMeta = str("metaEndpoint", a.cfg.OpenMeta)
		a.cfg.OpenPathField = str("pathField", a.cfg.OpenPathField)
		a.cfg.OpenCDN = str("cdnUrl", a.cfg.OpenCDN)
	}
}

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
type Object struct {
	Key     string
	Size    int64
	ModTime time.Time
}
type Storage interface {
	Create(context.Context, string, []byte, string) (Object, error)
	Get(context.Context, string) ([]byte, error)
	Delete(context.Context, string) error
	Meta(context.Context, string) (Object, error)
	PublicURL(string) string
	SignedURL(context.Context, string, string) (string, error)
	Prefix() string
}

type LocalStorage struct{ base, baseURL, prefix string }

func (s *LocalStorage) Prefix() string { return strings.Trim(s.prefix, "/") }
func (s *LocalStorage) path(key string) string {
	key = storageKey(s.prefix, key)
	return filepath.Join(s.base, filepath.FromSlash(key))
}
func (s *LocalStorage) Create(_ context.Context, key string, data []byte, _ string) (Object, error) {
	p := s.path(key)
	if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
		return Object{}, err
	}
	tmp := p + fmt.Sprintf(".tmp-%d", time.Now().UnixNano())
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return Object{}, err
	}
	if err := os.Rename(tmp, p); err != nil {
		return Object{}, err
	}
	st, err := os.Stat(p)
	if err != nil {
		return Object{}, err
	}
	return Object{Key: storageKey(s.prefix, key), Size: st.Size(), ModTime: st.ModTime()}, nil
}
func (s *LocalStorage) Get(_ context.Context, key string) ([]byte, error) {
	return os.ReadFile(s.path(key))
}
func (s *LocalStorage) Delete(_ context.Context, key string) error {
	err := os.Remove(s.path(key))
	if os.IsNotExist(err) {
		return nil
	}
	return err
}
func (s *LocalStorage) Meta(_ context.Context, key string) (Object, error) {
	st, err := os.Stat(s.path(key))
	if err != nil {
		return Object{}, err
	}
	return Object{Key: storageKey(s.prefix, key), Size: st.Size(), ModTime: st.ModTime()}, nil
}
func (s *LocalStorage) PublicURL(key string) string {
	return strings.TrimRight(s.baseURL, "/") + "/" + storageKey(s.prefix, key)
}
func (s *LocalStorage) SignedURL(_ context.Context, key, _ string) (string, error) {
	return "/api/photos/upload?key=" + urlQueryEscape(storageKey(s.prefix, key)), nil
}
func urlQueryEscape(v string) string {
	return strings.ReplaceAll(strings.ReplaceAll(v, "%", "%25"), " ", "%20")
}

type S3Storage struct {
	client              *minio.Client
	bucket, prefix, cdn string
}

func NewS3Storage(c Config) (*S3Storage, error) {
	endpoint := strings.TrimPrefix(strings.TrimPrefix(c.S3Endpoint, "https://"), "http://")
	secure := strings.HasPrefix(c.S3Endpoint, "https://")
	client, err := minio.New(endpoint, &minio.Options{Creds: credentials.NewStaticV4(c.S3AccessKey, c.S3SecretKey, c.S3Region), Secure: secure, Region: c.S3Region})
	if err != nil {
		return nil, err
	}
	return &S3Storage{client: client, bucket: c.S3Bucket, prefix: strings.Trim(c.S3Prefix, "/"), cdn: strings.TrimRight(c.S3CDN, "/")}, nil
}
func (s *S3Storage) Prefix() string      { return s.prefix }
func (s *S3Storage) key(k string) string { return storageKey(s.prefix, k) }
func (s *S3Storage) Create(ctx context.Context, k string, d []byte, ct string) (Object, error) {
	k = s.key(k)
	_, err := s.client.PutObject(ctx, s.bucket, k, strings.NewReader(string(d)), int64(len(d)), minio.PutObjectOptions{ContentType: ct})
	return Object{Key: k, Size: int64(len(d)), ModTime: time.Now()}, err
}
func (s *S3Storage) Get(ctx context.Context, k string) ([]byte, error) {
	o, err := s.client.GetObject(ctx, s.bucket, k, minio.GetObjectOptions{})
	if err != nil {
		return nil, err
	}
	defer o.Close()
	return io.ReadAll(o)
}
func (s *S3Storage) Delete(ctx context.Context, k string) error {
	return s.client.RemoveObject(ctx, s.bucket, k, minio.RemoveObjectOptions{})
}
func (s *S3Storage) Meta(ctx context.Context, k string) (Object, error) {
	st, err := s.client.StatObject(ctx, s.bucket, k, minio.StatObjectOptions{})
	return Object{Key: k, Size: st.Size, ModTime: st.LastModified}, err
}
func (s *S3Storage) PublicURL(k string) string {
	if s.cdn != "" {
		return s.cdn + "/" + k
	}
	return ""
}
func (s *S3Storage) SignedURL(ctx context.Context, k, ct string) (string, error) {
	u, err := s.client.PresignedPutObject(ctx, s.bucket, k, time.Hour)
	return u.String(), err
}

type OpenListStorage struct{ baseURL, root, token, upload, download, list, delete, meta, pathField, cdn string }

func (s *OpenListStorage) Prefix() string { return strings.Trim(s.root, "/") }
func (s *OpenListStorage) full(k string) string {
	return "/" + strings.Trim(storageKey(s.root, k), "/")
}
func (s *OpenListStorage) request(ctx context.Context, method, path string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(s.baseURL, "/")+path, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", s.token)
	return http.DefaultClient.Do(req)
}
func (s *OpenListStorage) Create(ctx context.Context, k string, d []byte, ct string) (Object, error) {
	endpoint := s.upload
	if endpoint == "" {
		endpoint = "/api/fs/put"
	}
	req, err := http.NewRequestWithContext(ctx, "PUT", strings.TrimRight(s.baseURL, "/")+endpoint, strings.NewReader(string(d)))
	if err != nil {
		return Object{}, err
	}
	req.Header.Set("Authorization", s.token)
	req.Header.Set("File-Path", s.full(k))
	req.Header.Set("Content-Type", ct)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return Object{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return Object{}, fmt.Errorf("openlist upload: %s", resp.Status)
	}
	return Object{Key: storageKey(s.root, k), Size: int64(len(d)), ModTime: time.Now()}, nil
}
func (s *OpenListStorage) Get(ctx context.Context, k string) ([]byte, error) {
	endpoint := s.download
	if endpoint == "" {
		endpoint = "/d" + s.full(k)
	} else {
		endpoint += "?" + s.pathField + "=" + urlQueryEscape(s.full(k))
	}
	resp, err := s.request(ctx, "GET", endpoint, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("openlist get: %s", resp.Status)
	}
	return io.ReadAll(resp.Body)
}
func (s *OpenListStorage) Delete(ctx context.Context, k string) error {
	endpoint := s.delete
	if endpoint == "" {
		endpoint = "/api/fs/remove"
	}
	normalized := strings.Trim(s.full(k), "/")
	idx := strings.LastIndex(normalized, "/")
	dir, name := "/", normalized
	if idx >= 0 {
		dir, name = "/"+normalized[:idx], normalized[idx+1:]
	}
	payload, _ := json.Marshal(map[string]any{"dir": dir, "names": []string{name}})
	resp, err := s.request(ctx, "POST", endpoint, strings.NewReader(string(payload)))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("openlist delete: %s", resp.Status)
	}
	return nil
}
func (s *OpenListStorage) Meta(ctx context.Context, k string) (Object, error) {
	endpoint := s.meta
	if endpoint == "" {
		endpoint = "/api/fs/get"
	}
	payload, _ := json.Marshal(map[string]any{s.pathField: s.full(k), "password": "", "page": 1, "per_page": 0, "refresh": false})
	resp, err := s.request(ctx, "POST", endpoint, strings.NewReader(string(payload)))
	if err != nil {
		return Object{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return Object{}, fmt.Errorf("openlist meta: %s", resp.Status)
	}
	var body struct {
		Data struct {
			Size     int64  `json:"size"`
			Modified string `json:"modified"`
			RawURL   string `json:"raw_url"`
		} `json:"data"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&body)
	return Object{Key: storageKey(s.root, k), Size: body.Data.Size, ModTime: parseTime(body.Data.Modified)}, nil
}
func parseTime(value string) time.Time {
	if t, err := time.Parse(time.RFC3339, value); err == nil {
		return t
	}
	return time.Time{}
}
func (s *OpenListStorage) PublicURL(k string) string {
	if s.cdn != "" {
		return strings.TrimRight(s.cdn, "/") + "/" + storageKey(s.root, k)
	}
	return strings.TrimRight(s.baseURL, "/") + "/d" + s.full(k)
}
func (s *OpenListStorage) SignedURL(_ context.Context, _ string, _ string) (string, error) {
	return "", errors.New("openlist does not support signed uploads")
}

// HTTP and JSON
func (a *App) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "same-origin")
	if r.Method == "OPTIONS" {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if strings.HasPrefix(r.URL.Path, "/api/") {
		a.api(w, r)
		return
	}
	if strings.HasPrefix(r.URL.Path, "/storage/") {
		a.serveStorage(w, r)
		return
	}
	if strings.HasPrefix(r.URL.Path, "/image/") {
		a.serveImage(w, r)
		return
	}
	if strings.HasPrefix(r.URL.Path, "/thumb/") {
		a.serveThumb(w, r)
		return
	}
	a.serveWeb(w, r)
}
func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
func errorJSON(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]any{"statusCode": status, "statusMessage": message, "message": message})
}
func decodeJSON(r *http.Request, v any) error {
	return json.NewDecoder(io.LimitReader(r.Body, 16<<20)).Decode(v)
}
func (a *App) api(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimSuffix(r.URL.Path, "/")
	switch {
	case path == "/api/login" && r.Method == "POST":
		a.login(w, r)
	case path == "/api/logout":
		a.logout(w, r)
	case path == "/api/profile":
		a.profile(w, r)
	case path == "/api/photos" && r.Method == "GET":
		a.photos(w, r, false)
	case path == "/api/photos/visible" && r.Method == "GET":
		a.photos(w, r, true)
	case path == "/api/photos" && r.Method == "POST":
		a.prepareUpload(w, r)
	case path == "/api/photos/upload" && r.Method == "PUT":
		a.upload(w, r)
	case path == "/api/photos/check-duplicate" && r.Method == "POST":
		a.checkDuplicate(w, r)
	case path == "/api/photos/reactions" && r.Method == "GET":
		a.reactions(w, r)
	case path == "/api/photos/status" && r.Method == "GET":
		a.photoStatus(w, r)
	case strings.HasPrefix(path, "/api/photos/"):
		a.photoRoute(w, r, strings.TrimPrefix(path, "/api/photos/"))
	case path == "/api/albums" && r.Method == "GET":
		a.albums(w, r)
	case path == "/api/albums" && r.Method == "POST":
		a.createAlbum(w, r)
	case strings.HasPrefix(path, "/api/albums/"):
		a.albumRoute(w, r, strings.TrimPrefix(path, "/api/albums/"))
	case strings.HasPrefix(path, "/api/queue/"):
		a.queueRoute(w, r, strings.TrimPrefix(path, "/api/queue/"))
	case strings.HasPrefix(path, "/api/system/settings"):
		a.settingsRoute(w, r, strings.TrimPrefix(path, "/api/system/settings"))
	case path == "/api/system/stats":
		a.systemStats(w, r)
	case path == "/api/system/logs":
		a.systemLogs(w, r)
	case strings.HasPrefix(path, "/api/wizard/"):
		a.wizardRoute(w, r, strings.TrimPrefix(path, "/api/wizard/"))
	case path == "/api/auth/github":
		errorJSON(w, http.StatusNotImplemented, "GitHub OAuth must be configured in the Go backend")
	default:
		errorJSON(w, http.StatusNotFound, "Not Found")
	}
}

func (a *App) user(r *http.Request) (map[string]any, bool) {
	c, err := r.Cookie(cookieName)
	if err != nil {
		return nil, false
	}
	parts := strings.Split(c.Value, ".")
	if len(parts) != 3 {
		return nil, false
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return nil, false
	}
	mac := hmac.New(sha256.New, a.cfg.SessionKey)
	mac.Write([]byte(parts[0]))
	if subtle.ConstantTimeCompare(mac.Sum(nil), mustDecode(parts[2])) != 1 {
		return nil, false
	}
	var id int64
	var exp int64
	fmt.Sscanf(string(payload), "%d:%d", &id, &exp)
	if id == 0 || exp < time.Now().Unix() {
		return nil, false
	}
	var name, email, avatar string
	var admin int
	if a.db.QueryRow(`SELECT name,email,COALESCE(avatar,''),is_admin FROM users WHERE id=?`, id).Scan(&name, &email, &avatar, &admin) != nil {
		return nil, false
	}
	return map[string]any{"id": id, "username": name, "email": email, "avatar": avatar, "isAdmin": admin}, true
}
func mustDecode(s string) []byte                              { b, _ := base64.RawURLEncoding.DecodeString(s); return b }
func (a *App) require(r *http.Request) (map[string]any, bool) { return a.user(r) }
func (a *App) requireAdmin(w http.ResponseWriter, r *http.Request) bool {
	u, ok := a.require(r)
	if !ok {
		errorJSON(w, 401, "Unauthorized")
		return false
	}
	v, _ := u["isAdmin"].(int)
	if v != 1 {
		errorJSON(w, 403, "Forbidden")
		return false
	}
	return true
}
func (a *App) setSession(w http.ResponseWriter, id int64) {
	payload := fmt.Sprintf("%d:%d", id, time.Now().Add(30*24*time.Hour).Unix())
	p := base64.RawURLEncoding.EncodeToString([]byte(payload))
	mac := hmac.New(sha256.New, a.cfg.SessionKey)
	mac.Write([]byte(p))
	sig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	http.SetCookie(w, &http.Cookie{Name: cookieName, Value: p + "." + sig, Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode, MaxAge: 30 * 24 * 3600})
}
func (a *App) login(w http.ResponseWriter, r *http.Request) {
	var body struct{ Email, Password string }
	if decodeJSON(r, &body) != nil {
		errorJSON(w, 400, "Invalid request")
		return
	}
	var id int64
	var hash string
	if a.db.QueryRow(`SELECT id,password FROM users WHERE email=?`, body.Email).Scan(&id, &hash) != nil || !verifyPassword(hash, body.Password) {
		errorJSON(w, 401, "Invalid credentials")
		return
	}
	a.setSession(w, id)
	w.WriteHeader(http.StatusCreated)
}
func verifyPassword(hash, password string) bool {
	if strings.HasPrefix(hash, "$2") {
		return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
	}
	if strings.HasPrefix(hash, "$scrypt$") {
		parts := strings.Split(hash, "$")
		if len(parts) != 5 {
			return false
		}
		params := map[string]int{}
		for _, pair := range strings.Split(parts[2], ",") {
			kv := strings.SplitN(pair, "=", 2)
			if len(kv) == 2 {
				params[kv[0]], _ = strconv.Atoi(kv[1])
			}
		}
		salt, err := base64.RawStdEncoding.DecodeString(parts[3])
		if err != nil {
			salt, err = base64.StdEncoding.DecodeString(parts[3])
		}
		expected, hashErr := base64.RawStdEncoding.DecodeString(parts[4])
		if hashErr != nil {
			expected, hashErr = base64.StdEncoding.DecodeString(parts[4])
		}
		if err != nil || hashErr != nil || params["n"] == 0 || params["r"] == 0 || params["p"] == 0 {
			return false
		}
		derived, deriveErr := scrypt.Key([]byte(password), salt, params["n"], params["r"], params["p"], len(expected))
		return deriveErr == nil && subtle.ConstantTimeCompare(derived, expected) == 1
	}
	return subtle.ConstantTimeCompare([]byte(hash), []byte(password)) == 1
}
func (a *App) logout(w http.ResponseWriter, _ *http.Request) {
	http.SetCookie(w, &http.Cookie{Name: cookieName, Value: "", Path: "/", MaxAge: -1, HttpOnly: true})
	w.WriteHeader(http.StatusNoContent)
}
func (a *App) profile(w http.ResponseWriter, r *http.Request) {
	u, ok := a.user(r)
	if !ok {
		writeJSON(w, 200, nil)
		return
	}
	writeJSON(w, 200, u)
}

type Photo struct {
	ID                string `json:"id"`
	Title             any    `json:"title"`
	Description       any    `json:"description"`
	Width             any    `json:"width"`
	Height            any    `json:"height"`
	AspectRatio       any    `json:"aspectRatio"`
	DateTaken         any    `json:"dateTaken"`
	StorageKey        any    `json:"storageKey"`
	ThumbnailKey      any    `json:"thumbnailKey"`
	FileSize          any    `json:"fileSize"`
	LastModified      any    `json:"lastModified"`
	OriginalURL       any    `json:"originalUrl"`
	ThumbnailURL      any    `json:"thumbnailUrl"`
	ThumbnailHash     any    `json:"thumbnailHash"`
	Tags              any    `json:"tags"`
	Exif              any    `json:"exif"`
	Latitude          any    `json:"latitude"`
	Longitude         any    `json:"longitude"`
	Country           any    `json:"country"`
	City              any    `json:"city"`
	LocationName      any    `json:"locationName"`
	IsLivePhoto       any    `json:"isLivePhoto"`
	LivePhotoVideoURL any    `json:"livePhotoVideoUrl"`
	LivePhotoVideoKey any    `json:"livePhotoVideoKey"`
}

func scanPhoto(rows *sql.Rows) (Photo, error) {
	var p Photo
	var tags, exif sql.NullString
	err := rows.Scan(&p.ID, &p.Title, &p.Description, &p.Width, &p.Height, &p.AspectRatio, &p.DateTaken, &p.StorageKey, &p.ThumbnailKey, &p.FileSize, &p.LastModified, &p.OriginalURL, &p.ThumbnailURL, &p.ThumbnailHash, &tags, &exif, &p.Latitude, &p.Longitude, &p.Country, &p.City, &p.LocationName, &p.IsLivePhoto, &p.LivePhotoVideoURL, &p.LivePhotoVideoKey)
	if tags.Valid {
		var v any
		if json.Unmarshal([]byte(tags.String), &v) == nil {
			p.Tags = v
		}
	}
	if exif.Valid {
		var v any
		if json.Unmarshal([]byte(exif.String), &v) == nil {
			p.Exif = v
		}
	}
	return p, err
}

const photoSelect = `SELECT id,title,description,width,height,aspect_ratio,date_taken,storage_key,thumbnail_key,file_size,last_modified,original_url,thumbnail_url,thumbnail_hash,tags,exif,latitude,longitude,country,city,location_name,is_live_photo,live_photo_video_url,live_photo_video_key FROM photos`

func (a *App) photos(w http.ResponseWriter, r *http.Request, visible bool) {
	if !visible {
		if u, ok := a.user(r); !ok || u == nil {
			errorJSON(w, 401, "Unauthorized")
			return
		}
	}
	query := photoSelect
	if visible {
		query += ` WHERE NOT EXISTS (SELECT 1 FROM album_photos ap JOIN albums al ON al.id=ap.album_id WHERE ap.photo_id=photos.id AND al.is_hidden=1)`
	}
	query += ` ORDER BY date_taken DESC`
	rows, err := a.db.Query(query)
	if err != nil {
		errorJSON(w, 500, err.Error())
		return
	}
	defer rows.Close()
	out := []Photo{}
	for rows.Next() {
		p, err := scanPhoto(rows)
		if err == nil {
			out = append(out, p)
		}
	}
	writeJSON(w, 200, out)
}

func (a *App) prepareUpload(w http.ResponseWriter, r *http.Request) {
	if !a.requireAdmin(w, r) {
		return
	}
	var b struct {
		FileName, ContentType string
		SkipDuplicateCheck    bool
	}
	if decodeJSON(r, &b) != nil || b.FileName == "" {
		errorJSON(w, 400, "fileName is required")
		return
	}
	key := storageKey(a.storage.Prefix(), b.FileName)
	id := safePhotoID(key)
	var existing Photo
	var found bool
	row := a.db.QueryRow(photoSelect+` WHERE id=?`, id)
	var rows *sql.Rows
	_ = rows
	var rawTags, rawExif sql.NullString
	err := row.Scan(&existing.ID, &existing.Title, &existing.Description, &existing.Width, &existing.Height, &existing.AspectRatio, &existing.DateTaken, &existing.StorageKey, &existing.ThumbnailKey, &existing.FileSize, &existing.LastModified, &existing.OriginalURL, &existing.ThumbnailURL, &existing.ThumbnailHash, &rawTags, &rawExif, &existing.Latitude, &existing.Longitude, &existing.Country, &existing.City, &existing.LocationName, &existing.IsLivePhoto, &existing.LivePhotoVideoURL, &existing.LivePhotoVideoKey)
	if err == nil {
		found = true
	}
	if found && !b.SkipDuplicateCheck {
		writeJSON(w, 200, map[string]any{"skipped": true, "duplicate": true, "existingPhoto": existing, "fileKey": key, "title": "Duplicate file", "message": "File already exists"})
		return
	}
	signed, err := a.storage.SignedURL(r.Context(), key, b.ContentType)
	if err != nil {
		signed = "/api/photos/upload?key=" + urlQueryEscape(key)
	}
	writeJSON(w, 200, map[string]any{"signedUrl": signed, "fileKey": key, "expiresIn": 3600, "duplicate": found, "existingPhoto": func() any {
		if found {
			return existing
		}
		return nil
	}()})
}
func (a *App) upload(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.user(r); !ok {
		errorJSON(w, 401, "Unauthorized")
		return
	}
	key := r.URL.Query().Get("key")
	if key == "" {
		errorJSON(w, 400, "key is required")
		return
	}
	max := int64(envInt("CFRAME_MAX_UPLOAD_MB", 256)) * 1024 * 1024
	r.Body = http.MaxBytesReader(w, r.Body, max)
	data, err := io.ReadAll(r.Body)
	if err != nil {
		errorJSON(w, 413, "Upload too large")
		return
	}
	if _, err = a.storage.Create(r.Context(), key, data, r.Header.Get("Content-Type")); err != nil {
		errorJSON(w, 500, "Upload failed")
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true, "key": key})
}
func (a *App) checkDuplicate(w http.ResponseWriter, r *http.Request) {
	if !a.requireAdmin(w, r) {
		return
	}
	var b struct{ FileName string }
	if decodeJSON(r, &b) != nil {
		errorJSON(w, 400, "Invalid request")
		return
	}
	id := safePhotoID(storageKey(a.storage.Prefix(), b.FileName))
	var count int
	a.db.QueryRow(`SELECT count(*) FROM photos WHERE id=?`, id).Scan(&count)
	writeJSON(w, 200, map[string]any{"duplicate": count > 0})
}
func (a *App) photoStatus(w http.ResponseWriter, r *http.Request) {
	if !a.requireAdmin(w, r) {
		return
	}
	var total, processed int
	a.db.QueryRow(`SELECT count(*) FROM photos`).Scan(&total)
	a.db.QueryRow(`SELECT count(*) FROM pipeline_queue WHERE status='completed'`).Scan(&processed)
	writeJSON(w, 200, map[string]any{"total": total, "processed": processed})
}
func (a *App) reactions(w http.ResponseWriter, r *http.Request) {
	ids := r.URL.Query()["ids"]
	if len(ids) == 1 {
		ids = strings.Split(ids[0], ",")
	}
	out := map[string]map[string]int{}
	for _, id := range ids {
		rows, _ := a.db.Query(`SELECT reaction_type,count(*) FROM photo_reactions WHERE photo_id=? GROUP BY reaction_type`, id)
		m := map[string]int{}
		for rows != nil && rows.Next() {
			var typ string
			var n int
			_ = rows.Scan(&typ, &n)
			m[typ] = n
		}
		if rows != nil {
			rows.Close()
		}
		out[id] = m
	}
	writeJSON(w, 200, out)
}

func (a *App) photoRoute(w http.ResponseWriter, r *http.Request, rest string) {
	parts := strings.Split(strings.Trim(rest, "/"), "/")
	if len(parts) == 0 {
		return
	}
	id := parts[0]
	if len(parts) == 1 && r.Method == "PUT" {
		a.updatePhoto(w, r, id)
		return
	}
	if len(parts) == 1 && r.Method == "DELETE" {
		a.deletePhoto(w, r, id)
		return
	}
	if len(parts) >= 2 && parts[1] == "albums" && r.Method == "GET" {
		writeJSON(w, 200, []any{})
		return
	}
	if len(parts) >= 2 && parts[1] == "livephoto" {
		a.photoLive(w, r, id)
		return
	}
	if len(parts) >= 2 && parts[1] == "reactions" {
		a.photoReaction(w, r, id)
		return
	}
	if len(parts) >= 2 && parts[1] == "exif" {
		writeJSON(w, 200, map[string]any{"success": true})
		return
	}
	errorJSON(w, 404, "Not Found")
}
func (a *App) updatePhoto(w http.ResponseWriter, r *http.Request, id string) {
	if !a.requireAdmin(w, r) {
		return
	}
	var b map[string]any
	if decodeJSON(r, &b) != nil {
		errorJSON(w, 400, "Invalid request")
		return
	}
	allowed := map[string]string{"title": "title", "description": "description", "tags": "tags", "latitude": "latitude", "longitude": "longitude"}
	sets := []string{}
	args := []any{}
	for key, col := range allowed {
		if v, ok := b[key]; ok {
			sets = append(sets, col+"=?")
			if key == "tags" {
				args = append(args, jsonValue(v))
			} else {
				args = append(args, v)
			}
		}
	}
	if len(sets) > 0 {
		args = append(args, id)
		_, _ = a.db.Exec(`UPDATE photos SET `+strings.Join(sets, ",")+` WHERE id=?`, args...)
	}
	writeJSON(w, 200, map[string]any{"success": true})
}
func (a *App) deletePhoto(w http.ResponseWriter, r *http.Request, id string) {
	if !a.requireAdmin(w, r) {
		return
	}
	var key, thumb string
	if a.db.QueryRow(`SELECT storage_key,thumbnail_key FROM photos WHERE id=?`, id).Scan(&key, &thumb) != nil {
		errorJSON(w, 404, "Photo not found")
		return
	}
	_ = a.storage.Delete(r.Context(), key)
	if thumb != "" {
		_ = a.storage.Delete(r.Context(), thumb)
	}
	_, _ = a.db.Exec(`DELETE FROM photos WHERE id=?`, id)
	w.WriteHeader(http.StatusNoContent)
}
func (a *App) photoReaction(w http.ResponseWriter, r *http.Request, id string) {
	u, _ := a.user(r)
	fingerprint := r.RemoteAddr
	if u != nil {
		fingerprint = fmt.Sprint(u["id"])
	}
	if r.Method == "GET" {
		rows, _ := a.db.Query(`SELECT reaction_type,count(*) FROM photo_reactions WHERE photo_id=? GROUP BY reaction_type`, id)
		out := map[string]int{}
		for rows != nil && rows.Next() {
			var typ string
			var count int
			_ = rows.Scan(&typ, &count)
			out[typ] = count
		}
		if rows != nil {
			rows.Close()
		}
		writeJSON(w, 200, out)
		return
	}
	if r.Method == "DELETE" {
		_, _ = a.db.Exec(`DELETE FROM photo_reactions WHERE photo_id=? AND fingerprint=?`, id, fingerprint)
		w.WriteHeader(http.StatusNoContent)
		return
	}
	var body struct {
		ReactionType string `json:"reactionType"`
	}
	if decodeJSON(r, &body) != nil || body.ReactionType == "" {
		errorJSON(w, 400, "reactionType is required")
		return
	}
	_, err := a.db.Exec(`INSERT INTO photo_reactions(photo_id,reaction_type,fingerprint,ip_address,user_agent) VALUES(?,?,?,?,?)`, id, body.ReactionType, fingerprint, r.RemoteAddr, r.UserAgent())
	if err != nil {
		errorJSON(w, 500, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"success": true})
}
func (a *App) photoLive(w http.ResponseWriter, _ *http.Request, id string) {
	var row Photo
	rows, err := a.db.Query(photoSelect+` WHERE id=?`, id)
	if err != nil {
		errorJSON(w, 500, err.Error())
		return
	}
	defer rows.Close()
	if rows.Next() {
		row, _ = scanPhoto(rows)
	}
	writeJSON(w, 200, map[string]any{"photo": row, "originalUrl": row.OriginalURL, "thumbnailUrl": row.ThumbnailURL, "livePhotoVideoUrl": row.LivePhotoVideoURL})
}
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

func (a *App) queueRoute(w http.ResponseWriter, r *http.Request, rest string) {
	if rest == "add-task" && r.Method == "POST" {
		a.addTask(w, r, false)
		return
	}
	if rest == "add-tasks" && r.Method == "POST" {
		a.addTask(w, r, true)
		return
	}
	if rest == "stats" && r.Method == "GET" {
		a.queueStats(w, r, 0)
		return
	}
	if strings.HasPrefix(rest, "stats/") && r.Method == "GET" {
		id, _ := strconv.ParseInt(strings.TrimPrefix(rest, "stats/"), 10, 64)
		a.queueStats(w, r, id)
		return
	}
	if rest == "task/list" && r.Method == "GET" {
		a.taskList(w, r)
		return
	}
	if rest == "task/clear" && r.Method == "DELETE" {
		a.taskClear(w, r)
		return
	}
	if rest == "task/retry" && r.Method == "POST" {
		a.taskRetry(w, r)
		return
	}
	if rest == "task/retry-batch" && r.Method == "POST" {
		a.taskRetryBatch(w, r)
		return
	}
	errorJSON(w, 404, "Not Found")
}
func (a *App) addTask(w http.ResponseWriter, r *http.Request, batch bool) {
	if !a.requireAdmin(w, r) {
		return
	}
	var body struct {
		Payload     map[string]any   `json:"payload"`
		Tasks       []map[string]any `json:"tasks"`
		Priority    int              `json:"priority"`
		MaxAttempts int              `json:"maxAttempts"`
	}
	if decodeJSON(r, &body) != nil {
		errorJSON(w, 400, "Invalid request")
		return
	}
	if body.MaxAttempts < 1 {
		body.MaxAttempts = 3
	}
	items := []map[string]any{}
	if batch {
		items = body.Tasks
	} else {
		items = []map[string]any{body.Payload}
	}
	ids := []int64{}
	for _, payload := range items {
		if payload == nil {
			continue
		}
		data, _ := json.Marshal(payload)
		res, err := a.db.Exec(`INSERT INTO pipeline_queue(payload,priority,max_attempts,status) VALUES(?,?,?,'pending')`, string(data), body.Priority, body.MaxAttempts)
		if err != nil {
			errorJSON(w, 500, err.Error())
			return
		}
		id, _ := res.LastInsertId()
		ids = append(ids, id)
	}
	a.wakeQueue()
	if batch {
		writeJSON(w, 201, map[string]any{"success": true, "taskIds": ids})
	} else {
		var id int64
		if len(ids) > 0 {
			id = ids[0]
		}
		writeJSON(w, 201, map[string]any{"success": true, "taskId": id, "message": "Task added to queue successfully"})
	}
}
func (a *App) queueStats(w http.ResponseWriter, r *http.Request, id int64) {
	if !a.requireAdmin(w, r) {
		return
	}
	if id > 0 {
		var payload, status, stage, errMsg string
		var attempts, max int
		if a.db.QueryRow(`SELECT payload,status,COALESCE(status_stage,''),COALESCE(error_message,''),attempts,max_attempts FROM pipeline_queue WHERE id=?`, id).Scan(&payload, &status, &stage, &errMsg, &attempts, &max) != nil {
			errorJSON(w, 404, "Task not found")
			return
		}
		var p any
		_ = json.Unmarshal([]byte(payload), &p)
		writeJSON(w, 200, map[string]any{"id": id, "payload": p, "status": status, "statusStage": stage, "errorMessage": errMsg, "attempts": attempts, "maxAttempts": max})
		return
	}
	rows, _ := a.db.Query(`SELECT status,count(*) FROM pipeline_queue GROUP BY status`)
	out := map[string]int{}
	for rows != nil && rows.Next() {
		var s string
		var n int
		_ = rows.Scan(&s, &n)
		out[s] = n
	}
	if rows != nil {
		rows.Close()
	}
	writeJSON(w, 200, out)
}
func (a *App) taskList(w http.ResponseWriter, r *http.Request) {
	if !a.requireAdmin(w, r) {
		return
	}
	rows, _ := a.db.Query(`SELECT id,payload,priority,attempts,max_attempts,status,status_stage,error_message,created_at,completed_at FROM pipeline_queue ORDER BY id DESC LIMIT 500`)
	out := []map[string]any{}
	for rows != nil && rows.Next() {
		var id, priority, attempts, max int
		var p, status, stage, errMsg string
		var created int64
		var completed sql.NullInt64
		_ = rows.Scan(&id, &p, &priority, &attempts, &max, &status, &stage, &errMsg, &created, &completed)
		var payload any
		_ = json.Unmarshal([]byte(p), &payload)
		out = append(out, map[string]any{"id": id, "payload": payload, "priority": priority, "attempts": attempts, "maxAttempts": max, "status": status, "statusStage": stage, "errorMessage": errMsg, "createdAt": time.Unix(created, 0), "completedAt": completed.Int64})
	}
	if rows != nil {
		rows.Close()
	}
	writeJSON(w, 200, out)
}
func (a *App) taskClear(w http.ResponseWriter, r *http.Request) {
	if !a.requireAdmin(w, r) {
		return
	}
	_, _ = a.db.Exec(`DELETE FROM pipeline_queue WHERE status IN ('completed','failed')`)
	writeJSON(w, 200, map[string]any{"success": true})
}
func (a *App) taskRetry(w http.ResponseWriter, r *http.Request) {
	if !a.requireAdmin(w, r) {
		return
	}
	var b struct {
		TaskID int64 `json:"taskId"`
	}
	if decodeJSON(r, &b) != nil {
		errorJSON(w, 400, "Invalid request")
		return
	}
	_, err := a.db.Exec(`UPDATE pipeline_queue SET status='pending',error_message=NULL,created_at=unixepoch() WHERE id=?`, b.TaskID)
	if err != nil {
		errorJSON(w, 500, err.Error())
		return
	}
	a.wakeQueue()
	writeJSON(w, 200, map[string]any{"success": true})
}
func (a *App) taskRetryBatch(w http.ResponseWriter, r *http.Request) {
	if !a.requireAdmin(w, r) {
		return
	}
	var b struct {
		TaskIDs []int64 `json:"taskIds"`
	}
	if decodeJSON(r, &b) != nil {
		errorJSON(w, 400, "Invalid request")
		return
	}
	for _, id := range b.TaskIDs {
		_, _ = a.db.Exec(`UPDATE pipeline_queue SET status='pending',error_message=NULL,created_at=unixepoch() WHERE id=?`, id)
	}
	a.wakeQueue()
	writeJSON(w, 200, map[string]any{"success": true})
}

func (a *App) settingsRoute(w http.ResponseWriter, r *http.Request, rest string) {
	if rest == "/all" && r.Method == "GET" {
		a.allSettings(w, r)
		return
	}
	if rest == "/schema" && r.Method == "GET" {
		writeJSON(w, 200, map[string]any{"fields": []any{}})
		return
	}
	if rest == "/fields" && r.Method == "GET" {
		writeJSON(w, 200, []any{})
		return
	}
	if !a.requireAdmin(w, r) {
		return
	}
	if r.Method == "PUT" && rest == "/batch" {
		var b map[string]any
		_ = decodeJSON(r, &b)
		for key, v := range b {
			parts := strings.SplitN(key, ":", 2)
			if len(parts) == 2 {
				a.setSetting(parts[0], parts[1], v)
			}
		}
		writeJSON(w, 200, map[string]any{"success": true})
		return
	}
	parts := strings.Split(strings.Trim(rest, "/"), "/")
	if len(parts) >= 2 && r.Method == "GET" {
		var value any
		if a.readSetting(parts[0], parts[1], &value) {
			writeJSON(w, 200, value)
		} else {
			errorJSON(w, 404, "Setting not found")
		}
		return
	}
	if len(parts) >= 2 && r.Method == "PUT" {
		var v any
		_ = decodeJSON(r, &v)
		a.setSetting(parts[0], parts[1], v)
		writeJSON(w, 200, map[string]any{"success": true})
		return
	}
	writeJSON(w, 200, map[string]any{"success": true})
}
func (a *App) allSettings(w http.ResponseWriter, _ *http.Request) {
	rows, _ := a.db.Query(`SELECT namespace,key,type,value,default_value,is_public FROM settings`)
	data := map[string]map[string]any{}
	for rows != nil && rows.Next() {
		var ns, key, typ string
		var value, def sql.NullString
		var pub int
		_ = rows.Scan(&ns, &key, &typ, &value, &def, &pub)
		if data[ns] == nil {
			data[ns] = map[string]any{}
		}
		data[ns][key] = parseSetting(typ, func() string {
			if value.Valid {
				return value.String
			}
			return def.String
		}())
	}
	if rows != nil {
		rows.Close()
	}
	if data["app"] == nil {
		data["app"] = map[string]any{}
	}
	if _, ok := data["app"]["title"]; !ok {
		data["app"]["title"] = env("NUXT_PUBLIC_APP_TITLE", "ChronoFrame")
	}
	writeJSON(w, 200, map[string]any{"timestamp": time.Now().UnixMilli(), "data": data})
}
func parseSetting(typ, value string) any {
	switch typ {
	case "number":
		n, _ := strconv.ParseFloat(value, 64)
		return n
	case "boolean":
		return value == "true" || value == "1"
	case "json":
		var v any
		if json.Unmarshal([]byte(value), &v) == nil {
			return v
		}
	}
	return value
}
func (a *App) readSetting(ns, key string, out *any) bool {
	var typ string
	var value, def sql.NullString
	if a.db.QueryRow(`SELECT type,value,default_value FROM settings WHERE namespace=? AND key=?`, ns, key).Scan(&typ, &value, &def) != nil {
		return false
	}
	v := value.String
	if !value.Valid {
		v = def.String
	}
	*out = parseSetting(typ, v)
	return true
}
func (a *App) setSetting(ns, key string, value any) {
	b := jsonValue(value)
	typ := "json"
	switch value.(type) {
	case string:
		typ = "string"
		b = fmt.Sprint(value)
	case bool:
		typ = "boolean"
		b = fmt.Sprint(value)
	case float64, int, int64:
		typ = "number"
	}
	_, _ = a.db.Exec(`INSERT INTO settings(namespace,key,type,value,updated_at) VALUES(?,?,?,?,unixepoch()) ON CONFLICT(namespace,key) DO UPDATE SET type=excluded.type,value=excluded.value,updated_at=unixepoch()`, ns, key, typ, b)
}
func (a *App) systemStats(w http.ResponseWriter, r *http.Request) {
	if !a.requireAdmin(w, r) {
		return
	}
	var total int
	a.db.QueryRow(`SELECT count(*) FROM photos`).Scan(&total)
	writeJSON(w, 200, map[string]any{"total": total, "dateRange": nil, "storage": map[string]any{}})
}
func (a *App) systemLogs(w http.ResponseWriter, r *http.Request) {
	if !a.requireAdmin(w, r) {
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	for _, line := range a.logs.Snapshot() {
		fmt.Fprintf(w, "data: %s\n\n", line)
	}
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
	select {
	case <-r.Context().Done():
	case <-time.After(30 * time.Second):
	}
}

func (a *App) wizardRoute(w http.ResponseWriter, r *http.Request, rest string) {
	if rest == "schema" && r.Method == "GET" {
		writeJSON(w, 200, map[string]any{"namespace": r.URL.Query().Get("namespace"), "fields": []any{}})
		return
	}
	if rest == "complete" && r.Method == "POST" {
		writeJSON(w, 200, map[string]any{"success": true})
		return
	}
	if rest == "submit" && r.Method == "POST" {
		var b struct {
			Admin   struct{ Email, Password, Username string }
			Site    map[string]any
			Storage map[string]any
			Map     map[string]any
		}
		if decodeJSON(r, &b) != nil {
			errorJSON(w, 400, "Invalid request")
			return
		}
		if b.Admin.Email != "" {
			hash, _ := bcrypt.GenerateFromPassword([]byte(b.Admin.Password), bcrypt.DefaultCost)
			_, _ = a.db.Exec(`INSERT INTO users(name,email,password,is_admin,created_at) VALUES(?,?,?,?,unixepoch()) ON CONFLICT(email) DO UPDATE SET name=excluded.name,password=excluded.password,is_admin=1`, b.Admin.Username, b.Admin.Email, string(hash), 1)
		}
		writeJSON(w, 200, map[string]any{"success": true})
		return
	}
	writeJSON(w, 200, map[string]any{"success": true})
}

func (a *App) serveStorage(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.storage.(*LocalStorage); !ok {
		errorJSON(w, 404, "Not Found")
		return
	}
	s := a.storage.(*LocalStorage)
	key := strings.TrimPrefix(r.URL.Path, "/storage/")
	p := s.path(key)
	if !safePath(s.base, p) {
		errorJSON(w, 400, "Invalid path")
		return
	}
	f, err := os.Open(p)
	if err != nil {
		errorJSON(w, 404, "Not Found")
		return
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		errorJSON(w, 404, "Not Found")
		return
	}
	etag := fmt.Sprintf(`W/"%d-%d"`, st.Size(), st.ModTime().UnixNano())
	w.Header().Set("ETag", etag)
	w.Header().Set("Last-Modified", st.ModTime().UTC().Format(http.TimeFormat))
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	w.Header().Set("Accept-Ranges", "bytes")
	http.ServeContent(w, r, st.Name(), st.ModTime(), f)
}
func safePath(base, p string) bool {
	base, _ = filepath.Abs(base)
	p, _ = filepath.Abs(p)
	return p == base || strings.HasPrefix(p, base+string(os.PathSeparator))
}
func (a *App) serveImage(w http.ResponseWriter, r *http.Request) {
	key := strings.TrimPrefix(r.URL.Path, "/image/")
	data, err := a.storage.Get(r.Context(), key)
	if err != nil {
		errorJSON(w, 404, "Photo not found")
		return
	}
	w.Header().Set("Cache-Control", "public,max-age=31536000,immutable")
	http.ServeContent(w, r, filepath.Base(key), time.Time{}, strings.NewReader(string(data)))
}
func (a *App) serveThumb(w http.ResponseWriter, r *http.Request) {
	target := strings.TrimPrefix(r.URL.Path, "/thumb/")
	target, _ = urlPathUnescape(target)
	if strings.HasPrefix(target, "http://") || strings.HasPrefix(target, "https://") || strings.HasPrefix(target, "/storage/") {
		resp, err := http.Get(target)
		if err != nil {
			errorJSON(w, 404, "Photo not found")
			return
		}
		defer resp.Body.Close()
		w.Header().Set("Content-Type", "image/jpeg")
		w.WriteHeader(resp.StatusCode)
		_, _ = io.Copy(w, resp.Body)
		return
	}
	data, err := a.storage.Get(r.Context(), target)
	if err != nil {
		errorJSON(w, 404, "Photo not found")
		return
	}
	w.Header().Set("Content-Type", "image/webp")
	w.Header().Set("Cache-Control", "public,max-age=31536000,immutable")
	w.Write(data)
}
func urlPathUnescape(v string) (string, error) { return strings.ReplaceAll(v, "%2F", "/"), nil }
func (a *App) serveWeb(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	if path == "" || path == "/" || filepath.Ext(path) == "" {
		path = "/index.html"
	}
	file := filepath.Join(a.cfg.WebDir, filepath.FromSlash(strings.TrimPrefix(path, "/")))
	if data, err := os.ReadFile(file); err == nil {
		http.ServeContent(w, r, filepath.Base(file), time.Time{}, strings.NewReader(string(data)))
		return
	}
	index := filepath.Join(a.cfg.WebDir, "index.html")
	http.ServeFile(w, r, index)
}

type LogBuffer struct {
	mu    sync.RWMutex
	lines []string
}

func NewLogBuffer() *LogBuffer { return &LogBuffer{lines: []string{}} }
func (l *LogBuffer) Add(scope, message string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.lines = append(l.lines, fmt.Sprintf("[%s] %s", scope, message))
	if len(l.lines) > 200 {
		l.lines = l.lines[len(l.lines)-200:]
	}
}
func (l *LogBuffer) Snapshot() []string {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return append([]string(nil), l.lines...)
}

var _ = sort.Strings
