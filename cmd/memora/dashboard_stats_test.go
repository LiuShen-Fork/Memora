package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"
)

type dashboardStatsResponse struct {
	Queue struct {
		Pending          int `json:"pending"`
		Failed           int `json:"failed"`
		OldestWaitSecond int `json:"oldestWaitSeconds"`
	} `json:"queue"`
	Storage struct {
		TotalSize      float64 `json:"totalSize"`
		ActiveProvider *struct {
			Provider string `json:"provider"`
			Name     string `json:"name"`
		} `json:"activeProvider"`
	} `json:"storage"`
}

func dashboardStats(t *testing.T, app *App) dashboardStatsResponse {
	t.Helper()
	response := httptest.NewRecorder()
	app.ServeHTTP(response, adminRequest(t, app, http.MethodGet, "/api/system/stats", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("/api/system/stats = %d, want the statistics", response.Code)
	}
	var stats dashboardStatsResponse
	if err := json.Unmarshal(response.Body.Bytes(), &stats); err != nil {
		t.Fatalf("statistics are not readable: %v, %s", err, response.Body.String())
	}
	return stats
}

// The dashboard says how much work is waiting, how much gave up, and how long
// the queue has been sitting on it.
func TestDashboardStatsSummariseTheQueue(t *testing.T) {
	app := newTestApp(t)
	now := time.Now().Unix()
	if _, err := app.db.Exec(`INSERT INTO pipeline_queue(payload,status,created_at,available_at) VALUES
		('{"type":"photo"}','pending',?,?),
		('{"type":"photo"}','pending',?,?),
		('{"type":"photo"}','failed',?,?),
		('{"type":"photo"}','completed',?,?)`,
		now-600, now-600,
		now-30, now-30,
		now-10, now-10,
		now-5, now-5); err != nil {
		t.Fatal(err)
	}
	stats := dashboardStats(t, app)
	if stats.Queue.Pending != 2 || stats.Queue.Failed != 1 {
		t.Fatalf("queue summary = %+v, want two pending and one failed", stats.Queue)
	}
	if stats.Queue.OldestWaitSecond < 595 || stats.Queue.OldestWaitSecond > 605 {
		t.Fatalf("oldestWaitSeconds = %d, want about 600", stats.Queue.OldestWaitSecond)
	}
}

// A task waiting out its retry delay has not been waiting on the queue, so a
// scheduled retry must not read as backlog.
func TestDashboardStatsIgnoreScheduledRetries(t *testing.T) {
	app := newTestApp(t)
	now := time.Now().Unix()
	if _, err := app.db.Exec(`INSERT INTO pipeline_queue(payload,status,created_at,available_at) VALUES
		('{"type":"photo"}','pending',?,?),
		('{"type":"photo"}','pending',?,?)`,
		now-300, now-300,
		now-10, now+900); err != nil {
		t.Fatal(err)
	}
	if wait := dashboardStats(t, app).Queue.OldestWaitSecond; wait < 295 || wait > 305 {
		t.Fatalf("oldestWaitSeconds = %d, want about 300 rather than a negative wait", wait)
	}
	// With nothing due at all the queue is not behind, whatever the retries say.
	if _, err := app.db.Exec(`UPDATE pipeline_queue SET available_at=unixepoch()+900`); err != nil {
		t.Fatal(err)
	}
	if wait := dashboardStats(t, app).Queue.OldestWaitSecond; wait != 0 {
		t.Fatalf("oldestWaitSeconds = %d, want no wait while every task is scheduled ahead", wait)
	}
}

// The dashboard names the storage that is actually in use, whether it was set
// up from the dashboard or from the environment.
func TestDashboardStatsNameTheActiveStorage(t *testing.T) {
	app := newTestApp(t)
	stats := dashboardStats(t, app)
	if stats.Storage.ActiveProvider == nil || stats.Storage.ActiveProvider.Provider != "local" {
		t.Fatalf("activeProvider = %+v, want local storage", stats.Storage.ActiveProvider)
	}
	if stats.Storage.ActiveProvider.Name != "" {
		t.Fatalf("name = %q, want none for storage that was never configured", stats.Storage.ActiveProvider.Name)
	}
	id, err := app.insertStorageConfig("Archive bucket", "s3", map[string]any{
		"provider": "s3", "endpoint": "https://s3.example.com", "bucket": "photos", "region": "us-east-1",
		"accessKeyId": "key", "secretAccessKey": "secret", "prefix": "photos/",
	})
	if err != nil {
		t.Fatal(err)
	}
	app.setSetting("storage", "provider", id)
	active := dashboardStats(t, app).Storage.ActiveProvider
	if active == nil || active.Provider != "s3" || active.Name != "Archive bucket" {
		t.Fatalf("activeProvider = %+v, want the configured bucket", active)
	}
	// A configuration deleted from under the setting is left un-named rather
	// than reported as whatever provider happens to be configured.
	if _, err := app.db.Exec(`DELETE FROM settings_storage_providers WHERE id=?`, id); err != nil {
		t.Fatal(err)
	}
	if active := dashboardStats(t, app).Storage.ActiveProvider; active == nil || active.Name != "" {
		t.Fatalf("activeProvider = %+v, want the provider alone", active)
	}
}

type recentTasksResponse struct {
	Tasks []struct {
		ID           int64  `json:"id"`
		Type         string `json:"type"`
		Status       string `json:"status"`
		StatusStage  string `json:"statusStage"`
		ErrorMessage string `json:"errorMessage"`
		CreatedAt    string `json:"createdAt"`
		CompletedAt  string `json:"completedAt"`
		FileName     string `json:"fileName"`
		Photo        *struct {
			ID            string `json:"id"`
			Title         string `json:"title"`
			ThumbnailURL  string `json:"thumbnailUrl"`
			ThumbnailHash string `json:"thumbnailHash"`
			OriginalURL   string `json:"originalUrl"`
		} `json:"photo"`
	} `json:"tasks"`
}

func recentTasks(t *testing.T, app *App, request *http.Request) recentTasksResponse {
	t.Helper()
	response := httptest.NewRecorder()
	app.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("/api/queue/recent = %d, want the activity list: %s", response.Code, response.Body.String())
	}
	var activity recentTasksResponse
	if err := json.Unmarshal(response.Body.Bytes(), &activity); err != nil {
		t.Fatalf("activity is not readable: %v, %s", err, response.Body.String())
	}
	return activity
}

// The dashboard shows what the queue did last, each entry named by the photo it
// was working on where there is one.
func TestRecentTasksListWhatTheQueueTouched(t *testing.T) {
	app := newTestApp(t)
	if _, err := app.db.Exec(`INSERT INTO photos(id,title,storage_key,original_url,thumbnail_url,thumbnail_hash) VALUES('holiday','Holiday','photos/holiday.jpg','/storage/photos/holiday.jpg','/storage/photos/thumbnails/holiday.webp','hash')`); err != nil {
		t.Fatal(err)
	}
	now := time.Now().Unix()
	if _, err := app.db.Exec(`INSERT INTO pipeline_queue(payload,status,status_stage,error_message,created_at,completed_at,available_at) VALUES
		('{"type":"photo","storageKey":"photos/holiday.jpg"}','completed','exif',NULL,?,?,?),
		('{"type":"photo-reverse-geocoding","photoId":"holiday"}','failed','reverse-geocoding','no geocoder',?,?,?),
		('{"type":"photo","storageKey":"photos/gone.jpg"}','pending',NULL,NULL,?,NULL,?)`,
		now-10, now-5, now-10,
		now-20, now-15, now-20,
		now-30, now-30); err != nil {
		t.Fatal(err)
	}
	activity := recentTasks(t, app, adminRequest(t, app, http.MethodGet, "/api/queue/recent", nil))
	if len(activity.Tasks) != 3 {
		t.Fatalf("got %d tasks, want the three that were queued", len(activity.Tasks))
	}
	newest := activity.Tasks[0]
	if newest.Status != "completed" || newest.StatusStage != "exif" || newest.CompletedAt == "" {
		t.Fatalf("newest task = %+v, want its stage and completion time", newest)
	}
	if newest.Photo == nil || newest.Photo.ID != "holiday" || newest.Photo.Title != "Holiday" ||
		newest.Photo.ThumbnailURL != "/storage/photos/thumbnails/holiday.webp" || newest.Photo.ThumbnailHash != "hash" {
		t.Fatalf("newest task = %+v, want the photo it produced", newest.Photo)
	}
	// An upload names its photo by the storage key the photo is named after.
	if newest.FileName != "holiday.jpg" || newest.CreatedAt == "" {
		t.Fatalf("newest task = %+v, want its upload's file name and the time it was queued", newest)
	}

	failed := activity.Tasks[1]
	if failed.Status != "failed" || failed.ErrorMessage != "no geocoder" {
		t.Fatalf("failed task = %+v, want its error", failed)
	}
	// A task that names its photo outright is joined to it the same way.
	if failed.Photo == nil || failed.Photo.ID != "holiday" {
		t.Fatalf("failed task = %+v, want the photo it names", failed.Photo)
	}
	if failed.FileName != "" {
		t.Fatalf("failed task = %+v, want no file name when the task carries no upload", failed)
	}

	// A photo that is not there yet has no picture to show.
	waiting := activity.Tasks[2]
	if waiting.Status != "pending" || waiting.FileName != "gone.jpg" || waiting.Photo != nil {
		t.Fatalf("waiting task = %+v, want a file name and no photo", waiting)
	}
}

// The list is short enough to read at a glance.
func TestRecentTasksListStopsAtFive(t *testing.T) {
	app := newTestApp(t)
	now := time.Now().Unix()
	for i := int64(0); i < 8; i++ {
		if _, err := app.db.Exec(`INSERT INTO pipeline_queue(payload,status,created_at,available_at) VALUES('{"type":"photo"}','completed',?,?)`, now+i, now+i); err != nil {
			t.Fatal(err)
		}
	}
	activity := recentTasks(t, app, adminRequest(t, app, http.MethodGet, "/api/queue/recent", nil))
	if len(activity.Tasks) != recentTaskLimit {
		t.Fatalf("got %d tasks, want %d", len(activity.Tasks), recentTaskLimit)
	}
	for i, task := range activity.Tasks {
		want := strconv.FormatInt(now+7-int64(i), 10)
		if got := strconv.FormatInt(taskTime(task.CreatedAt), 10); got != want {
			t.Fatalf("task %d was created at %s, want %s — newest first", i, got, want)
		}
	}
}

func taskTime(createdAt string) int64 {
	parsed, err := time.Parse(time.RFC3339, createdAt)
	if err != nil {
		return 0
	}
	return parsed.Unix()
}

// The activity list is part of the dashboard, which is signed-in only.
func TestRecentTasksNeedASession(t *testing.T) {
	app := newTestApp(t)
	response := httptest.NewRecorder()
	app.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/queue/recent", nil))
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("/api/queue/recent = %d without a session, want 401", response.Code)
	}
}
