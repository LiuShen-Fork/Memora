package main

import (
	"context"
	"fmt"
	"io"
	"time"
)

const (
	defaultMediaMaxBytes = 256 << 20
	defaultMediaTimeout  = 2 * time.Minute
)

func (a *App) mediaLimit() int64 {
	if a.cfg.MediaMaxBytes > 0 {
		return a.cfg.MediaMaxBytes
	}
	return defaultMediaMaxBytes
}

func (a *App) mediaContext(parent context.Context) (context.Context, context.CancelFunc) {
	timeout := a.cfg.MediaTimeout
	if timeout <= 0 {
		timeout = defaultMediaTimeout
	}
	return context.WithTimeout(parent, timeout)
}

func readLimited(reader io.Reader, maxBytes int64) ([]byte, error) {
	if maxBytes < 1 {
		maxBytes = defaultMediaMaxBytes
	}
	data, err := io.ReadAll(io.LimitReader(reader, maxBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > maxBytes {
		return nil, fmt.Errorf("media exceeds configured limit of %d bytes", maxBytes)
	}
	return data, nil
}

func (a *App) readStorageBytes(ctx context.Context, key string) ([]byte, error) {
	if readerStorage, ok := a.storage.(ReaderStorage); ok {
		reader, _, err := readerStorage.Open(ctx, key)
		if err != nil {
			return nil, err
		}
		defer reader.Close()
		return readLimited(reader, a.mediaLimit())
	}
	data, err := a.storage.Get(ctx, key)
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > a.mediaLimit() {
		return nil, fmt.Errorf("media exceeds configured limit of %d bytes", a.mediaLimit())
	}
	return data, nil
}

func (a *App) acquireMedia(ctx context.Context) (func(), error) {
	if a.mediaSlots == nil {
		return func() {}, nil
	}
	select {
	case a.mediaSlots <- struct{}{}:
		return func() { <-a.mediaSlots }, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}
