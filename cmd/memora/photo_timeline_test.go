package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

type timelineDay struct {
	Date  string `json:"date"`
	Count int    `json:"count"`
}

func photoTimeline(t *testing.T, app *App, query string) []timelineDay {
	t.Helper()
	response := httptest.NewRecorder()
	app.ServeHTTP(response, adminRequest(t, app, http.MethodGet, "/api/photos/timeline"+query, nil))
	if response.Code != http.StatusOK {
		t.Fatalf("/api/photos/timeline%s = %d, want the timeline", query, response.Code)
	}
	var body struct {
		Days []timelineDay `json:"days"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("timeline is not readable: %v, %s", err, response.Body.String())
	}
	return body.Days
}

// The calendar on the dashboard counts photos, and the count belongs to the
// dates rather than to whatever the browser happened to have loaded: a day
// with two photos is two, whether or not any of them were fetched.
func TestPhotoTimelineCountsPhotosByDay(t *testing.T) {
	app := newTestApp(t)
	if _, err := app.db.Exec(`INSERT INTO photos(id,date_taken,last_modified) VALUES
		('a','2026-03-01T09:00:00Z','2026-03-01T09:00:00Z'),
		('b','2026-03-01T21:30:00Z','2026-03-01T21:30:00Z'),
		('c','2026-03-02T08:00:00Z','2026-03-02T08:00:00Z'),
		('d','2025-12-31T23:59:00Z','2025-12-31T23:59:00Z')`); err != nil {
		t.Fatal(err)
	}

	days := photoTimeline(t, app, "")
	want := []timelineDay{
		{Date: "2025-12-31", Count: 1},
		{Date: "2026-03-01", Count: 2},
		{Date: "2026-03-02", Count: 1},
	}
	if len(days) != len(want) {
		t.Fatalf("timeline = %+v, want %+v", days, want)
	}
	for i, day := range want {
		if days[i] != day {
			t.Fatalf("timeline = %+v, want %+v", days, want)
		}
	}
}

// A photo the queue has not dated yet still arrived on a day, so the calendar
// counts it under the day it was written rather than dropping it.
func TestPhotoTimelineFallsBackToLastModified(t *testing.T) {
	app := newTestApp(t)
	if _, err := app.db.Exec(`INSERT INTO photos(id,date_taken,last_modified) VALUES
		('undated',NULL,'2026-05-04T10:00:00Z'),
		('empty','','2026-05-05T10:00:00Z')`); err != nil {
		t.Fatal(err)
	}

	days := photoTimeline(t, app, "")
	want := []timelineDay{
		{Date: "2026-05-04", Count: 1},
		{Date: "2026-05-05", Count: 1},
	}
	if len(days) != len(want) {
		t.Fatalf("timeline = %+v, want an entry for each undated photo", days)
	}
	for i, day := range want {
		if days[i] != day {
			t.Fatalf("timeline = %+v, want %+v", days, want)
		}
	}
}

// The calendar asks for one year at a time, and the days outside it should not
// travel.
func TestPhotoTimelineHonoursARange(t *testing.T) {
	app := newTestApp(t)
	if _, err := app.db.Exec(`INSERT INTO photos(id,date_taken) VALUES
		('a','2024-06-01T09:00:00Z'),
		('b','2025-06-01T09:00:00Z'),
		('c','2026-06-01T09:00:00Z')`); err != nil {
		t.Fatal(err)
	}

	days := photoTimeline(t, app, "?from=2025-01-01&to=2025-12-31")
	if len(days) != 1 || days[0].Date != "2025-06-01" || days[0].Count != 1 {
		t.Fatalf("timeline for 2025 = %+v, want only the one day in that year", days)
	}
	if days := photoTimeline(t, app, "?from=2027-01-01"); len(days) != 0 {
		t.Fatalf("timeline from 2027 = %+v, want nothing", days)
	}
}
