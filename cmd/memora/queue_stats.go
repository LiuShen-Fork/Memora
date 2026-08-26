package main

import (
	"strconv"
	"time"
)

// queuePoolStats exposes the same aggregate shape as the Nuxt worker pool.
// Worker counters are derived from durable queue state so they survive restarts.
func (a *App) queuePoolStats() map[string]any {
	var completed, failed, inStages int
	_ = a.db.QueryRow(`SELECT COALESCE(SUM(status='completed'),0), COALESCE(SUM(status='failed'),0), COALESCE(SUM(status='in-stages'),0) FROM pipeline_queue`).Scan(&completed, &failed, &inStages)
	workers := a.cfg.WorkerCount
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
