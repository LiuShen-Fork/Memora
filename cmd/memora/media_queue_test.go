package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// A retry has to wait without pretending the task was just created: the delay
// belongs in the schedule, not in the arrival time.
func TestRetrySchedulesTheNextAttemptWithoutMovingTheArrival(t *testing.T) {
	app := newTestApp(t)
	arrived := time.Now().Unix() - 600
	result, err := app.db.Exec(`INSERT INTO pipeline_queue(payload,status,created_at) VALUES('{"type":"photo"}','in-stages',?)`, arrived)
	if err != nil {
		t.Fatal(err)
	}
	id, _ := result.LastInsertId()
	app.failTask(id, "temporary failure")
	var status string
	var attempts int
	var createdAt, availableAt int64
	if err := app.db.QueryRow(`SELECT status,attempts,created_at,available_at FROM pipeline_queue WHERE id=?`, id).Scan(&status, &attempts, &createdAt, &availableAt); err != nil {
		t.Fatal(err)
	}
	if status != "pending" || attempts != 1 {
		t.Fatalf("after a retry = (%q,%d), want (pending,1)", status, attempts)
	}
	if createdAt != arrived {
		t.Fatalf("created_at = %d after a retry, want the original %d", createdAt, arrived)
	}
	// The first retry waits a second, give or take the jitter.
	delay := availableAt - time.Now().Unix()
	if delay < 1 || delay > 2 {
		t.Fatalf("retry scheduled %ds out, want between 1 and 2", delay)
	}
	task, err := app.claimTask()
	if err != nil {
		t.Fatal(err)
	}
	if task != nil {
		t.Fatalf("claimed task %d while its retry delay was still running", task.ID)
	}
	// Once the delay has elapsed the task is handed out again, and the wait does
	// not push it behind a task that arrived later but is due now.
	if _, err := app.db.Exec(`UPDATE pipeline_queue SET available_at=unixepoch()-1 WHERE id=?`, id); err != nil {
		t.Fatal(err)
	}
	if _, err := app.db.Exec(`INSERT INTO pipeline_queue(payload,status) VALUES('{"type":"photo"}','pending')`); err != nil {
		t.Fatal(err)
	}
	task, err = app.claimTask()
	if err != nil {
		t.Fatal(err)
	}
	if task == nil || task.ID != id {
		t.Fatalf("claimed %v, want the retry that has been waiting longest", task)
	}
}

func TestRetryDelayDoublesUpToHalfAMinute(t *testing.T) {
	want := map[int]int64{1: 1, 2: 2, 3: 4, 4: 8, 5: 16, 6: 30, 10: 30}
	for attempts, base := range want {
		delay := retryDelay(attempts)
		if delay < base || delay > base+base/4 {
			t.Fatalf("retryDelay(%d) = %d, want between %d and %d", attempts, delay, base, base+base/4)
		}
	}
	// The wait is spread out, so a batch that failed together does not return
	// together. Ten samples of a nine-value range colliding every time would
	// mean the jitter is not there at all.
	seen := map[int64]bool{}
	for i := 0; i < 10; i++ {
		seen[retryDelay(6)] = true
	}
	if len(seen) < 2 {
		t.Fatalf("retryDelay(6) returned one value over ten calls: %v", seen)
	}
}

// An upgrading install keeps the tasks it already has. The one that was waiting
// out a backoff keeps waiting for the moment it was scheduled for — under the
// old build that moment was written into created_at — and nothing else is held
// back by the column arriving.
func TestQueueUpgradeSchedulesExistingTasksAtTheirArrival(t *testing.T) {
	app := newSchemaTestApp(t)
	if _, err := app.db.Exec(`CREATE TABLE pipeline_queue (id INTEGER PRIMARY KEY AUTOINCREMENT, payload TEXT NOT NULL, priority INTEGER NOT NULL DEFAULT 0, attempts INTEGER NOT NULL DEFAULT 0, max_attempts INTEGER NOT NULL DEFAULT 3, status TEXT NOT NULL DEFAULT 'pending', status_stage TEXT, error_message TEXT, created_at INTEGER NOT NULL DEFAULT (unixepoch()), completed_at INTEGER)`); err != nil {
		t.Fatal(err)
	}
	now := time.Now().Unix()
	if _, err := app.db.Exec(`INSERT INTO pipeline_queue(id,payload,created_at) VALUES(1,'{"type":"photo"}',?), (2,'{"type":"photo"}',?)`, now-100, now+100); err != nil {
		t.Fatal(err)
	}
	if err := app.applySchemaStep(queueAvailableAtStep); err != nil {
		t.Fatal(err)
	}
	var availableAt, createdAt int64
	for id, want := range map[int64]int64{1: now - 100, 2: now + 100} {
		if err := app.db.QueryRow(`SELECT available_at,created_at FROM pipeline_queue WHERE id=?`, id).Scan(&availableAt, &createdAt); err != nil {
			t.Fatal(err)
		}
		if availableAt != createdAt || availableAt != want {
			t.Fatalf("task %d = (available_at %d, created_at %d), want both %d", id, availableAt, createdAt, want)
		}
	}
	// A task inserted by the new build is due immediately rather than waiting
	// for a schedule that was never set.
	result, err := app.db.Exec(`INSERT INTO pipeline_queue(payload,status) VALUES('{"type":"photo"}','pending')`)
	if err != nil {
		t.Fatal(err)
	}
	id, _ := result.LastInsertId()
	if err := app.db.QueryRow(`SELECT available_at FROM pipeline_queue WHERE id=?`, id).Scan(&availableAt); err != nil {
		t.Fatal(err)
	}
	if availableAt != 0 {
		t.Fatalf("a new task is scheduled at %d, want 0 (due now)", availableAt)
	}
}

// The dashboard needs to tell a task that is waiting from one that is ready,
// and it reads that from the task list rather than from the database.
func TestTaskListReportsWhenAWaitingTaskComesBack(t *testing.T) {
	app := newTestApp(t)
	now := time.Now().Unix()
	if _, err := app.db.Exec(`INSERT INTO pipeline_queue(payload,status,available_at) VALUES('{"type":"photo"}','pending',?),('{"type":"photo"}','completed',?)`, now+120, now-120); err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	app.ServeHTTP(response, adminRequest(t, app, http.MethodGet, "/api/queue/task/list", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("task list status = %d, body = %s", response.Code, response.Body.String())
	}
	var body struct {
		Data []struct {
			ID          int64  `json:"id"`
			Status      string `json:"status"`
			AvailableAt string `json:"availableAt"`
		} `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Data) != 2 {
		t.Fatalf("task list returned %d rows, want 2: %s", len(body.Data), response.Body.String())
	}
	rows := map[string]int64{}
	for _, task := range body.Data {
		rows[task.Status] = task.ID
		if task.Status == "pending" {
			if task.AvailableAt == "" {
				t.Fatalf("waiting task = %+v, want a return time", task)
			}
			if parsed, err := time.Parse(time.RFC3339, task.AvailableAt); err != nil {
				t.Fatalf("availableAt %q is not a time: %v", task.AvailableAt, err)
			} else if parsed.Unix() != now+120 {
				t.Fatalf("availableAt = %v, want %v", parsed.Unix(), now+120)
			}
		}
		// A task with nothing to wait for reports no time at all, rather than
		// the epoch it was inserted with.
		if task.Status == "completed" && task.AvailableAt != "" {
			t.Fatalf("ready task = %+v, want no return time", task)
		}
	}
	if _, ok := rows["pending"]; !ok {
		t.Fatalf("the pending task is missing from the list: %s", response.Body.String())
	}
	if _, ok := rows["completed"]; !ok {
		t.Fatalf("the completed task is missing from the list: %s", response.Body.String())
	}
}
