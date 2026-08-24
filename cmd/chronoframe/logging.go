package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

type LogBuffer struct {
	mu       sync.RWMutex
	lines    []string
	filePath string
}

func NewLogBuffer(filePath ...string) *LogBuffer {
	path := ""
	if len(filePath) > 0 {
		path = filePath[0]
	}
	return &LogBuffer{lines: []string{}, filePath: path}
}
func (l *LogBuffer) Add(scope, message string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	line := fmt.Sprintf("[%s] %s", scope, message)
	l.lines = append(l.lines, line)
	if len(l.lines) > 200 {
		l.lines = l.lines[len(l.lines)-200:]
	}
	if l.filePath != "" {
		if err := os.MkdirAll(filepath.Dir(l.filePath), 0o755); err == nil {
			if file, err := os.OpenFile(l.filePath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644); err == nil {
				_, _ = fmt.Fprintln(file, line)
				_ = file.Close()
			}
		}
	}
}
func (l *LogBuffer) Snapshot() []string {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return append([]string(nil), l.lines...)
}

func (l *LogBuffer) Path() string {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.filePath
}

func (l *LogBuffer) Tail(maxLines int) ([]string, int64, error) {
	path := l.Path()
	if path == "" {
		return l.Snapshot(), 0, nil
	}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return l.Snapshot(), 0, nil
	}
	if err != nil {
		return nil, 0, err
	}
	lines := nonEmptyLines(string(data))
	if maxLines > 0 && len(lines) > maxLines {
		lines = lines[len(lines)-maxLines:]
	}
	return lines, int64(len(data)), nil
}

func (l *LogBuffer) Since(offset int64) ([]string, int64, error) {
	path := l.Path()
	if path == "" {
		return nil, offset, nil
	}
	file, err := os.Open(path)
	if os.IsNotExist(err) {
		return nil, 0, nil
	}
	if err != nil {
		return nil, offset, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, offset, err
	}
	if info.Size() < offset {
		offset = 0
	}
	if _, err := file.Seek(offset, io.SeekStart); err != nil {
		return nil, offset, err
	}
	data, err := io.ReadAll(file)
	if err != nil {
		return nil, offset, err
	}
	return nonEmptyLines(string(data)), info.Size(), nil
}

func nonEmptyLines(value string) []string {
	lines := strings.Split(value, "\n")
	result := make([]string, 0, len(lines))
	for _, line := range lines {
		if line = strings.TrimSpace(line); line != "" {
			result = append(result, line)
		}
	}
	return result
}

var _ = sort.Strings
