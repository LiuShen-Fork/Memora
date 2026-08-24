package main

import (
	"bytes"
	"fmt"
	"io"
	"mime"
	"net/http"
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
	http.ServeContent(w, r, st.Name(), st.ModTime(), f)
}
func safePath(base, p string) bool {
	base, _ = filepath.Abs(base)
	p, _ = filepath.Abs(p)
	return p == base || strings.HasPrefix(p, base+string(os.PathSeparator))
}
func (a *App) serveImage(w http.ResponseWriter, r *http.Request) {
	key := strings.TrimPrefix(r.URL.Path, "/image/")
	if streamStorage, ok := a.storage.(ReaderStorage); ok {
		reader, object, err := streamStorage.Open(r.Context(), key)
		if err != nil {
			errorJSON(w, http.StatusNotFound, "Photo not found")
			return
		}
		defer reader.Close()
		w.Header().Set("Cache-Control", "public,max-age=31536000,immutable")
		if contentType := mime.TypeByExtension(strings.ToLower(filepath.Ext(key))); contentType != "" {
			w.Header().Set("Content-Type", contentType)
		}
		if seeker, ok := reader.(io.ReadSeeker); ok {
			http.ServeContent(w, r, filepath.Base(key), object.ModTime, seeker)
			return
		}
		if object.Size >= 0 {
			w.Header().Set("Content-Length", fmt.Sprint(object.Size))
		}
		_, _ = io.Copy(w, reader)
		return
	}
	data, err := a.storage.Get(r.Context(), key)
	if err != nil {
		errorJSON(w, 404, "Photo not found")
		return
	}
	w.Header().Set("Cache-Control", "public,max-age=31536000,immutable")
	http.ServeContent(w, r, filepath.Base(key), time.Time{}, bytes.NewReader(data))
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
	if !safePath(a.cfg.WebDir, file) {
		errorJSON(w, http.StatusNotFound, "Not Found")
		return
	}
	if data, err := os.ReadFile(file); err == nil {
		http.ServeContent(w, r, filepath.Base(file), time.Time{}, bytes.NewReader(data))
		return
	}
	index := filepath.Join(a.cfg.WebDir, "index.html")
	http.ServeFile(w, r, index)
}
