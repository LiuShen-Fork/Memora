package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func queueConcurrencyRequest(t *testing.T, app *App, method string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var payload []byte
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		payload = encoded
	}
	response := httptest.NewRecorder()
	app.ServeHTTP(response, adminRequest(t, app, method, "/api/queue/concurrency", payload))
	return response
}

func readQueueConcurrency(t *testing.T, app *App) (concurrency, workers int) {
	t.Helper()
	response := queueConcurrencyRequest(t, app, http.MethodGet, nil)
	if response.Code != http.StatusOK {
		t.Fatalf("concurrency status = %d, body = %s", response.Code, response.Body.String())
	}
	var state struct {
		Concurrency int `json:"concurrency"`
		Min         int `json:"min"`
		Max         int `json:"max"`
		Workers     int `json:"workers"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &state); err != nil {
		t.Fatal(err)
	}
	if state.Min != minQueueWorkers || state.Max != maxQueueWorkers {
		t.Fatalf("range = %d..%d, want %d..%d", state.Min, state.Max, minQueueWorkers, maxQueueWorkers)
	}
	return state.Concurrency, state.Workers
}

// Changing the number has to reach the running pool, not just the database: the
// point of the setting is that a backlog starts moving without a restart.
func TestQueueConcurrencyResizesTheRunningPool(t *testing.T) {
	app := newTestApp(t)
	app.startWorkers()
	t.Cleanup(app.stopWorkers)
	if workers := app.workerCount(); workers != 1 {
		t.Fatalf("pool started with %d workers, want the 1 the environment asked for", workers)
	}
	response := queueConcurrencyRequest(t, app, http.MethodPut, map[string]any{"concurrency": 6})
	if response.Code != http.StatusOK {
		t.Fatalf("update status = %d, body = %s", response.Code, response.Body.String())
	}
	if workers := app.workerCount(); workers != 6 {
		t.Fatalf("pool runs %d workers after the change, want 6", workers)
	}
	concurrency, workers := readQueueConcurrency(t, app)
	if concurrency != 6 || workers != 6 {
		t.Fatalf("state = (%d stored, %d running), want (6,6)", concurrency, workers)
	}
	// Shrinking works the same way, and the workers that were retired are not
	// counted any more.
	if response := queueConcurrencyRequest(t, app, http.MethodPut, map[string]any{"concurrency": 2}); response.Code != http.StatusOK {
		t.Fatalf("shrink status = %d, body = %s", response.Code, response.Body.String())
	}
	if workers := app.workerCount(); workers != 2 {
		t.Fatalf("pool runs %d workers after shrinking, want 2", workers)
	}
	// The setting outlives the process, so the next start reads the same number
	// once the environment has nothing to say.
	var stored any
	if !app.readSetting("system", "queueConcurrency", &stored) || stored != float64(2) {
		t.Fatalf("stored concurrency = %#v, want 2", stored)
	}
}

func TestQueueConcurrencyRefusesValuesOutsideTheRange(t *testing.T) {
	app := newTestApp(t)
	for _, value := range []int{0, -1, 9, 100} {
		response := queueConcurrencyRequest(t, app, http.MethodPut, map[string]any{"concurrency": value})
		if response.Code != http.StatusBadRequest {
			t.Fatalf("concurrency %d was accepted: %d %s", value, response.Code, response.Body.String())
		}
	}
	if response := queueConcurrencyRequest(t, app, http.MethodPut, map[string]any{}); response.Code != http.StatusBadRequest {
		t.Fatalf("a missing value was accepted: %d %s", response.Code, response.Body.String())
	}
	// The same range holds for the settings endpoint, which is the other way in.
	batch := httptest.NewRecorder()
	body := []byte(`{"updates":[{"namespace":"system","key":"queueConcurrency","value":99}]}`)
	app.ServeHTTP(batch, adminRequest(t, app, http.MethodPut, "/api/system/settings/batch", body))
	if !bytes.Contains(batch.Body.Bytes(), []byte("between")) {
		t.Fatalf("settings batch accepted an out-of-range value: %d %s", batch.Code, batch.Body.String())
	}
	if concurrency, _ := readQueueConcurrency(t, app); concurrency != defaultQueueWorkers {
		t.Fatalf("stored concurrency = %d after refused writes, want the default %d", concurrency, defaultQueueWorkers)
	}
}

func TestQueueConcurrencyNeedsAnAdministrator(t *testing.T) {
	app := newTestApp(t)
	anonymous := httptest.NewRecorder()
	app.ServeHTTP(anonymous, httptest.NewRequest(http.MethodGet, "/api/queue/concurrency", nil))
	if anonymous.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous read = %d, want 401", anonymous.Code)
	}
	anonymous = httptest.NewRecorder()
	app.ServeHTTP(anonymous, httptest.NewRequest(http.MethodPut, "/api/queue/concurrency", bytes.NewReader([]byte(`{"concurrency":3}`))))
	if anonymous.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous write = %d, want 401", anonymous.Code)
	}
}

// An operator's environment is a starting point, not an override of everything
// the dashboard does afterwards.
func TestQueueWorkersComeFromTheEnvironmentWhenItIsSet(t *testing.T) {
	app := newTestApp(t)
	app.cfg.WorkerCount = 0
	if workers := app.initialQueueWorkers(); workers != defaultQueueWorkers {
		t.Fatalf("initial workers = %d, want the default %d", workers, defaultQueueWorkers)
	}
	app.setSetting("system", "queueConcurrency", 3)
	if workers := app.initialQueueWorkers(); workers != 3 {
		t.Fatalf("initial workers = %d, want the stored 3", workers)
	}
	// CFRAME_WORKERS wins at startup and is clamped to what the queue can run.
	app.cfg.WorkerCount = 90
	if workers := app.initialQueueWorkers(); workers != maxQueueWorkers {
		t.Fatalf("initial workers = %d, want the ceiling %d", workers, maxQueueWorkers)
	}
	app.cfg.WorkerCount = 2
	if workers := app.initialQueueWorkers(); workers != 2 {
		t.Fatalf("initial workers = %d, want the 2 the environment asked for", workers)
	}
}

// Shutting down has to end even when a worker is in the middle of a task that
// outlives the drain, or the process would never exit.
func TestStoppingWorkersEndsWithIdleWorkers(t *testing.T) {
	app := newTestApp(t)
	app.cfg.WorkerCount = 4
	app.startWorkers()
	done := make(chan struct{})
	go func() {
		app.stopWorkers()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("stopping the workers did not end")
	}
}
