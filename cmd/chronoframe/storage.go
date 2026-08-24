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
func (s *LocalStorage) Create(_ context.Context, key string, data []byte, _ string) (Object, error) {
	p, err := s.path(key)
	if err != nil {
		return Object{}, err
	}
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
	return &S3Storage{client: client, bucket: c.S3Bucket, prefix: strings.Trim(c.S3Prefix, "/"), cdn: strings.TrimRight(c.S3CDN, "/")}, nil
}
func (s *S3Storage) Prefix() string      { return s.prefix }
func (s *S3Storage) key(k string) string { return storageKey(s.prefix, k) }
func (s *S3Storage) Create(ctx context.Context, k string, d []byte, ct string) (Object, error) {
	k = s.key(k)
	_, err := s.client.PutObject(ctx, s.bucket, k, bytes.NewReader(d), int64(len(d)), minio.PutObjectOptions{ContentType: ct})
	return Object{Key: k, Size: int64(len(d)), ModTime: time.Now()}, err
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
	return "/image/" + strings.TrimLeft(key, "/")
}
func (s *S3Storage) SignedURL(ctx context.Context, k, ct string) (string, error) {
	u, err := s.client.PresignedPutObject(ctx, s.bucket, s.key(k), time.Hour)
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
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return http.DefaultClient.Do(req)
}
func (s *OpenListStorage) Create(ctx context.Context, k string, d []byte, ct string) (Object, error) {
	endpoint := s.upload
	if endpoint == "" {
		endpoint = "/api/fs/put"
	}
	req, err := http.NewRequestWithContext(ctx, "PUT", strings.TrimRight(s.baseURL, "/")+endpoint, bytes.NewReader(d))
	if err != nil {
		return Object{}, err
	}
	req.Header.Set("Authorization", s.token)
	req.Header.Set("File-Path", url.QueryEscape(s.full(k)))
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
func (s *OpenListStorage) Open(ctx context.Context, k string) (io.ReadCloser, Object, error) {
	endpoint := s.download
	if endpoint == "" {
		endpoint = "/d" + s.full(k)
	} else {
		endpoint += "?" + s.pathField + "=" + url.QueryEscape(s.full(k))
	}
	resp, err := s.request(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, Object{}, err
	}
	if resp.StatusCode >= 300 {
		resp.Body.Close()
		return nil, Object{}, fmt.Errorf("openlist get: %s", resp.Status)
	}
	return resp.Body, Object{Key: storageKey(s.root, k), Size: resp.ContentLength, ModTime: time.Time{}}, nil
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
	resp, err := s.request(ctx, "POST", endpoint, bytes.NewReader(payload))
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
