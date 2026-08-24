package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	_ "modernc.org/sqlite"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
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
	db, err := openDatabase(cfg.DBPath)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	app := &App{cfg: cfg, db: db, queueWake: make(chan struct{}, 1), stop: make(chan struct{}), logs: NewLogBuffer(filepath.Join(cfg.DataDir, "logs", "app.log"))}
	if err := app.ensureSchema(); err != nil {
		log.Fatal(err)
	}
	if err := app.ensureDefaultSettings(); err != nil {
		log.Fatal(err)
	}
	app.migrateEnvironmentSettings()
	app.storage = app.loadStorage()
	app.startWorkers()
	defer app.stopWorkers()

	server := &http.Server{Addr: cfg.Addr, Handler: app}
	app.logs.Add("server", "ChronoFrame Go backend listening on "+cfg.Addr)
	log.Printf("ChronoFrame Go backend listening on %s", cfg.Addr)
	if err := app.serve(server); err != nil {
		log.Fatal(err)
	}
}

func (a *App) serve(server *http.Server) error {
	errorsCh := make(chan error, 1)
	go func() { errorsCh <- server.ListenAndServe() }()

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(signals)

	select {
	case err := <-errorsCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case sig := <-signals:
		a.logs.Add("server", "received "+sig.String()+", shutting down")
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		return server.Shutdown(ctx)
	}
}

func openDatabase(path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", path+"?_pragma=busy_timeout(10000)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)&_pragma=foreign_keys(ON)")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	return db, nil
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
	boolean := func(key string, fallback bool) bool {
		if value, ok := config[key].(bool); ok {
			return value
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
		a.cfg.S3PathStyle = boolean("forcePathStyle", a.cfg.S3PathStyle)
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
