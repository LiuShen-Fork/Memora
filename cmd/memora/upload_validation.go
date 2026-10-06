package main

import (
	"path/filepath"
	"strings"
)

const defaultUploadMIMEs = "image/jpeg,image/png,image/webp,image/gif,image/bmp,image/tiff,image/heic,image/heif,video/mp4"

// uploadContentTypes maps the extensions the app accepts to the type a browser
// reports for them. Browsers regularly have no type at all for HEIC/HEIF and
// send an empty type, which reaches the server as application/octet-stream.
var uploadContentTypes = map[string]string{
	".avif": "image/avif",
	".bmp":  "image/bmp",
	".gif":  "image/gif",
	".heic": "image/heic",
	".heif": "image/heif",
	".hif":  "image/heif",
	".jpeg": "image/jpeg",
	".jpg":  "image/jpeg",
	".mov":  "video/quicktime",
	".mp4":  "video/mp4",
	".png":  "image/png",
	".tif":  "image/tiff",
	".tiff": "image/tiff",
	".webp": "image/webp",
}

func (a *App) maxUploadBytes() int64 {
	megabytes := envInt("CFRAME_MAX_UPLOAD_MB", 32)
	var configured any
	if a.readSetting("system", "upload.maxFileSize", &configured) {
		switch value := configured.(type) {
		case float64:
			megabytes = int(value)
		case int:
			megabytes = value
		}
	}
	if megabytes < 1 {
		megabytes = 1
	}
	return int64(megabytes) * 1024 * 1024
}

func uploadMIMEAllowed(contentType, key string) bool {
	if !envBool("NUXT_UPLOAD_MIME_WHITELIST_ENABLED", true) {
		return true
	}
	contentType = strings.TrimSpace(strings.Split(contentType, ";")[0])
	// Only an undeclared type falls back to the extension. A type the client did
	// report stays authoritative, so this cannot become a way around the
	// whitelist.
	if contentType == "application/octet-stream" {
		if resolved, ok := uploadContentTypes[strings.ToLower(filepath.Ext(key))]; ok {
			contentType = resolved
		}
	}
	for _, allowed := range strings.Split(env("NUXT_UPLOAD_MIME_WHITELIST", defaultUploadMIMEs), ",") {
		if contentType == strings.TrimSpace(allowed) {
			return true
		}
	}
	return false
}

func duplicateMode(a *App) string {
	var value any
	if !a.readSetting("system", "upload.duplicateCheck.mode", &value) {
		return "skip"
	}
	mode, _ := value.(string)
	if mode == "warn" || mode == "block" || mode == "skip" {
		return mode
	}
	return "skip"
}

func duplicateCheckEnabled(a *App) bool {
	var value any
	if !a.readSetting("system", "upload.duplicateCheck.enabled", &value) {
		return true
	}
	enabled, ok := value.(bool)
	return !ok || enabled
}
