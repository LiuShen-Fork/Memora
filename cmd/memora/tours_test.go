package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func tourRecords(t *testing.T, app *App, request *http.Request) map[string]string {
	t.Helper()
	response := httptest.NewRecorder()
	app.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("tours = %d, %s", response.Code, response.Body.String())
	}
	var records map[string]string
	if err := json.Unmarshal(response.Body.Bytes(), &records); err != nil {
		t.Fatalf("tour records are not readable: %v, %s", err, response.Body.String())
	}
	return records
}

func setTour(t *testing.T, app *App, body string) *httptest.ResponseRecorder {
	t.Helper()
	response := httptest.NewRecorder()
	app.ServeHTTP(response, adminRequest(t, app, http.MethodPut, "/api/system/tours", []byte(body)))
	return response
}

// A site that has never shown the guides says so, and every guide it knows is
// waiting.
func TestToursStartWaitingToBeShown(t *testing.T) {
	app := newTestApp(t)
	records := tourRecords(t, app, adminRequest(t, app, http.MethodGet, "/api/system/tours", nil))
	if len(records) != len(tourIDs) {
		t.Fatalf("records = %v, want one per guide", records)
	}
	for id, status := range records {
		if status != "pending" {
			t.Fatalf("%s = %q, want it waiting to be shown", id, status)
		}
	}
}

// Answering one guide leaves the others alone, and a skipped guide is not shown
// again.
func TestToursRememberOneAnswerAtATime(t *testing.T) {
	app := newTestApp(t)
	response := setTour(t, app, `{"action":"set","id":"photos","status":"skipped"}`)
	if response.Code != http.StatusOK {
		t.Fatalf("answering a guide = %d, %s", response.Code, response.Body.String())
	}
	var records map[string]string
	if err := json.Unmarshal(response.Body.Bytes(), &records); err != nil {
		t.Fatal(err)
	}
	if records["photos"] != "skipped" {
		t.Fatalf("photos = %q, want the answer that was just given", records["photos"])
	}
	if records["albums"] != "pending" {
		t.Fatalf("albums = %q, want it left waiting", records["albums"])
	}
	// The answer has to survive the request that gave it.
	records = tourRecords(t, app, adminRequest(t, app, http.MethodGet, "/api/system/tours", nil))
	if records["photos"] != "skipped" {
		t.Fatalf("photos = %q, want the answer to have been kept", records["photos"])
	}
}

func TestToursResetPutsThemAllBack(t *testing.T) {
	app := newTestApp(t)
	if response := setTour(t, app, `{"action":"set","id":"photos","status":"completed"}`); response.Code != http.StatusOK {
		t.Fatalf("answering a guide = %d", response.Code)
	}
	records := tourRecords(t, app, adminRequest(t, app, http.MethodPut, "/api/system/tours", []byte(`{"action":"reset"}`)))
	for id, status := range records {
		if status != "pending" {
			t.Fatalf("%s = %q, want every guide waiting again", id, status)
		}
	}
}

func TestToursRefuseWhatTheyDoNotKnow(t *testing.T) {
	app := newTestApp(t)
	for _, body := range []string{
		`{"action":"set","id":"nowhere","status":"completed"}`,
		`{"action":"set","id":"photos","status":"halfway"}`,
		`{"action":"forget"}`,
		`not json`,
	} {
		if response := setTour(t, app, body); response.Code != http.StatusBadRequest {
			t.Fatalf("%s = %d, want it refused", body, response.Code)
		}
	}
	if _, err := app.db.Exec(`INSERT INTO users(name,email,password,created_at,is_admin) VALUES('visitor','visitor@example.com','password',unixepoch(),0)`); err != nil {
		t.Fatal(err)
	}
	var visitorID int64
	if err := app.db.QueryRow(`SELECT id FROM users WHERE email='visitor@example.com'`).Scan(&visitorID); err != nil {
		t.Fatal(err)
	}
	signedIn := httptest.NewRecorder()
	app.setSession(signedIn, visitorID)
	request := httptest.NewRequest(http.MethodGet, "/api/system/tours", nil)
	request.AddCookie(signedIn.Result().Cookies()[0])
	response := httptest.NewRecorder()
	app.ServeHTTP(response, request)
	if response.Code != http.StatusForbidden {
		t.Fatalf("a visitor reading the tours = %d, want it refused", response.Code)
	}
	response = httptest.NewRecorder()
	app.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/system/tours", nil))
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("a signed-out reader = %d, want it refused", response.Code)
	}
}
