package main

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	defaultDBPath = "./data/app.sqlite3"
	defaultWebDir = "./web"
	cookieName    = "cframe_session"
)

// Config contains all process-level configuration. Provider-specific values
// are kept here only until an active provider is resolved from SQLite.
type Config struct {
	DBPath        string
	DataDir       string
	WebDir        string
	Addr          string
	SessionKey    []byte
	WorkerCount   int
	FFmpeg        string
	ExifTool      string
	Provider      string
	LocalPath     string
	LocalBaseURL  string
	LocalPrefix   string
	S3Endpoint    string
	S3Bucket      string
	S3Region      string
	S3AccessKey   string
	S3SecretKey   string
	S3Prefix      string
	S3CDN         string
	S3PathStyle   bool
	OpenBaseURL   string
	OpenRootPath  string
	OpenToken     string
	OpenUpload    string
	OpenDownload  string
	OpenList      string
	OpenDelete    string
	OpenMeta      string
	OpenPathField string
	OpenCDN       string
}

func env(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func envBool(key string, fallback bool) bool {
	value := strings.ToLower(strings.TrimSpace(os.Getenv(key)))
	if value == "" {
		return fallback
	}
	return value == "1" || value == "true" || value == "yes"
}

func envInt(key string, fallback int) int {
	value, err := strconv.Atoi(env(key, ""))
	if err != nil || value < 1 {
		return fallback
	}
	return value
}

func loadConfig() Config {
	dataDir := env("CFRAME_DATA_DIR", "./data")
	secret := env("NUXT_SESSION_PASSWORD", env("CFRAME_SESSION_SECRET", "change-me-in-production"))
	return Config{
		DBPath:        env("DATABASE_URL", filepath.Join(dataDir, "app.sqlite3")),
		DataDir:       dataDir,
		WebDir:        env("CFRAME_WEB_DIR", defaultWebDir),
		Addr:          env("CFRAME_ADDR", ":3000"),
		SessionKey:    []byte(secret),
		WorkerCount:   envInt("CFRAME_WORKERS", 2),
		FFmpeg:        env("CFRAME_FFMPEG_PATH", "ffmpeg"),
		ExifTool:      env("EXIFTOOL_PATH", "exiftool"),
		Provider:      env("NUXT_STORAGE_PROVIDER", "local"),
		LocalPath:     env("NUXT_PROVIDER_LOCAL_PATH", filepath.Join(dataDir, "storage")),
		LocalBaseURL:  env("NUXT_PROVIDER_LOCAL_BASE_URL", "/storage"),
		LocalPrefix:   env("NUXT_PROVIDER_LOCAL_PREFIX", "photos/"),
		S3Endpoint:    os.Getenv("NUXT_PROVIDER_S3_ENDPOINT"),
		S3Bucket:      os.Getenv("NUXT_PROVIDER_S3_BUCKET"),
		S3Region:      env("NUXT_PROVIDER_S3_REGION", "auto"),
		S3AccessKey:   os.Getenv("NUXT_PROVIDER_S3_ACCESS_KEY_ID"),
		S3SecretKey:   os.Getenv("NUXT_PROVIDER_S3_SECRET_ACCESS_KEY"),
		S3Prefix:      env("NUXT_PROVIDER_S3_PREFIX", "photos/"),
		S3CDN:         os.Getenv("NUXT_PROVIDER_S3_CDN_URL"),
		S3PathStyle:   envBool("NUXT_PROVIDER_S3_FORCE_PATH_STYLE", false),
		OpenBaseURL:   os.Getenv("NUXT_PROVIDER_OPENLIST_BASE_URL"),
		OpenRootPath:  os.Getenv("NUXT_PROVIDER_OPENLIST_ROOT_PATH"),
		OpenToken:     os.Getenv("NUXT_PROVIDER_OPENLIST_TOKEN"),
		OpenUpload:    env("NUXT_PROVIDER_OPENLIST_ENDPOINT_UPLOAD", "/api/fs/put"),
		OpenDownload:  os.Getenv("NUXT_PROVIDER_OPENLIST_ENDPOINT_DOWNLOAD"),
		OpenList:      os.Getenv("NUXT_PROVIDER_OPENLIST_ENDPOINT_LIST"),
		OpenDelete:    env("NUXT_PROVIDER_OPENLIST_ENDPOINT_DELETE", "/api/fs/remove"),
		OpenMeta:      env("NUXT_PROVIDER_OPENLIST_ENDPOINT_META", "/api/fs/get"),
		OpenPathField: env("NUXT_PROVIDER_OPENLIST_PATH_FIELD", "path"),
		OpenCDN:       os.Getenv("NUXT_PROVIDER_OPENLIST_CDN_URL"),
	}
}
