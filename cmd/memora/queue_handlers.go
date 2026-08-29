package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"
)

func (a *App) queueRoute(w http.ResponseWriter, r *http.Request, rest string) {
	if rest == "add-task" && r.Method == "POST" {
		a.addTask(w, r, false)
		return
	}
	if rest == "add-tasks" && r.Method == "POST" {
		a.addTask(w, r, true)
		return
	}
	if rest == "stats" && r.Method == "GET" {
		a.queueStats(w, r, 0)
		return
	}
	if strings.HasPrefix(rest, "stats/") && r.Method == "GET" {
		id, err := strconv.ParseInt(strings.TrimPrefix(rest, "stats/"), 10, 64)
		if err != nil || id <= 0 {
			errorJSON(w, http.StatusBadRequest, "Invalid task ID")
			return
		}
		a.queueStats(w, r, id)
		return
	}
	if rest == "task/list" && r.Method == "GET" {
		a.taskList(w, r)
		return
	}
	if rest == "task/clear" && r.Method == "DELETE" {
		a.taskClear(w, r)
		return
	}
	if rest == "task/retry" && r.Method == "POST" {
		a.taskRetry(w, r)
		return
	}
	if rest == "task/retry-batch" && r.Method == "POST" {
		a.taskRetryBatch(w, r)
		return
	}
	if strings.HasPrefix(rest, "failed/") && r.Method == "DELETE" {
		a.taskDeleteFailed(w, r, strings.TrimPrefix(rest, "failed/"))
		return
	}
	errorJSON(w, 404, "Not Found")
}
func (a *App) addTask(w http.ResponseWriter, r *http.Request, batch bool) {
	if _, ok := a.require(r); !ok {
		errorJSON(w, http.StatusUnauthorized, "Unauthorized")
		return
	}
	var body struct {
		Payload map[string]any `json:"payload"`
		Tasks   []struct {
			Payload     map[string]any `json:"payload"`
			Priority    *int           `json:"priority"`
			MaxAttempts *int           `json:"maxAttempts"`
		} `json:"tasks"`
		Priority           int `json:"priority"`
		MaxAttempts        int `json:"maxAttempts"`
		DefaultPriority    int `json:"defaultPriority"`
		DefaultMaxAttempts int `json:"defaultMaxAttempts"`
	}
	if decodeJSON(r, &body) != nil {
		errorJSON(w, 400, "Invalid request")
		return
	}
	if batch {
		if len(body.Tasks) < 1 || len(body.Tasks) > 1000 {
			errorJSON(w, http.StatusBadRequest, "tasks must contain between 1 and 1000 items")
			return
		}
		// Batch requests use per-item payload/options in the Nuxt contract.
		if body.DefaultPriority < 0 || body.DefaultPriority > 9 || body.DefaultMaxAttempts < 0 || body.DefaultMaxAttempts > 5 {
			errorJSON(w, http.StatusBadRequest, "invalid default queue options")
			return
		}
		for _, task := range body.Tasks {
			if err := validateQueuePayload(task.Payload); err != nil {
				errorJSON(w, http.StatusBadRequest, err.Error())
				return
			}
			if task.Priority != nil && (*task.Priority < 0 || *task.Priority > 9) {
				errorJSON(w, 400, "priority must be between 0 and 9")
				return
			}
			if task.MaxAttempts != nil && (*task.MaxAttempts < 1 || *task.MaxAttempts > 5) {
				errorJSON(w, 400, "maxAttempts must be between 1 and 5")
				return
			}
		}
		body.Priority = body.DefaultPriority
		body.MaxAttempts = body.DefaultMaxAttempts
	} else {
		if err := validateQueuePayload(body.Payload); err != nil {
			errorJSON(w, http.StatusBadRequest, err.Error())
			return
		}
		if body.Priority < 0 || body.Priority > 9 || body.MaxAttempts < 0 || body.MaxAttempts > 5 {
			errorJSON(w, http.StatusBadRequest, "invalid queue options")
			return
		}
	}
	if body.MaxAttempts < 1 {
		body.MaxAttempts = 3
	}
	items := []map[string]any{}
	priorities := []int{}
	maxAttempts := []int{}
	if batch {
		for _, task := range body.Tasks {
			items = append(items, task.Payload)
			priority := body.DefaultPriority
			if task.Priority != nil {
				priority = *task.Priority
			}
			attempts := body.DefaultMaxAttempts
			if task.MaxAttempts != nil {
				attempts = *task.MaxAttempts
			}
			if attempts < 1 {
				attempts = 3
			}
			priorities = append(priorities, priority)
			maxAttempts = append(maxAttempts, attempts)
		}
	} else {
		items = []map[string]any{body.Payload}
		priorities = []int{body.Priority}
		maxAttempts = []int{body.MaxAttempts}
	}
	ids := []int64{}
	tx, err := a.db.BeginTx(r.Context(), nil)
	if err != nil {
		errorJSON(w, http.StatusInternalServerError, "Failed to queue tasks")
		return
	}
	defer tx.Rollback()
	insertTask, err := tx.PrepareContext(r.Context(), `INSERT INTO pipeline_queue(payload,priority,max_attempts,status) VALUES(?,?,?,'pending')`)
	if err != nil {
		errorJSON(w, http.StatusInternalServerError, "Failed to prepare queue insert")
		return
	}
	defer insertTask.Close()
	for _, payload := range items {
		if payload == nil {
			continue
		}
		data, _ := json.Marshal(payload)
		res, err := insertTask.ExecContext(r.Context(), string(data), priorities[len(ids)], maxAttempts[len(ids)])
		if err != nil {
			errorJSON(w, http.StatusInternalServerError, err.Error())
			return
		}
		id, _ := res.LastInsertId()
		ids = append(ids, id)
	}
	if err := tx.Commit(); err != nil {
		errorJSON(w, http.StatusInternalServerError, "Failed to queue tasks")
		return
	}
	a.wakeQueue()
	if batch {
		results := make([]map[string]any, 0, len(ids))
		for i, id := range ids {
			results = append(results, map[string]any{"index": i, "taskId": id, "payload": items[i], "success": true})
		}
		writeJSON(w, 201, map[string]any{"success": true, "totalTasks": len(items), "successCount": len(ids), "errorCount": 0, "results": results, "message": fmt.Sprintf("Processed %d tasks: %d successful, 0 failed", len(items), len(ids))})
	} else {
		var id int64
		if len(ids) > 0 {
			id = ids[0]
		}
		writeJSON(w, 201, map[string]any{"success": true, "taskId": id, "message": "Task added to queue successfully", "payload": body.Payload})
	}
}

func validateQueuePayload(payload map[string]any) error {
	typeName, _ := payload["type"].(string)
	if typeName == "" {
		return fmt.Errorf("payload.type is required")
	}
	switch typeName {
	case "photo", "live-photo-video":
		if key, _ := payload["storageKey"].(string); key == "" {
			return fmt.Errorf("payload.storageKey is required")
		}
		if rawAlbumID, exists := payload["albumId"]; exists {
			albumID, ok := numberValue(rawAlbumID)
			if !ok || albumID < 1 || math.Trunc(albumID) != albumID {
				return fmt.Errorf("payload.albumId is invalid")
			}
		}
	case "photo-metadata-update":
		if id, _ := payload["photoId"].(string); id == "" {
			return fmt.Errorf("payload.photoId is required")
		}
		if updates, ok := payload["updates"].(map[string]any); !ok || len(updates) == 0 {
			return fmt.Errorf("payload.updates is required")
		}
	case "photo-reverse-geocoding":
		if id, _ := payload["photoId"].(string); id == "" {
			return fmt.Errorf("payload.photoId is required")
		}
		if v, ok := payload["latitude"]; ok {
			if n, ok := v.(float64); !ok || n < -90 || n > 90 {
				return fmt.Errorf("payload.latitude is invalid")
			}
		}
		if v, ok := payload["longitude"]; ok {
			if n, ok := v.(float64); !ok || n < -180 || n > 180 {
				return fmt.Errorf("payload.longitude is invalid")
			}
		}
	case "photo-erase-location":
		if id, _ := payload["photoId"].(string); id == "" {
			return fmt.Errorf("payload.photoId is required")
		}
	default:
		return fmt.Errorf("unsupported payload.type: %s", typeName)
	}
	return nil
}
func (a *App) queueStats(w http.ResponseWriter, r *http.Request, id int64) {
	if _, ok := a.require(r); !ok {
		errorJSON(w, http.StatusUnauthorized, "Unauthorized")
		return
	}
	if id > 0 {
		var payload, status, stage, errMsg string
		var attempts, max int
		if a.db.QueryRow(`SELECT payload,status,COALESCE(status_stage,''),COALESCE(error_message,''),attempts,max_attempts FROM pipeline_queue WHERE id=?`, id).Scan(&payload, &status, &stage, &errMsg, &attempts, &max) != nil {
			errorJSON(w, 404, "Task not found")
			return
		}
		var p any
		_ = json.Unmarshal([]byte(payload), &p)
		var created, completed sql.NullInt64
		_ = a.db.QueryRow(`SELECT created_at,completed_at FROM pipeline_queue WHERE id=?`, id).Scan(&created, &completed)
		result := map[string]any{"id": id, "payload": p, "status": status, "statusStage": stage, "errorMessage": errMsg, "attempts": attempts, "maxAttempts": max, "createdAt": nil, "completedAt": nil}
		if created.Valid {
			result["createdAt"] = time.Unix(created.Int64, 0)
		}
		if completed.Valid {
			result["completedAt"] = time.Unix(completed.Int64, 0)
		}
		writeJSON(w, 200, result)
		return
	}
	rows, _ := a.db.Query(`SELECT status,count(*) FROM pipeline_queue GROUP BY status`)
	out := map[string]int{}
	for rows != nil && rows.Next() {
		var s string
		var n int
		_ = rows.Scan(&s, &n)
		out[s] = n
	}
	if rows != nil {
		rows.Close()
	}
	pool := a.queuePoolStats()
	writeJSON(w, 200, map[string]any{"timestamp": time.Now().UTC().Format(time.RFC3339Nano), "pool": pool, "queue": out})
}
func (a *App) taskList(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.require(r); !ok {
		errorJSON(w, http.StatusUnauthorized, "Unauthorized")
		return
	}
	statusFilter := r.URL.Query().Get("status")
	typeFilter := r.URL.Query().Get("type")
	// Queue history can grow indefinitely. Keep the dashboard response bounded;
	// completed history remains durable and can still be cleared explicitly.
	page, pageSize, _ := paginationParams(r)
	if value, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && value > 0 && r.URL.Query().Get("pageSize") == "" {
		pageSize = value
	}
	if pageSize > 100 {
		pageSize = 100
	}
	query := `SELECT id,payload,priority,attempts,max_attempts,status,status_stage,error_message,created_at,completed_at FROM pipeline_queue`
	conditions := []string{}
	args := []any{}
	if statusFilter != "" {
		conditions = append(conditions, "status=?")
		args = append(args, statusFilter)
	}
	if typeFilter != "" {
		conditions = append(conditions, "json_extract(payload,'$.type')=?")
		args = append(args, typeFilter)
	}
	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ")
	}
	stats := map[string]int{"pending": 0, "in-stages": 0, "completed": 0, "failed": 0}
	statsQuery := `SELECT status,count(*) FROM pipeline_queue`
	if len(conditions) > 0 {
		statsQuery += " WHERE " + strings.Join(conditions, " AND ")
	}
	statsQuery += " GROUP BY status"
	statsRows, statsErr := a.db.Query(statsQuery, args...)
	if statsErr == nil {
		for statsRows.Next() {
			var status string
			var count int
			if statsRows.Scan(&status, &count) == nil {
				stats[status] = count
			}
		}
		_ = statsRows.Close()
	}
	var total int
	countQuery := `SELECT COUNT(*) FROM pipeline_queue`
	if len(conditions) > 0 {
		countQuery += " WHERE " + strings.Join(conditions, " AND ")
	}
	if err := a.db.QueryRowContext(r.Context(), countQuery, args...).Scan(&total); err != nil {
		errorJSON(w, http.StatusInternalServerError, "Failed to count tasks")
		return
	}
	query += " ORDER BY created_at DESC LIMIT ? OFFSET ?"
	queryArgs := append(append([]any(nil), args...), pageSize, (page-1)*pageSize)
	rows, err := a.db.Query(query, queryArgs...)
	if err != nil {
		errorJSON(w, http.StatusInternalServerError, "Failed to fetch task list")
		return
	}
	out := make([]map[string]any, 0, pageSize)
	for rows != nil && rows.Next() {
		var id, priority, attempts, max int
		var p, status, stage, errMsg string
		var created int64
		var completed sql.NullInt64
		_ = rows.Scan(&id, &p, &priority, &attempts, &max, &status, &stage, &errMsg, &created, &completed)
		var payload any
		_ = json.Unmarshal([]byte(p), &payload)
		var completedAt any
		if completed.Valid {
			completedAt = time.Unix(completed.Int64, 0)
		}
		out = append(out, map[string]any{"id": id, "payload": payload, "priority": priority, "attempts": attempts, "maxAttempts": max, "status": status, "statusStage": stage, "errorMessage": errMsg, "createdAt": time.Unix(created, 0), "completedAt": completedAt})
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		errorJSON(w, http.StatusInternalServerError, "Failed to fetch task list")
		return
	}
	if err := rows.Close(); err != nil {
		errorJSON(w, http.StatusInternalServerError, "Failed to fetch task list")
		return
	}
	writeJSON(w, 200, map[string]any{"success": true, "data": out, "page": page, "pageSize": pageSize, "total": total, "totalPages": (total + pageSize - 1) / pageSize, "stats": stats})
}
func (a *App) taskClear(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.require(r); !ok {
		errorJSON(w, http.StatusUnauthorized, "Unauthorized")
		return
	}
	includeCompleted := r.URL.Query().Get("includeCompleted") != "false"
	includeFailed := r.URL.Query().Get("includeFailed") != "false"
	if !includeCompleted && !includeFailed {
		errorJSON(w, http.StatusBadRequest, "At least one task status must be included")
		return
	}
	olderThanText := r.URL.Query().Get("olderThanDays")
	var olderThan int64
	var olderThanDays int64
	if olderThanText != "" {
		parsed, err := strconv.ParseInt(olderThanText, 10, 64)
		if err != nil || parsed < 0 {
			errorJSON(w, http.StatusBadRequest, "olderThanDays must be a non-negative integer")
			return
		}
		olderThanDays = parsed
		olderThan = time.Now().Unix() - parsed*24*60*60
	}
	statuses := []string{}
	if includeCompleted {
		statuses = append(statuses, "'completed'")
	}
	if includeFailed {
		statuses = append(statuses, "'failed'")
	}
	where := `status IN (` + strings.Join(statuses, ",") + `)`
	args := []any{}
	if olderThanText != "" {
		where += " AND created_at < ?"
		args = append(args, olderThan)
	}
	counts := map[string]int64{"completed": 0, "failed": 0}
	rows, _ := a.db.Query(`SELECT status,count(*) FROM pipeline_queue WHERE `+where+` GROUP BY status`, args...)
	for rows != nil && rows.Next() {
		var status string
		var count int64
		_ = rows.Scan(&status, &count)
		counts[status] = count
	}
	if rows != nil {
		rows.Close()
	}
	result, err := a.db.Exec(`DELETE FROM pipeline_queue WHERE `+where, args...)
	if err != nil {
		errorJSON(w, http.StatusInternalServerError, "Failed to clear tasks")
		return
	}
	deleted, _ := result.RowsAffected()
	response := map[string]any{"success": true, "deletedCount": deleted, "breakdown": counts}
	if olderThanText != "" {
		response["filter"] = map[string]any{"olderThanDays": olderThanDays, "thresholdDate": time.Unix(olderThan, 0).UTC()}
	}
	writeJSON(w, http.StatusOK, response)
}
func (a *App) taskRetry(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.require(r); !ok {
		errorJSON(w, http.StatusUnauthorized, "Unauthorized")
		return
	}
	var b struct {
		TaskID int64 `json:"taskId"`
	}
	if decodeJSON(r, &b) != nil {
		errorJSON(w, 400, "Invalid request")
		return
	}
	var payload string
	var status string
	if err := a.db.QueryRow(`SELECT payload,status FROM pipeline_queue WHERE id=?`, b.TaskID).Scan(&payload, &status); err != nil {
		errorJSON(w, http.StatusNotFound, "Task not found")
		return
	}
	if status != "failed" {
		errorJSON(w, http.StatusBadRequest, "Task is not in failed status, current status: "+status)
		return
	}
	if _, err := a.db.Exec(`UPDATE pipeline_queue SET status='pending',status_stage=NULL,error_message=NULL,attempts=0,created_at=unixepoch() WHERE id=?`, b.TaskID); err != nil {
		errorJSON(w, 500, err.Error())
		return
	}
	var parsed map[string]any
	_ = json.Unmarshal([]byte(payload), &parsed)
	a.wakeQueue()
	writeJSON(w, 200, map[string]any{"success": true, "message": fmt.Sprintf("Task %d has been reset and will be retried", b.TaskID), "taskId": b.TaskID, "payload": map[string]any{"type": parsed["type"], "storageKey": parsed["storageKey"]}})
}
func (a *App) taskRetryBatch(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.require(r); !ok {
		errorJSON(w, http.StatusUnauthorized, "Unauthorized")
		return
	}
	var b struct {
		TaskIDs  []int64 `json:"taskIds"`
		RetryAll bool    `json:"retryAll"`
	}
	if decodeJSON(r, &b) != nil {
		errorJSON(w, 400, "Invalid request")
		return
	}
	if !b.RetryAll && len(b.TaskIDs) == 0 {
		errorJSON(w, http.StatusBadRequest, "Either taskIds array or retryAll flag must be provided")
		return
	}
	if b.RetryAll {
		var retriedTasks []map[string]any
		rows, _ := a.db.Query(`SELECT id,payload FROM pipeline_queue WHERE status='failed'`)
		for rows != nil && rows.Next() {
			var id int64
			var payload string
			_ = rows.Scan(&id, &payload)
			var parsed map[string]any
			_ = json.Unmarshal([]byte(payload), &parsed)
			retriedTasks = append(retriedTasks, map[string]any{"id": id, "type": parsed["type"], "storageKey": parsed["storageKey"]})
		}
		if rows != nil {
			rows.Close()
		}
		result, err := a.db.Exec(`UPDATE pipeline_queue SET status='pending',status_stage=NULL,error_message=NULL,attempts=0,created_at=unixepoch() WHERE status='failed'`)
		if err != nil {
			errorJSON(w, http.StatusInternalServerError, err.Error())
			return
		}
		retried, _ := result.RowsAffected()
		a.wakeQueue()
		writeJSON(w, http.StatusOK, map[string]any{"success": true, "message": fmt.Sprintf("Successfully reset %d failed tasks for retry", retried), "retriedCount": retried, "skippedCount": 0, "retriedTasks": retriedTasks, "skippedTasks": []any{}})
		return
	}
	retried := 0
	skipped := 0
	var retriedTasks, skippedTasks []map[string]any
	for _, id := range b.TaskIDs {
		var payload, status string
		if err := a.db.QueryRow(`SELECT payload,status FROM pipeline_queue WHERE id=?`, id).Scan(&payload, &status); err != nil {
			skipped++
			skippedTasks = append(skippedTasks, map[string]any{"id": id, "status": "missing", "reason": "Task not found"})
			continue
		}
		if status != "failed" {
			skipped++
			skippedTasks = append(skippedTasks, map[string]any{"id": id, "status": status, "reason": "Task is not in failed status (current: " + status + ")"})
			continue
		}
		_, _ = a.db.Exec(`UPDATE pipeline_queue SET status='pending',status_stage=NULL,error_message=NULL,attempts=0,created_at=unixepoch() WHERE id=?`, id)
		var parsed map[string]any
		_ = json.Unmarshal([]byte(payload), &parsed)
		retried++
		retriedTasks = append(retriedTasks, map[string]any{"id": id, "type": parsed["type"], "storageKey": parsed["storageKey"]})
	}
	a.wakeQueue()
	writeJSON(w, 200, map[string]any{"success": true, "message": fmt.Sprintf("Successfully reset %d failed tasks for retry", retried), "retriedCount": retried, "skippedCount": skipped, "retriedTasks": retriedTasks, "skippedTasks": skippedTasks})
}

func (a *App) taskDeleteFailed(w http.ResponseWriter, r *http.Request, idText string) {
	if _, ok := a.require(r); !ok {
		errorJSON(w, http.StatusUnauthorized, "Unauthorized")
		return
	}
	id, err := strconv.ParseInt(idText, 10, 64)
	if err != nil {
		errorJSON(w, http.StatusBadRequest, "Invalid task ID")
		return
	}
	result, err := a.db.Exec(`DELETE FROM pipeline_queue WHERE id=? AND status='failed'`, id)
	if err != nil {
		errorJSON(w, http.StatusInternalServerError, "Failed to delete task")
		return
	}
	changed, _ := result.RowsAffected()
	if changed == 0 {
		errorJSON(w, http.StatusNotFound, "Failed task not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"success": true})
}
