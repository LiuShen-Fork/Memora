package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"
	"time"
)

const (
	defaultMediaMaxBytes = 256 << 20
	defaultMediaTimeout  = 2 * time.Minute
	maxToolOutputBytes   = 256 << 20
	maxExifOutputBytes   = 16 << 20
)

func (a *App) mediaLimit() int64 {
	if a.cfg.MediaMaxBytes > 0 {
		return a.cfg.MediaMaxBytes
	}
	return defaultMediaMaxBytes
}

type cappedBuffer struct {
	bytes.Buffer
	limit    int
	exceeded bool
}

func (b *cappedBuffer) Write(data []byte) (int, error) {
	remaining := b.limit - b.Len()
	if remaining <= 0 {
		b.exceeded = true
		return 0, fmt.Errorf("command output exceeds limit")
	}
	if len(data) > remaining {
		_, _ = b.Buffer.Write(data[:remaining])
		b.exceeded = true
		return remaining, fmt.Errorf("command output exceeds limit")
	}
	return b.Buffer.Write(data)
}

func runCommandOutput(ctx context.Context, cmd *exec.Cmd, maxBytes int) ([]byte, error) {
	stdout := &cappedBuffer{limit: maxBytes}
	var stderr bytes.Buffer
	cmd.Stdout = stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if stderr.Len() > 0 {
			return nil, fmt.Errorf("%w: %s", err, stderr.String())
		}
		return nil, err
	}
	if stdout.exceeded {
		return nil, fmt.Errorf("command output exceeds limit of %d bytes", maxBytes)
	}
	return stdout.Bytes(), nil
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
