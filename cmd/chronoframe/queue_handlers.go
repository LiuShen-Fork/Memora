package main

import (
	"database/sql"
	"encoding/json"
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
	if !a.requireAdmin(w, r) {
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
	if !a.requireAdmin(w, r) {
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
	if !a.requireAdmin(w, r) {
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
	if !a.requireAdmin(w, r) {
		return
	}
	includeCompleted := r.URL.Query().Get("includeCompleted") != "false"
	includeFailed := r.URL.Query().Get("includeFailed") != "false"
	if !includeCompleted && !includeFailed {
		errorJSON(w, http.StatusBadRequest, "At least one task status must be included")
		return
	}
	statuses := []string{}
	if includeCompleted {
		statuses = append(statuses, "'completed'")
	}
	if includeFailed {
		statuses = append(statuses, "'failed'")
	}
	result, err := a.db.Exec(`DELETE FROM pipeline_queue WHERE status IN (` + strings.Join(statuses, ",") + `)`)
	if err != nil {
		errorJSON(w, http.StatusInternalServerError, "Failed to clear tasks")
		return
	}
	deleted, _ := result.RowsAffected()
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "deletedCount": deleted, "breakdown": map[string]any{"completed": 0, "failed": 0}})
}
func (a *App) taskRetry(w http.ResponseWriter, r *http.Request) {
	if !a.requireAdmin(w, r) {
		return
	}
	var b struct {
		TaskID int64 `json:"taskId"`
	}
	if decodeJSON(r, &b) != nil {
		errorJSON(w, 400, "Invalid request")
		return
	}
	_, err := a.db.Exec(`UPDATE pipeline_queue SET status='pending',error_message=NULL,created_at=unixepoch() WHERE id=?`, b.TaskID)
	if err != nil {
		errorJSON(w, 500, err.Error())
		return
	}
	a.wakeQueue()
	writeJSON(w, 200, map[string]any{"success": true})
}
func (a *App) taskRetryBatch(w http.ResponseWriter, r *http.Request) {
	if !a.requireAdmin(w, r) {
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
	if b.RetryAll {
		result, err := a.db.Exec(`UPDATE pipeline_queue SET status='pending',error_message=NULL,created_at=unixepoch() WHERE status='failed'`)
		if err != nil {
			errorJSON(w, http.StatusInternalServerError, err.Error())
			return
		}
		retried, _ := result.RowsAffected()
		a.wakeQueue()
		writeJSON(w, http.StatusOK, map[string]any{"success": true, "retriedCount": retried})
		return
	}
	for _, id := range b.TaskIDs {
		_, _ = a.db.Exec(`UPDATE pipeline_queue SET status='pending',error_message=NULL,created_at=unixepoch() WHERE id=?`, id)
	}
	a.wakeQueue()
	writeJSON(w, 200, map[string]any{"success": true, "retriedCount": len(b.TaskIDs)})
}

func (a *App) taskDeleteFailed(w http.ResponseWriter, r *http.Request, idText string) {
	if !a.requireAdmin(w, r) {
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
