package main

import (
	"bytes"
	"compress/gzip"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
)

// The generated frontend ships as plain files: a few megabytes of JavaScript,
// the stylesheet and the icons. They are served straight from disk, which means
// without compression a first visit transfers every byte of the bundle as it
// sits on disk — and the browser that comes back asks for it all again unless
// the response says it may keep it.

// assetCacheLimit bounds the compressed copies held in memory. The whole
// bundle compresses to under two megabytes, so the cap only matters if a
// deployment serves a much larger frontend; when it is reached the cache is
// emptied rather than grown, because a second read of the disk is cheaper than
// an unbounded map.
const assetCacheLimit = 64 << 20

// minCompressibleSize keeps tiny files out of the cache. The headers a
// compressed response needs cost about as much as the bodies it saves.
const minCompressibleSize = 1024

// compressibleAssetTypes maps the frontend's text assets to the exact content
// types Go should answer with. mime.TypeByExtension consults the Windows
// registry, which does not always know the web's newer types, and the Go
// backend runs on Windows too.
var compressibleAssetTypes = map[string]string{
	".css":         "text/css; charset=utf-8",
	".html":        "text/html; charset=utf-8",
	".js":          "text/javascript; charset=utf-8",
	".json":        "application/json; charset=utf-8",
	".map":         "application/json; charset=utf-8",
	".mjs":         "text/javascript; charset=utf-8",
	".svg":         "image/svg+xml",
	".txt":         "text/plain; charset=utf-8",
	".webmanifest": "application/manifest+json",
	".xml":         "application/xml; charset=utf-8",
}

type staticAssetEntry struct {
	size    int64
	modTime int64
	body    []byte
}

type staticAssetCache struct {
	mu      sync.Mutex
	entries map[string]staticAssetEntry
	total   int64
}

var staticAssets = &staticAssetCache{entries: map[string]staticAssetEntry{}}

// compressed answers with the gzipped form of the file, reusing the copy kept
// from an earlier request while the file on disk is unchanged.
func (c *staticAssetCache) compressed(file string, info os.FileInfo) ([]byte, error) {
	size, modTime := info.Size(), info.ModTime().UnixNano()
	c.mu.Lock()
	entry, ok := c.entries[file]
	c.mu.Unlock()
	if ok && entry.size == size && entry.modTime == modTime {
		return entry.body, nil
	}
	raw, err := os.ReadFile(file)
	if err != nil {
		return nil, err
	}
	var buffer bytes.Buffer
	writer, err := gzip.NewWriterLevel(&buffer, gzip.BestCompression)
	if err != nil {
		return nil, err
	}
	if _, err := writer.Write(raw); err != nil {
		return nil, err
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}
	body := buffer.Bytes()
	c.mu.Lock()
	if previous, ok := c.entries[file]; ok {
		c.total -= int64(len(previous.body))
	}
	if c.total+int64(len(body)) > assetCacheLimit {
		c.entries = map[string]staticAssetEntry{}
		c.total = 0
	}
	c.entries[file] = staticAssetEntry{size: size, modTime: modTime, body: body}
	c.total += int64(len(body))
	c.mu.Unlock()
	return body, nil
}

// serveStatic answers one file from the generated frontend. Build assets carry
// a content hash in their name, so they never change under the same address and
// the browser may keep them for as long as it likes; the remaining files (the
// icons and the manifest) keep their name across builds and get a short life.
func (a *App) serveStatic(w http.ResponseWriter, r *http.Request, file string, info os.FileInfo) {
	if strings.HasPrefix(r.URL.Path, "/_nuxt/") {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	} else {
		w.Header().Set("Cache-Control", "public, max-age=3600")
	}
	// The same address answers differently depending on this header, and a
	// shared cache in front of the backend must be told so.
	w.Header().Add("Vary", "Accept-Encoding")

	extension := strings.ToLower(filepath.Ext(file))
	contentType, compressible := compressibleAssetTypes[extension]
	if compressible && info.Size() >= minCompressibleSize && acceptsGzip(r) {
		if body, err := staticAssets.compressed(file, info); err == nil {
			etag := `W/"` + strconv.FormatInt(info.Size(), 10) + "-" + strconv.FormatInt(info.ModTime().UnixNano(), 10) + `-gz"`
			w.Header().Set("ETag", etag)
			if r.Header.Get("If-None-Match") == etag {
				w.WriteHeader(http.StatusNotModified)
				return
			}
			w.Header().Set("Content-Type", contentType)
			w.Header().Set("Content-Encoding", "gzip")
			w.Header().Set("Content-Length", strconv.Itoa(len(body)))
			w.Header().Set("Last-Modified", info.ModTime().UTC().Format(http.TimeFormat))
			w.WriteHeader(http.StatusOK)
			if r.Method != http.MethodHead {
				_, _ = w.Write(body)
			}
			return
		}
	}
	http.ServeFile(w, r, file)
}
