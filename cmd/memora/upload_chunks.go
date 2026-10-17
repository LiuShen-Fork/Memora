package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/minio/minio-go/v7"
)

// uploadPartSize is how much of a file travels in one request when an upload is
// sent in parts, and the smallest part an object store that caps them accepts.
// Files smaller than this are not worth splitting, so they keep the single
// request every earlier version used.
const uploadPartSize = 8 << 20

// maxUploadParts keeps a plan inside what the object stores allow; a file that
// would need more parts than this is sent in one request instead.
const maxUploadParts = 10000

// uploadSessionTTL is how long an upload that stopped sending parts is kept
// before its temporary file is thrown away.
const uploadSessionTTL = time.Hour

// uploadSession is one file on its way in. A session that receives parts holds
// them in a temporary file the parts are written into at their own offsets; a
// session that goes straight to an object store holds the upload id its parts
// belong to.
type uploadSession struct {
	mu          sync.Mutex
	id          string
	owner       int64
	key         string
	contentType string
	size        int64
	partSize    int64
	parts       int
	received    map[int]string
	active      map[int]bool
	completing  bool
	completed   bool
	cancelled   bool
	touched     time.Time
	s3UploadID  string
	dir         string
	path        string
}

// partLength is how many bytes part index carries. Only the last part of a file
// that does not divide evenly is short.
func (s *uploadSession) partLength(index int) int64 {
	if index == s.parts-1 {
		return s.size - int64(index)*s.partSize
	}
	return s.partSize
}

func newUploadID() string {
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return strconv.FormatInt(time.Now().UnixNano(), 36)
	}
	return hex.EncodeToString(raw)
}

func (a *App) putUploadSession(session *uploadSession) {
	a.uploadsMu.Lock()
	defer a.uploadsMu.Unlock()
	if a.uploads == nil {
		a.uploads = map[string]*uploadSession{}
	}
	a.uploads[session.id] = session
}

func (a *App) uploadSessionByID(id string) (*uploadSession, bool) {
	a.uploadsMu.Lock()
	defer a.uploadsMu.Unlock()
	session, ok := a.uploads[id]
	return session, ok
}

func (a *App) dropUploadSession(id string) {
	a.uploadsMu.Lock()
	defer a.uploadsMu.Unlock()
	delete(a.uploads, id)
}

// uploadSessionFor reads the session a request names, and refuses one that
// belongs to somebody else rather than telling the caller it exists.
func (a *App) uploadSessionFor(w http.ResponseWriter, r *http.Request, rest string) (*uploadSession, bool) {
	user, ok := a.require(r)
	if !ok {
		errorJSON(w, http.StatusUnauthorized, "Unauthorized")
		return nil, false
	}
	id := strings.Trim(rest, "/")
	session, found := a.uploadSessionByID(id)
	if !found || id == "" || strings.ContainsAny(id, "/\\") || session.owner != userID(user) {
		errorJSON(w, http.StatusNotFound, "Upload session not found")
		return nil, false
	}
	return session, true
}

func userID(user map[string]any) int64 {
	id, _ := user["id"].(int64)
	return id
}

// uploadPlanFor says how a file of this size should travel. A missing plan
// means the single signed URL the response already carries, which is what every
// upload used before this and what a provider that cannot take parts still
// gets.
func (a *App) uploadPlanFor(ctx context.Context, owner int64, key, contentType string, size int64) map[string]any {
	// The size limit is a property of the site, not of the way a file is sent,
	// so a file too large to accept is not offered a way around it.
	if size <= uploadPartSize || size > a.maxUploadBytes() {
		return nil
	}
	parts := int((size + uploadPartSize - 1) / uploadPartSize)
	if parts > maxUploadParts {
		return nil
	}
	a.sweepUploads(time.Now())
	if storage, ok := a.storage.(*S3Storage); ok {
		return a.s3MultipartPlan(ctx, storage, owner, key, contentType, size, parts)
	}
	return a.chunkedPlan(owner, key, contentType, size, parts)
}

// chunkedPlan opens a session whose parts this server receives and joins back
// together.
func (a *App) chunkedPlan(owner int64, key, contentType string, size int64, parts int) map[string]any {
	id := newUploadID()
	dir := filepath.Join(a.cfg.DataDir, "uploads", id)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		a.logs.Add("storage", "starting a chunked upload: "+err.Error())
		return nil
	}
	path := filepath.Join(dir, "file")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		_ = os.RemoveAll(dir)
		a.logs.Add("storage", "starting a chunked upload: "+err.Error())
		return nil
	}
	_ = file.Close()
	a.putUploadSession(&uploadSession{
		id:          id,
		owner:       owner,
		key:         key,
		contentType: contentType,
		size:        size,
		partSize:    uploadPartSize,
		parts:       parts,
		received:    map[int]string{},
		active:      map[int]bool{},
		touched:     time.Now(),
		dir:         dir,
		path:        path,
	})
	return map[string]any{
		"mode":      "chunks",
		"url":       "/api/photos/chunks/" + id,
		"partSize":  uploadPartSize,
		"partCount": parts,
	}
}

// s3MultipartPlan starts a multipart upload on the object store and hands back
// one signed URL per part, so the parts travel from the browser straight to the
// store instead of through this server.
func (a *App) s3MultipartPlan(ctx context.Context, storage *S3Storage, owner int64, key, contentType string, size int64, parts int) map[string]any {
	object := storage.key(key)
	uploadID, err := storage.core.NewMultipartUpload(ctx, storage.bucket, object, minio.PutObjectOptions{ContentType: contentType})
	if err != nil {
		a.logs.Add("storage", "starting a multipart upload: "+err.Error())
		return nil
	}
	urls := make([]string, 0, parts)
	for index := 0; index < parts; index++ {
		params := url.Values{"uploadId": {uploadID}, "partNumber": {strconv.Itoa(index + 1)}}
		signed, signErr := storage.client.PresignHeader(ctx, http.MethodPut, storage.bucket, object, time.Hour, params, nil)
		if signErr != nil {
			a.logs.Add("storage", "signing multipart parts: "+signErr.Error())
			_ = storage.core.AbortMultipartUpload(context.Background(), storage.bucket, object, uploadID)
			return nil
		}
		urls = append(urls, signed.String())
	}
	id := newUploadID()
	a.putUploadSession(&uploadSession{
		id:          id,
		owner:       owner,
		key:         key,
		contentType: contentType,
		size:        size,
		partSize:    uploadPartSize,
		parts:       parts,
		received:    map[int]string{},
		active:      map[int]bool{},
		touched:     time.Now(),
		s3UploadID:  uploadID,
	})
	return map[string]any{
		"mode":      "multipart",
		"url":       "/api/photos/multipart/" + id,
		"partSize":  uploadPartSize,
		"partCount": parts,
		"partUrls":  urls,
	}
}

// chunkUploadRoute receives the parts of an upload this server is holding, and
// puts the file into storage once they have all arrived.
func (a *App) chunkUploadRoute(w http.ResponseWriter, r *http.Request, rest string) {
	session, ok := a.uploadSessionFor(w, r, rest)
	if !ok {
		return
	}
	switch r.Method {
	case http.MethodPut:
		a.receiveUploadPart(w, r, session)
	case http.MethodPost:
		a.completeChunkUpload(w, r, session)
	case http.MethodDelete:
		a.abortUpload(w, session)
	default:
		errorJSON(w, http.StatusMethodNotAllowed, "Method Not Allowed")
	}
}

// receiveUploadPart writes one part at its own offset. Parts arrive in any
// order and several at a time, which is the point of sending a large file this
// way.
func (a *App) receiveUploadPart(w http.ResponseWriter, r *http.Request, session *uploadSession) {
	index, err := strconv.Atoi(r.URL.Query().Get("index"))
	if err != nil || index < 0 || index >= session.parts {
		errorJSON(w, http.StatusBadRequest, "Invalid chunk index")
		return
	}
	session.mu.Lock()
	cancelled, completed, active := session.cancelled, session.completed || session.completing, session.active[index]
	if !cancelled && !completed && !active {
		session.active[index] = true
		session.touched = time.Now()
	}
	session.mu.Unlock()
	if cancelled {
		errorJSON(w, http.StatusGone, "Upload cancelled")
		return
	}
	if completed {
		errorJSON(w, http.StatusConflict, "Upload already completed")
		return
	}
	if active {
		errorJSON(w, http.StatusConflict, "Chunk is busy")
		return
	}
	defer func() {
		session.mu.Lock()
		delete(session.active, index)
		session.touched = time.Now()
		session.mu.Unlock()
	}()

	expected := session.partLength(index)
	if r.ContentLength > expected {
		errorJSON(w, http.StatusRequestEntityTooLarge, "Chunk too large")
		return
	}
	// The part is read whole so its content can be checked against a retry of
	// the same part before anything is written.
	data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, expected))
	if err != nil {
		errorJSON(w, http.StatusRequestEntityTooLarge, "Chunk too large")
		return
	}
	if int64(len(data)) != expected {
		errorJSON(w, http.StatusBadRequest, "Invalid chunk size")
		return
	}
	digest := sha256.Sum256(data)
	hexDigest := hex.EncodeToString(digest[:])
	session.mu.Lock()
	previous, seen := session.received[index]
	session.mu.Unlock()
	if seen {
		if previous != hexDigest {
			errorJSON(w, http.StatusConflict, "Chunk content mismatch")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
		return
	}
	file, err := os.OpenFile(session.path, os.O_WRONLY, 0o644)
	if err != nil {
		a.logs.Add("storage", "writing a part: "+err.Error())
		errorJSON(w, http.StatusInternalServerError, "Unable to store the upload")
		return
	}
	_, writeErr := file.WriteAt(data, int64(index)*session.partSize)
	closeErr := file.Close()
	if writeErr != nil || closeErr != nil {
		a.logs.Add("storage", "writing a part: "+errorText(writeErr, closeErr))
		errorJSON(w, http.StatusInternalServerError, "Unable to store the upload")
		return
	}
	session.mu.Lock()
	session.received[index] = hexDigest
	session.mu.Unlock()
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// completeChunkUpload joins the parts into the file the upload was prepared
// for. It refuses while a part is still arriving, because the last part to
// arrive is not necessarily the last one the file needs.
func (a *App) completeChunkUpload(w http.ResponseWriter, r *http.Request, session *uploadSession) {
	session.mu.Lock()
	cancelled, completed, busy := session.cancelled, session.completed, session.completing || len(session.active) > 0
	received := len(session.received)
	if !cancelled && !completed && !busy {
		session.completing = true
	}
	session.mu.Unlock()
	if cancelled {
		errorJSON(w, http.StatusGone, "Upload cancelled")
		return
	}
	if completed {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "key": session.key})
		return
	}
	if busy {
		errorJSON(w, http.StatusConflict, "Upload is busy")
		return
	}
	defer func() {
		session.mu.Lock()
		session.completing = false
		session.touched = time.Now()
		session.mu.Unlock()
	}()

	if received != session.parts {
		errorJSON(w, http.StatusBadRequest, "Upload is incomplete")
		return
	}
	if err := a.storeChunkedFile(r.Context(), session); err != nil {
		if !errors.Is(err, errIncompleteUpload) {
			a.logs.Add("storage", err.Error())
		}
		errorJSON(w, http.StatusBadRequest, err.Error())
		return
	}
	session.mu.Lock()
	session.completed = true
	session.mu.Unlock()
	a.dropUploadSession(session.id)
	releaseUploadFiles(session)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "key": session.key})
}

// errIncompleteUpload is an upload whose parts are not all there yet, which the
// caller can fix by sending the rest.
var errIncompleteUpload = errors.New("Upload is incomplete")

// storeChunkedFile hands the assembled file to storage. The file is closed
// before the temporary directory is removed, because Windows will not remove a
// file another handle still holds open.
func (a *App) storeChunkedFile(ctx context.Context, session *uploadSession) error {
	writer, ok := a.storage.(ReaderWriterStorage)
	if !ok {
		return errors.New("Storage cannot receive this upload")
	}
	file, err := os.Open(session.path)
	if err != nil {
		return errIncompleteUpload
	}
	info, statErr := file.Stat()
	if statErr != nil || info.Size() != session.size {
		_ = file.Close()
		return errIncompleteUpload
	}
	_, err = writer.CreateReader(ctx, session.key, file, session.size, session.contentType)
	_ = file.Close()
	return err
}

// multipartUploadRoute finishes an upload whose parts went straight to the
// object store.
func (a *App) multipartUploadRoute(w http.ResponseWriter, r *http.Request, rest string) {
	session, ok := a.uploadSessionFor(w, r, rest)
	if !ok {
		return
	}
	switch r.Method {
	case http.MethodPost:
		a.completeMultipartUpload(w, r, session)
	case http.MethodDelete:
		a.abortUpload(w, session)
	default:
		errorJSON(w, http.StatusMethodNotAllowed, "Method Not Allowed")
	}
}

// completeMultipartUpload asks the object store to join the parts it has been
// given. The parts are read back from the store rather than taken from the
// caller, so an upload completes even when the browser is not allowed to read
// the ETag of the part it just sent.
func (a *App) completeMultipartUpload(w http.ResponseWriter, r *http.Request, session *uploadSession) {
	storage, ok := a.storage.(*S3Storage)
	if !ok || session.s3UploadID == "" {
		errorJSON(w, http.StatusNotFound, "Upload session not found")
		return
	}
	session.mu.Lock()
	cancelled, completed, busy := session.cancelled, session.completed, session.completing
	if !cancelled && !completed && !busy {
		session.completing = true
	}
	session.mu.Unlock()
	if cancelled {
		errorJSON(w, http.StatusGone, "Upload cancelled")
		return
	}
	if completed {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "key": session.key})
		return
	}
	if busy {
		errorJSON(w, http.StatusConflict, "Upload is busy")
		return
	}
	defer func() {
		session.mu.Lock()
		session.completing = false
		session.touched = time.Now()
		session.mu.Unlock()
	}()

	object := storage.key(session.key)
	listed, err := storage.core.ListObjectParts(r.Context(), storage.bucket, object, session.s3UploadID, 0, maxUploadParts)
	if err != nil {
		a.logs.Add("storage", err.Error())
		errorJSON(w, http.StatusInternalServerError, err.Error())
		return
	}
	if len(listed.ObjectParts) != session.parts {
		errorJSON(w, http.StatusBadRequest, "Upload is incomplete")
		return
	}
	parts := make([]minio.CompletePart, 0, len(listed.ObjectParts))
	for _, part := range listed.ObjectParts {
		parts = append(parts, minio.CompletePart{PartNumber: part.PartNumber, ETag: part.ETag})
	}
	if _, err := storage.core.CompleteMultipartUpload(r.Context(), storage.bucket, object, session.s3UploadID, parts, minio.PutObjectOptions{ContentType: session.contentType}); err != nil {
		a.logs.Add("storage", err.Error())
		errorJSON(w, http.StatusInternalServerError, err.Error())
		return
	}
	session.mu.Lock()
	session.completed = true
	session.mu.Unlock()
	a.dropUploadSession(session.id)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "key": session.key})
}

// abortUpload throws away an upload in progress: the temporary file it was
// being written into, and the parts an object store is holding for it.
func (a *App) abortUpload(w http.ResponseWriter, session *uploadSession) {
	session.mu.Lock()
	session.cancelled = true
	uploadID, key, completed, dir := session.s3UploadID, session.key, session.completed, session.dir
	session.mu.Unlock()
	a.dropUploadSession(session.id)
	if dir != "" && !completed {
		releaseUploadFiles(session)
	}
	if uploadID != "" && !completed {
		if storage, ok := a.storage.(*S3Storage); ok {
			if err := storage.core.AbortMultipartUpload(context.Background(), storage.bucket, storage.key(key), uploadID); err != nil {
				a.logs.Add("storage", "aborting a multipart upload: "+err.Error())
			}
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// releaseUploadFiles removes the directory an upload was assembled in. A part
// still being written holds the file open on Windows, which leaves the
// directory for the sweep to collect later.
func releaseUploadFiles(session *uploadSession) {
	if session.dir == "" {
		return
	}
	if err := os.RemoveAll(session.dir); err != nil {
		session.mu.Lock()
		session.touched = time.Now()
		session.mu.Unlock()
	}
}

// sweepUploads drops the uploads nobody is sending any more, and the temporary
// directories a previous process left behind. It runs when a new upload
// arrives, which is often enough to keep a server that only ever receives
// uploads tidy, and costs nothing on one that does not.
func (a *App) sweepUploads(now time.Time) {
	a.uploadsMu.Lock()
	stale := make([]*uploadSession, 0)
	for id, session := range a.uploads {
		session.mu.Lock()
		expired := now.Sub(session.touched) > uploadSessionTTL && !session.completing && len(session.active) == 0
		session.mu.Unlock()
		if expired {
			delete(a.uploads, id)
			stale = append(stale, session)
		}
	}
	a.uploadsMu.Unlock()
	for _, session := range stale {
		session.mu.Lock()
		uploadID, key := session.s3UploadID, session.key
		session.cancelled = true
		session.mu.Unlock()
		releaseUploadFiles(session)
		if uploadID != "" {
			if storage, ok := a.storage.(*S3Storage); ok {
				if err := storage.core.AbortMultipartUpload(context.Background(), storage.bucket, storage.key(key), uploadID); err != nil {
					a.logs.Add("storage", "aborting a multipart upload: "+err.Error())
				}
			}
		}
	}
	a.sweepUploadDirs(now)
}

// sweepUploadDirs removes directories in the uploads area that no session
// claims, which is what a process that stopped mid-upload leaves behind.
func (a *App) sweepUploadDirs(now time.Time) {
	root := filepath.Join(a.cfg.DataDir, "uploads")
	entries, err := os.ReadDir(root)
	if err != nil {
		return
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if _, live := a.uploadSessionByID(entry.Name()); live {
			continue
		}
		dir := filepath.Join(root, entry.Name())
		info, err := os.Stat(filepath.Join(dir, "file"))
		if err != nil {
			info, err = os.Stat(dir)
		}
		if err != nil || now.Sub(info.ModTime()) <= uploadSessionTTL {
			continue
		}
		if err := os.RemoveAll(dir); err != nil {
			a.logs.Add("storage", fmt.Sprintf("removing the abandoned upload %s: %v", entry.Name(), err))
		}
	}
}

func errorText(errs ...error) string {
	for _, err := range errs {
		if err != nil {
			return err.Error()
		}
	}
	return "unknown error"
}
