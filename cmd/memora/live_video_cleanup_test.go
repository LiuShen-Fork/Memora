package main

import (
	"context"
	"sync/atomic"
	"testing"
)

type countingStorage struct {
	Storage
	metaCalls atomic.Int64
}

func (s *countingStorage) Meta(ctx context.Context, key string) (Object, error) {
	s.metaCalls.Add(1)
	return s.Storage.Meta(ctx, key)
}

func TestCleanupInvalidGeneratedVideosSkipsOrdinaryPhotos(t *testing.T) {
	app := newTestApp(t)
	if _, err := app.db.Exec(`INSERT INTO photos(id,storage_key,is_live_photo) VALUES('ordinary-photo','photos/ordinary.jpg',0)`); err != nil {
		t.Fatal(err)
	}

	counting := &countingStorage{Storage: app.storage}
	app.storage = counting
	app.cleanupInvalidGeneratedVideos()

	if calls := counting.metaCalls.Load(); calls != 0 {
		t.Fatalf("cleanup probed generated video for ordinary photo %d time(s)", calls)
	}
}
