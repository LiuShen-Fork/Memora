package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
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
		id, _ := strconv.ParseInt(strings.TrimPrefix(rest, "stats/"), 10, 64)
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
		Payload     map[string]any   `json:"payload"`
		Tasks       []map[string]any `json:"tasks"`
		Priority    int              `json:"priority"`
		MaxAttempts int              `json:"maxAttempts"`
	}
	if decodeJSON(r, &body) != nil {
		errorJSON(w, 400, "Invalid request")
		return
	}
	if body.MaxAttempts < 1 {
		body.MaxAttempts = 3
	}
	items := []map[string]any{}
	if batch {
		items = body.Tasks
	} else {
		items = []map[string]any{body.Payload}
	}
	ids := []int64{}
	for _, payload := range items {
		if payload == nil {
			continue
		}
		data, _ := json.Marshal(payload)
		res, err := a.db.Exec(`INSERT INTO pipeline_queue(payload,priority,max_attempts,status) VALUES(?,?,?,'pending')`, string(data), body.Priority, body.MaxAttempts)
		if err != nil {
			errorJSON(w, 500, err.Error())
			return
		}
		id, _ := res.LastInsertId()
		ids = append(ids, id)
	}
	a.wakeQueue()
	if batch {
		writeJSON(w, 201, map[string]any{"success": true, "taskIds": ids})
	} else {
		var id int64
		if len(ids) > 0 {
			id = ids[0]
		}
		writeJSON(w, 201, map[string]any{"success": true, "taskId": id, "message": "Task added to queue successfully"})
	}
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
		writeJSON(w, 200, map[string]any{"id": id, "payload": p, "status": status, "statusStage": stage, "errorMessage": errMsg, "attempts": attempts, "maxAttempts": max})
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
	writeJSON(w, 200, out)
}
func (a *App) taskList(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.require(r); !ok {
		errorJSON(w, http.StatusUnauthorized, "Unauthorized")
		return
	}
	statusFilter := r.URL.Query().Get("status")
	typeFilter := r.URL.Query().Get("type")
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
	query += " ORDER BY created_at DESC LIMIT 500"
	rows, err := a.db.Query(query, args...)
	if err != nil {
		errorJSON(w, http.StatusInternalServerError, "Failed to fetch task list")
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows != nil && rows.Next() {
		var id, priority, attempts, max int
		var p, status, stage, errMsg string
		var created int64
		var completed sql.NullInt64
		_ = rows.Scan(&id, &p, &priority, &attempts, &max, &status, &stage, &errMsg, &created, &completed)
		var payload any
		_ = json.Unmarshal([]byte(p), &payload)
		out = append(out, map[string]any{"id": id, "payload": payload, "priority": priority, "attempts": attempts, "maxAttempts": max, "status": status, "statusStage": stage, "errorMessage": errMsg, "createdAt": time.Unix(created, 0), "completedAt": completed.Int64})
	}
	writeJSON(w, 200, map[string]any{"success": true, "data": out})
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
