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
	rows, _ := a.db.Query(`SELECT id,payload,priority,attempts,max_attempts,status,status_stage,error_message,created_at,completed_at FROM pipeline_queue ORDER BY id DESC LIMIT 500`)
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
	if rows != nil {
		rows.Close()
	}
	writeJSON(w, 200, out)
}
func (a *App) taskClear(w http.ResponseWriter, r *http.Request) {
	if !a.requireAdmin(w, r) {
		return
	}
	_, _ = a.db.Exec(`DELETE FROM pipeline_queue WHERE status IN ('completed','failed')`)
	writeJSON(w, 200, map[string]any{"success": true})
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
		TaskIDs []int64 `json:"taskIds"`
	}
	if decodeJSON(r, &b) != nil {
		errorJSON(w, 400, "Invalid request")
		return
	}
	for _, id := range b.TaskIDs {
		_, _ = a.db.Exec(`UPDATE pipeline_queue SET status='pending',error_message=NULL,created_at=unixepoch() WHERE id=?`, id)
	}
	a.wakeQueue()
	writeJSON(w, 200, map[string]any{"success": true})
}
