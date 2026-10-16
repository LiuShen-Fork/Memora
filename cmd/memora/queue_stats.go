package main

import (
	"database/sql"
	"strconv"
	"time"
)

// queueSummary is what the dashboard reads at a glance: how much is waiting,
// how much gave up, and how long the oldest waiting task has been sitting.
func (a *App) queueSummary() map[string]any {
	var pending, failed int
	_ = a.db.QueryRow(`SELECT COALESCE(SUM(status='pending'),0), COALESCE(SUM(status='failed'),0) FROM pipeline_queue`).Scan(&pending, &failed)
	var oldest sql.NullInt64
	// A task waiting out a retry delay is not a stuck queue, so the wait is
	// measured from the moment the task was due rather than from its creation.
	_ = a.db.QueryRow(`SELECT MIN(MAX(created_at,available_at)) FROM pipeline_queue WHERE status='pending'`).Scan(&oldest)
	wait := 0
	if oldest.Valid {
		if seconds := int(time.Now().Unix() - oldest.Int64); seconds > 0 {
			wait = seconds
		}
	}
	return map[string]any{"pending": pending, "failed": failed, "oldestWaitSeconds": wait}
}

// queuePoolStats exposes the same aggregate shape as the Nuxt worker pool.
// Worker counters are derived from durable queue state so they survive restarts.
func (a *App) queuePoolStats() map[string]any {
	var completed, failed, inStages int
	_ = a.db.QueryRow(`SELECT COALESCE(SUM(status='completed'),0), COALESCE(SUM(status='failed'),0), COALESCE(SUM(status='in-stages'),0) FROM pipeline_queue`).Scan(&completed, &failed, &inStages)
	workers := a.workerCount()
	if workers < 1 {
		workers = 1
	}
	active := inStages
	if active > workers {
		active = workers
	}
	workerStats := make([]map[string]any, 0, workers)
	for i := 0; i < workers; i++ {
		processed := completed / workers
		if i < completed%workers {
			processed++
		}
		errors := failed / workers
		if i < failed%workers {
			errors++
		}
		rate := 0.0
		if processed+errors > 0 {
			rate = float64(processed) / float64(processed+errors) * 100
		}
		workerStats = append(workerStats, map[string]any{
			"workerId":       "worker-" + strconv.Itoa(i+1),
			"isProcessing":   i < active,
			"processedCount": processed,
			"errorCount":     errors,
			"uptime":         int(time.Since(processStartedAt).Seconds()),
			"successRate":    rate,
		})
	}
	return map[string]any{
		"isActive":           true,
		"workerCount":        workers,
		"totalWorkers":       workers,
		"activeWorkers":      active,
		"totalProcessed":     completed,
		"totalErrors":        failed,
		"averageSuccessRate": successRate(completed, failed),
		"workers":            workerStats,
	}
}

func successRate(processed, failed int) float64 {
	if processed+failed == 0 {
		return 0
	}
	return float64(processed) / float64(processed+failed) * 100
}
