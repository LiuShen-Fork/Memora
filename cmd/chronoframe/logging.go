package main

import (
	"fmt"
	"sort"
	"sync"
)

type LogBuffer struct {
	mu    sync.RWMutex
	lines []string
}

func NewLogBuffer() *LogBuffer { return &LogBuffer{lines: []string{}} }
func (l *LogBuffer) Add(scope, message string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.lines = append(l.lines, fmt.Sprintf("[%s] %s", scope, message))
	if len(l.lines) > 200 {
		l.lines = l.lines[len(l.lines)-200:]
	}
}
func (l *LogBuffer) Snapshot() []string {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return append([]string(nil), l.lines...)
}

var _ = sort.Strings
