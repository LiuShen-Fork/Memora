package main

import (
	"os"
	"runtime"
	"strconv"
	"strings"
)

// memoryStats reports the memory limit visible to the process. Container
// limits are more useful than the Go heap size on the dashboard.
func memoryStats() (used, total uint64) {
	for _, paths := range [][2]string{
		{"/sys/fs/cgroup/memory.current", "/sys/fs/cgroup/memory.max"},
		{"/sys/fs/cgroup/memory/memory.usage_in_bytes", "/sys/fs/cgroup/memory/memory.limit_in_bytes"},
	} {
		current, currentOK := readMemoryValue(paths[0])
		limit, limitOK := readMemoryValue(paths[1])
		if currentOK && limitOK && limit > 0 && limit < 1<<60 {
			return current, limit
		}
	}

	var stats runtime.MemStats
	runtime.ReadMemStats(&stats)
	return stats.Alloc, stats.Sys
}

func readMemoryValue(path string) (uint64, bool) {
	value, err := os.ReadFile(path)
	if err != nil {
		return 0, false
	}
	text := strings.TrimSpace(string(value))
	if text == "" || text == "max" {
		return 0, false
	}
	parsed, err := strconv.ParseUint(text, 10, 64)
	return parsed, err == nil
}
