package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// logLinesKeptInMemory bounds what a buffer that has nowhere to write still
// remembers, so a server whose log file cannot be written does not grow without
// end. A buffer with a file keeps everything: the file is the history.
const logLinesKeptInMemory = 200

type LogBuffer struct {
	mu       sync.RWMutex
	lines    []string
	filePath string
}

// logLine is one line of the log and the byte position the file had reached
// once that line was written. The position is what a reader that lost the
// stream resumes from, so no line is read twice and none is passed over.
type logLine struct {
	text   string
	offset int64
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
	if len(l.lines) > logLinesKeptInMemory {
		l.lines = l.lines[len(l.lines)-logLinesKeptInMemory:]
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

// Tail reads the last maxLines of the log, or the whole of it when maxLines is
// not positive, together with the size the file had reached. A buffer whose
// file is not there is answered from what it remembers.
func (l *LogBuffer) Tail(maxLines int) ([]logLine, int64, error) {
	path := l.Path()
	if path == "" {
		return rememberedLines(l.Snapshot()), 0, nil
	}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return rememberedLines(l.Snapshot()), 0, nil
	}
	if err != nil {
		return nil, 0, err
	}
	lines := readLogLines(data, 0)
	if maxLines > 0 && len(lines) > maxLines {
		lines = lines[len(lines)-maxLines:]
	}
	return lines, int64(len(data)), nil
}

// Since reads what was written after offset, and answers with the size the file
// has reached now. A file that shrank below the offset was replaced or emptied,
// and is read from its beginning again rather than from a position it no longer
// has.
func (l *LogBuffer) Since(offset int64) ([]logLine, int64, error) {
	path := l.Path()
	if path == "" {
		return nil, offset, nil
	}
	file, err := os.Open(path)
	if os.IsNotExist(err) {
		return nil, offset, nil
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
	return readLogLines(data, offset), info.Size(), nil
}

// readLogLines cuts the lines out of what was read from base, tagging each with
// the position that follows it. A last line whose newline has not been written
// yet is left out: the writer is in the middle of it, and the next read hands it
// over whole.
func readLogLines(data []byte, base int64) []logLine {
	lines := make([]logLine, 0, bytes.Count(data, []byte{'\n'}))
	for start := 0; start < len(data); {
		end := bytes.IndexByte(data[start:], '\n')
		if end < 0 {
			break
		}
		end += start
		if text := strings.TrimSpace(string(data[start:end])); text != "" {
			lines = append(lines, logLine{text: text, offset: base + int64(end) + 1})
		}
		start = end + 1
	}
	return lines
}

// rememberedLines is what a buffer answers with when it has no file to read.
// The lines carry no position, because there is nothing to resume in: what such
// a buffer remembers is handed over as a backlog, and it has nothing to add
// afterwards.
func rememberedLines(lines []string) []logLine {
	remembered := make([]logLine, 0, len(lines))
	for _, line := range lines {
		remembered = append(remembered, logLine{text: line})
	}
	return remembered
}
