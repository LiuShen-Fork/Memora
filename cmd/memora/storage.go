package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type Object struct {
	Key     string
	Size    int64
	ModTime time.Time
}

var storageHTTPClient = &http.Client{Timeout: 2 * time.Minute}
var externalHTTPClient = &http.Client{Timeout: 30 * time.Second}

// ErrStorageNotFound identifies a provider response that definitively says
// the object no longer exists. Callers can clean stale database references
// without treating transient provider failures as missing media.
var ErrStorageNotFound = errors.New("storage object not found")

type Storage interface {
	Create(context.Context, string, []byte, string) (Object, error)
	Get(context.Context, string) ([]byte, error)
	Delete(context.Context, string) error
	Meta(context.Context, string) (Object, error)
	PublicURL(string) string
	SignedURL(context.Context, string, string) (string, error)
	Prefix() string
}

type ReaderStorage interface {
	Open(context.Context, string) (io.ReadCloser, Object, error)
}

type ReaderWriterStorage interface {
	CreateReader(context.Context, string, io.Reader, int64, string) (Object, error)
}

type LocalStorage struct{ base, baseURL, prefix string }

func (s *LocalStorage) Prefix() string { return strings.Trim(s.prefix, "/") }
func (s *LocalStorage) Open(_ context.Context, key string) (io.ReadCloser, Object, error) {
	p, err := s.path(key)
	if err != nil {
		return nil, Object{}, err
	}
	file, err := os.Open(p)
	if err != nil {
		return nil, Object{}, err
	}
	info, err := file.Stat()
	if err != nil {
		file.Close()
		return nil, Object{}, err
	}
	return file, Object{Key: storageKey(s.prefix, key), Size: info.Size(), ModTime: info.ModTime()}, nil
}
func (s *LocalStorage) path(key string) (string, error) {
	rawKey := strings.TrimLeft(strings.ReplaceAll(key, "\\", "/"), "/")
	cleanedKey := filepath.Clean(filepath.FromSlash(rawKey))
	if rawKey == "" || cleanedKey == "." || cleanedKey == ".." || strings.HasPrefix(cleanedKey, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("invalid storage key")
	}
	key = strings.TrimLeft(storageKey(s.prefix, filepath.ToSlash(cleanedKey)), "/")
	return filepath.Join(s.base, filepath.FromSlash(key)), nil
}
func (s *LocalStorage) Create(ctx context.Context, key string, data []byte, contentType string) (Object, error) {
	return s.CreateReader(ctx, key, bytes.NewReader(data), int64(len(data)), contentType)
}
func (s *LocalStorage) CreateReader(_ context.Context, key string, reader io.Reader, size int64, _ string) (Object, error) {
	p, err := s.path(key)
	if err != nil {
		return Object{}, err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
		return Object{}, err
	}
	tmp := p + fmt.Sprintf(".tmp-%d", time.Now().UnixNano())
	file, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return Object{}, err
	}
	written, copyErr := io.Copy(file, reader)
	closeErr := file.Close()
	if copyErr != nil {
		_ = os.Remove(tmp)
		return Object{}, copyErr
	}
	if closeErr != nil {
		_ = os.Remove(tmp)
		return Object{}, closeErr
	}
	if size >= 0 && written != size {
		_ = os.Remove(tmp)
		return Object{}, fmt.Errorf("short storage write: wrote %d of %d bytes", written, size)
	}
	if err := os.Chmod(tmp, 0644); err != nil {
		_ = os.Remove(tmp)
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
	p, err := s.path(key)
	if err != nil {
		return nil, err
	}
	return os.ReadFile(p)
}
func (s *LocalStorage) Delete(_ context.Context, key string) error {
	p, err := s.path(key)
	if err != nil {
		return err
	}
	err = os.Remove(p)
	if os.IsNotExist(err) {
		return nil
	}
	return err
}
func (s *LocalStorage) Meta(_ context.Context, key string) (Object, error) {
	p, err := s.path(key)
	if err != nil {
		return Object{}, err
	}
	st, err := os.Stat(p)
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
func urlQueryEscape(v string) string { return url.QueryEscape(v) }

type S3Storage struct {
	client              *minio.Client
	bucket, prefix, cdn string
	endpoint, region    string
}

func NewS3Storage(c Config) (*S3Storage, error) {
	endpoint := strings.TrimPrefix(strings.TrimPrefix(c.S3Endpoint, "https://"), "http://")
	secure := strings.HasPrefix(c.S3Endpoint, "https://")
	clientOptions := minio.Options{Creds: credentials.NewStaticV4(c.S3AccessKey, c.S3SecretKey, c.S3Region), Secure: secure, Region: c.S3Region}
	if c.S3PathStyle {
		clientOptions.BucketLookup = minio.BucketLookupPath
	}
	client, err := minio.New(endpoint, &clientOptions)
	if err != nil {
		return nil, err
	}
	return &S3Storage{client: client, bucket: c.S3Bucket, prefix: strings.Trim(c.S3Prefix, "/"), cdn: strings.TrimRight(c.S3CDN, "/"), endpoint: c.S3Endpoint, region: c.S3Region}, nil
}
func (s *S3Storage) Prefix() string      { return s.prefix }
func (s *S3Storage) key(k string) string { return storageKey(s.prefix, k) }
func (s *S3Storage) Create(ctx context.Context, k string, d []byte, ct string) (Object, error) {
	return s.CreateReader(ctx, k, bytes.NewReader(d), int64(len(d)), ct)
}
func (s *S3Storage) CreateReader(ctx context.Context, k string, reader io.Reader, size int64, ct string) (Object, error) {
	k = s.key(k)
	_, err := s.client.PutObject(ctx, s.bucket, k, reader, size, minio.PutObjectOptions{ContentType: ct})
	return Object{Key: k, Size: size, ModTime: time.Now()}, err
}
func (s *S3Storage) Open(ctx context.Context, k string) (io.ReadCloser, Object, error) {
	key := s.key(k)
	reader, err := s.client.GetObject(ctx, s.bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return nil, Object{}, err
	}
	meta, err := s.client.StatObject(ctx, s.bucket, key, minio.StatObjectOptions{})
	if err != nil {
		reader.Close()
		return nil, Object{}, err
	}
	return reader, Object{Key: key, Size: meta.Size, ModTime: meta.LastModified}, nil
}
func (s *S3Storage) Get(ctx context.Context, k string) ([]byte, error) {
	o, _, err := s.Open(ctx, k)
	if err != nil {
		return nil, err
	}
	defer o.Close()
	return io.ReadAll(o)
}
func (s *S3Storage) Delete(ctx context.Context, k string) error {
	return s.client.RemoveObject(ctx, s.bucket, s.key(k), minio.RemoveObjectOptions{})
}
func (s *S3Storage) Meta(ctx context.Context, k string) (Object, error) {
	key := s.key(k)
	st, err := s.client.StatObject(ctx, s.bucket, key, minio.StatObjectOptions{})
	return Object{Key: key, Size: st.Size, ModTime: st.LastModified}, err
}
func (s *S3Storage) PublicURL(k string) string {
	key := s.key(k)
	if s.cdn != "" {
		return s.cdn + "/" + key
	}
	endpoint := strings.TrimRight(s.endpoint, "/")
	if endpoint == "" || strings.Contains(endpoint, "amazonaws.com") {
		return fmt.Sprintf("https://%s.s3.%s.amazonaws.com/%s", s.bucket, s.region, key)
	}
	if strings.Contains(endpoint, "aliyuncs.com") {
		if !strings.Contains(endpoint, "://") {
			return ""
		}
		parts := strings.SplitN(endpoint, "://", 2)
		return parts[0] + "://" + s.bucket + "." + parts[1] + "/" + key
	}
	return endpoint + "/" + s.bucket + "/" + key
}
func (s *S3Storage) SignedURL(ctx context.Context, k, ct string) (string, error) {
	u, err := s.client.PresignedPutObject(ctx, s.bucket, s.key(k), time.Hour)
	return u.String(), err
}

type OpenListStorage struct{ baseURL, root, token, upload, download, list, delete, meta, pathField, cdn string }

func (s *OpenListStorage) Prefix() string { return strings.Trim(s.root, "/") }
func (s *OpenListStorage) pathFieldName() string {
	if name := strings.TrimSpace(s.pathField); name != "" {
		return name
	}
	return "path"
}
func (s *OpenListStorage) full(k string) string {
	return "/" + strings.Trim(storageKey(s.root, k), "/")
}
func openListPath(path string) string {
	parts := strings.Split(path, "/")
	for i, part := range parts {
		parts[i] = url.PathEscape(part)
	}
	return strings.Join(parts, "/")
}
func (s *OpenListStorage) request(ctx context.Context, method, path string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(s.baseURL, "/")+path, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", s.token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return storageHTTPClient.Do(req)
}
func (s *OpenListStorage) Create(ctx context.Context, k string, d []byte, ct string) (Object, error) {
	return s.CreateReader(ctx, k, bytes.NewReader(d), int64(len(d)), ct)
}
func (s *OpenListStorage) CreateReader(ctx context.Context, k string, reader io.Reader, size int64, ct string) (Object, error) {
	endpoint := s.upload
	if endpoint == "" {
		endpoint = "/api/fs/put"
	}
	req, err := http.NewRequestWithContext(ctx, "PUT", strings.TrimRight(s.baseURL, "/")+endpoint, reader)
	if err != nil {
		return Object{}, err
	}
	req.Header.Set("Authorization", s.token)
	// OpenList unescapes this header once. Encode each segment so separators
	// remain path separators while reserved characters in filenames survive.
	req.Header.Set("File-Path", openListPath(s.full(k)))
	req.Header.Set("Content-Type", ct)
	if size >= 0 {
		req.ContentLength = size
	}
	resp, err := storageHTTPClient.Do(req)
	if err != nil {
		return Object{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return Object{}, s.responseError("upload", s.full(k), resp)
	}
	return Object{Key: storageKey(s.root, k), Size: size, ModTime: time.Now()}, nil
}
func (s *OpenListStorage) Open(ctx context.Context, k string) (io.ReadCloser, Object, error) {
	endpoint := s.download
	if endpoint == "" {
		endpoint = "/d" + s.full(k)
	} else {
		endpoint += "?" + url.QueryEscape(s.pathFieldName()) + "=" + url.QueryEscape(s.full(k))
	}
	resp, err := s.request(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, Object{}, err
	}
	if resp.StatusCode >= 300 {
		err := s.responseError("get", s.full(k), resp)
		_ = resp.Body.Close()
		if s.download == "" && !errors.Is(err, ErrStorageNotFound) {
			rawURL, rawErr := s.rawURL(ctx, k)
			if rawErr != nil {
				err = fmt.Errorf("%w; raw_url lookup: %v", err, rawErr)
			} else if rawReader, rawResp, fetchErr := s.openRawURL(ctx, rawURL); fetchErr == nil {
				return rawReader, Object{Key: storageKey(s.root, k), Size: rawResp.ContentLength, ModTime: time.Time{}}, nil
			} else {
				err = fmt.Errorf("%w; raw_url fallback: %v", err, fetchErr)
			}
		}
		return nil, Object{}, err
	}
	return resp.Body, Object{Key: storageKey(s.root, k), Size: resp.ContentLength, ModTime: time.Time{}}, nil
}

func (s *OpenListStorage) openRawURL(ctx context.Context, rawURL string) (io.ReadCloser, *http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, nil, err
	}
	// OpenList may return an authenticated API URL instead of a public link.
	// Only forward the token to the configured OpenList host, never to a
	// third-party URL returned by a storage driver.
	if target, targetErr := url.Parse(rawURL); targetErr == nil {
		if base, baseErr := url.Parse(strings.TrimRight(s.baseURL, "/")); baseErr == nil &&
			strings.EqualFold(target.Scheme, base.Scheme) && strings.EqualFold(target.Host, base.Host) {
			req.Header.Set("Authorization", s.token)
		}
	}
	resp, err := storageHTTPClient.Do(req)
	if err != nil {
		return nil, nil, err
	}
	if resp.StatusCode >= 300 {
		err := s.responseError("raw_url", rawURL, resp)
		_ = resp.Body.Close()
		return nil, nil, err
	}
	return resp.Body, resp, nil
}

func (s *OpenListStorage) rawURL(ctx context.Context, k string) (string, error) {
	endpoint := s.meta
	if endpoint == "" {
		endpoint = "/api/fs/get"
	}
	payload, _ := json.Marshal(map[string]any{s.pathFieldName(): s.full(k), "password": "", "page": 1, "per_page": 0, "refresh": false})
	resp, err := s.request(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return "", s.responseError("meta", s.full(k), resp)
	}
	var body struct {
		Data struct {
			RawURL string `json:"raw_url"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return "", fmt.Errorf("openlist meta decode: %w", err)
	}
	if strings.TrimSpace(body.Data.RawURL) == "" {
		return "", errors.New("openlist meta: response did not include raw_url")
	}
	rawURL, err := url.Parse(body.Data.RawURL)
	if err != nil {
		return "", fmt.Errorf("openlist meta raw_url: %w", err)
	}
	if !rawURL.IsAbs() {
		base, err := url.Parse(strings.TrimRight(s.baseURL, "/"))
		if err != nil {
			return "", fmt.Errorf("openlist base URL: %w", err)
		}
		rawURL = base.ResolveReference(rawURL)
	}
	return rawURL.String(), nil
}

func (s *OpenListStorage) responseError(operation, path string, resp *http.Response) error {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	message := strings.TrimSpace(string(body))
	if message == "" {
		message = "no response body"
	}
	log.Printf("[storage/openlist] %s %s failed: %s (%s)", operation, path, resp.Status, message)
	lowerMessage := strings.ToLower(message)
	if resp.StatusCode == http.StatusNotFound || strings.Contains(lowerMessage, "filenotfound") || strings.Contains(lowerMessage, "object not found") || strings.Contains(lowerMessage, "file is notfound") {
		return fmt.Errorf("%w: openlist %s: %s: %s", ErrStorageNotFound, operation, resp.Status, message)
	}
	return fmt.Errorf("openlist %s: %s: %s", operation, resp.Status, message)
}
func (s *OpenListStorage) Get(ctx context.Context, k string) ([]byte, error) {
	reader, _, err := s.Open(ctx, k)
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	return io.ReadAll(reader)
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
	resp, err := s.request(ctx, "POST", endpoint, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return s.responseError("delete", s.full(k), resp)
	}
	return nil
}
func (s *OpenListStorage) Meta(ctx context.Context, k string) (Object, error) {
	endpoint := s.meta
	if endpoint == "" {
		endpoint = "/api/fs/get"
	}
	payload, _ := json.Marshal(map[string]any{s.pathFieldName(): s.full(k), "password": "", "page": 1, "per_page": 0, "refresh": false})
	resp, err := s.request(ctx, "POST", endpoint, bytes.NewReader(payload))
	if err != nil {
		return Object{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return Object{}, s.responseError("meta", s.full(k), resp)
	}
	var body struct {
		Data struct {
			Size     int64  `json:"size"`
			Modified string `json:"modified"`
			RawURL   string `json:"raw_url"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return Object{}, fmt.Errorf("openlist meta decode: %w", err)
	}
	return Object{Key: storageKey(s.root, k), Size: body.Data.Size, ModTime: parseTime(body.Data.Modified)}, nil
}
func parseTime(value string) time.Time {
	value = strings.ReplaceAll(strings.TrimSpace(value), "\u202f", " ")
	for _, layout := range []string{
		time.RFC3339,
		"Jan 2, 2006, 3:04:05 PM -07",
		"Jan 2, 2006, 3:04:05 PM MST",
	} {
		if t, err := time.Parse(layout, value); err == nil {
			return t
		}
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
