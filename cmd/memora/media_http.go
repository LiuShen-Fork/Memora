package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func (a *App) serveStorage(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.storage.(*LocalStorage); !ok {
		errorJSON(w, 404, "Not Found")
		return
	}
	s := a.storage.(*LocalStorage)
	key := strings.TrimPrefix(r.URL.Path, "/storage/")
	p, err := s.path(key)
	if err != nil || !safePath(s.base, p) {
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
	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	http.ServeContent(w, r, st.Name(), st.ModTime(), f)
}
func safePath(base, p string) bool {
	base, _ = filepath.Abs(base)
	p, _ = filepath.Abs(p)
	return p == base || strings.HasPrefix(p, base+string(os.PathSeparator))
}
func (a *App) serveImage(w http.ResponseWriter, r *http.Request) {
	key := strings.TrimPrefix(r.URL.Path, "/image/")
	mediaCtx, cancel := a.mediaContext(r.Context())
	defer cancel()
	if streamStorage, ok := a.storage.(ReaderStorage); ok {
		reader, object, resolvedKey, err := a.openMedia(streamStorage, mediaCtx, key)
		if err != nil {
			errorJSON(w, http.StatusNotFound, "Photo not found")
			return
		}
		defer reader.Close()
		w.Header().Set("Cache-Control", "public,max-age=31536000,immutable")
		if contentType := mime.TypeByExtension(strings.ToLower(filepath.Ext(resolvedKey))); contentType != "" {
			w.Header().Set("Content-Type", contentType)
		}
		if seeker, ok := reader.(io.ReadSeeker); ok {
			http.ServeContent(w, r, filepath.Base(resolvedKey), object.ModTime, seeker)
			return
		}
		if r.Header.Get("Range") != "" {
			data, err := readLimited(reader, a.mediaLimit())
			if err != nil {
				errorJSON(w, http.StatusBadGateway, "Unable to read photo")
				return
			}
			http.ServeContent(w, r, filepath.Base(resolvedKey), object.ModTime, bytes.NewReader(data))
			return
		}
		if object.Size >= 0 {
			w.Header().Set("Content-Length", fmt.Sprint(object.Size))
		}
		_, _ = io.Copy(w, reader)
		return
	}
	data, err := a.readStorageBytes(mediaCtx, key)
	if err != nil {
		errorJSON(w, 404, "Photo not found")
		return
	}
	w.Header().Set("Cache-Control", "public,max-age=31536000,immutable")
	http.ServeContent(w, r, filepath.Base(key), time.Time{}, bytes.NewReader(data))
}

// openMedia tolerates legacy records whose video extension does not match the
// object that was actually uploaded. The requested key remains the public URL;
// only the storage lookup and response metadata use the resolved key.
func (a *App) openMedia(storage ReaderStorage, ctx context.Context, key string) (io.ReadCloser, Object, string, error) {
	keys := []string{key}
	switch strings.ToLower(filepath.Ext(key)) {
	case ".mov":
		keys = append(keys, strings.TrimSuffix(key, filepath.Ext(key))+".mp4")
	case ".mp4":
		keys = append(keys, strings.TrimSuffix(key, filepath.Ext(key))+".mov")
	}
	var lastErr error
	for _, candidate := range keys {
		reader, object, err := storage.Open(ctx, candidate)
		if err == nil {
			return reader, object, candidate, nil
		}
		lastErr = err
	}
	return nil, Object{}, "", lastErr
}
func (a *App) serveThumb(w http.ResponseWriter, r *http.Request) {
	target := strings.TrimPrefix(r.URL.Path, "/thumb/")
	target, _ = urlPathUnescape(target)
	if strings.HasPrefix(target, "/storage/") || strings.HasPrefix(target, "/image/") {
		key := strings.TrimPrefix(strings.TrimPrefix(target, "/storage/"), "/image/")
		data, err := a.readStorageBytes(r.Context(), key)
		if err != nil {
			errorJSON(w, 404, "Photo not found")
			return
		}
		a.writeThumbnail(w, r, data, mime.TypeByExtension(strings.ToLower(filepath.Ext(key))))
		return
	}
	if strings.HasPrefix(target, "http://") || strings.HasPrefix(target, "https://") {
		req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, target, nil)
		if err != nil {
			errorJSON(w, 404, "Photo not found")
			return
		}
		mediaCtx, cancel := a.mediaContext(r.Context())
		defer cancel()
		req = req.WithContext(mediaCtx)
		resp, err := storageHTTPClient.Do(req)
		if err != nil {
			errorJSON(w, 404, "Photo not found")
			return
		}
		defer resp.Body.Close()
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			errorJSON(w, http.StatusNotFound, "Photo not found")
			return
		}
		data, err := readLimited(resp.Body, a.mediaLimit())
		if err != nil {
			errorJSON(w, http.StatusBadGateway, "Unable to read photo")
			return
		}
		a.writeThumbnail(w, r, data, resp.Header.Get("Content-Type"))
		return
	}
	data, err := a.readStorageBytes(r.Context(), target)
	if err != nil {
		errorJSON(w, 404, "Photo not found")
		return
	}
	a.writeThumbnail(w, r, data, mime.TypeByExtension(strings.ToLower(filepath.Ext(target))))
}

// writeThumbnail mirrors the Nuxt Sharp route: every source is normalized to
// an auto-oriented JPEG, while retaining a readable source as a fallback when
// FFmpeg cannot decode a legacy format.
func (a *App) writeThumbnail(w http.ResponseWriter, r *http.Request, data []byte, sourceType string) {
	mediaCtx, cancel := a.mediaContext(r.Context())
	defer cancel()
	thumbnail, err := ffmpegJPEGContext(mediaCtx, a.cfg.FFmpeg, data)
	if err == nil && len(thumbnail) > 0 {
		data = thumbnail
		sourceType = "image/jpeg"
	}
	if sourceType == "" {
		sourceType = http.DetectContentType(data)
	}
	w.Header().Set("Content-Type", sourceType)
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	_, _ = w.Write(data)
}
func urlPathUnescape(v string) (string, error) { return url.PathUnescape(v) }
func (a *App) serveWeb(w http.ResponseWriter, r *http.Request) {
	webDir := a.cfg.WebDir
	// Older deployments may still copy the generated bundle to ./web. When
	// the default output directory is absent, preserve that layout without
	// requiring a configuration change.
	if _, err := os.Stat(filepath.Join(webDir, "index.html")); err != nil && a.cfg.WebDir == defaultWebDir {
		if _, fallbackErr := os.Stat(filepath.Join("web", "index.html")); fallbackErr == nil {
			webDir = "./web"
		}
	}
	path := r.URL.Path
	if path == "" || path == "/" || filepath.Ext(path) == "" {
		path = "/index.html"
	}
	file := filepath.Join(webDir, filepath.FromSlash(strings.TrimPrefix(path, "/")))
	if !safePath(webDir, file) {
		errorJSON(w, http.StatusNotFound, "Not Found")
		return
	}
	if info, err := os.Stat(file); err == nil && !info.IsDir() {
		if strings.HasPrefix(path, "/_nuxt/") {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		} else {
			w.Header().Set("Cache-Control", "no-cache")
		}
		http.ServeFile(w, r, file)
		return
	}
	index := filepath.Join(webDir, "index.html")
	w.Header().Set("Cache-Control", "no-cache")
	http.ServeFile(w, r, index)
}
