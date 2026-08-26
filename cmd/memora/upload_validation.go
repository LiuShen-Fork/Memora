package main

import "strings"

const defaultUploadMIMEs = "image/jpeg,image/png,image/webp,image/gif,image/bmp,image/tiff,image/heic,image/heif,video/quicktime,video/mp4"

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

func uploadMIMEAllowed(contentType string) bool {
	if !envBool("NUXT_UPLOAD_MIME_WHITELIST_ENABLED", true) {
		return true
	}
	contentType = strings.TrimSpace(strings.Split(contentType, ";")[0])
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
