package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// wizardCall sends one request to the setup API.
func wizardCall(t *testing.T, app *App, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	request := httptest.NewRequest(method, "/api/wizard/"+path, reader)
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	response := httptest.NewRecorder()
	app.ServeHTTP(response, request)
	return response
}

// Once setup has finished, every setup endpoint is closed: the requests that
// create the administrator or repoint storage would otherwise be an anonymous
// way to take the site over.
func TestSetupClosesTheWizardOnceItIsComplete(t *testing.T) {
	app := newTestApp(t)
	if response := submitSetup(t, app, "a-password-of-my-own"); response.Code != http.StatusOK {
		t.Fatalf("setting the site up = %d, %s", response.Code, response.Body.String())
	}
	var passwordBefore string
	if err := app.db.QueryRow(`SELECT COALESCE(password,'') FROM users WHERE email='admin@example.com'`).Scan(&passwordBefore); err != nil {
		t.Fatal(err)
	}

	attempts := []struct {
		path string
		body string
	}{
		{"admin", `{"email":"admin@example.com","password":"attacker-password","username":"admin"}`},
		{"site", `{"title":"Hijacked"}`},
		{"storage", `{"name":"Attacker","config":{"provider":"local","basePath":"/tmp/attacker","baseUrl":"/storage","prefix":"photos/"}}`},
		{"map", `{"provider":"maplibre","token":"attacker-token"}`},
		{"complete", `{}`},
		{"submit", string(setupPayload(app, "attacker-password"))},
	}
	for _, attempt := range attempts {
		response := wizardCall(t, app, http.MethodPost, attempt.path, attempt.body)
		if response.Code != http.StatusForbidden {
			t.Fatalf("POST /api/wizard/%s after setup = %d, want it refused", attempt.path, response.Code)
		}
	}
	if response := wizardCall(t, app, http.MethodGet, "schema?namespace=admin", ""); response.Code != http.StatusForbidden {
		t.Fatalf("GET /api/wizard/schema after setup = %d, want it refused", response.Code)
	}

	// The refused requests left the finished site as it was.
	var title string
	if err := app.db.QueryRow(`SELECT value FROM settings WHERE namespace='app' AND key='title'`).Scan(&title); err != nil || title != "Test Gallery" {
		t.Fatalf("site title after the refused requests = %q (%v), want Test Gallery", title, err)
	}
	var passwordAfter string
	if err := app.db.QueryRow(`SELECT COALESCE(password,'') FROM users WHERE email='admin@example.com'`).Scan(&passwordAfter); err != nil || passwordAfter != passwordBefore {
		t.Fatalf("administrator password changed after the refused requests (%v)", err)
	}
	var storageConfigs int
	if err := app.db.QueryRow(`SELECT COUNT(*) FROM settings_storage_providers`).Scan(&storageConfigs); err != nil || storageConfigs != 1 {
		t.Fatalf("storage configurations after the refused requests = %d (%v), want 1", storageConfigs, err)
	}
	var firstLaunch any
	if !app.readSetting("system", "firstLaunch", &firstLaunch) || firstLaunch != false {
		t.Fatalf("firstLaunch after the refused requests = %#v, want false", firstLaunch)
	}
}

// Setup itself still works while firstLaunch is true, and the same endpoints
// close as soon as the final step has run.
func TestWizardStaysOpenUntilSetupCompletes(t *testing.T) {
	app := newTestApp(t)

	if response := wizardCall(t, app, http.MethodGet, "schema?namespace=admin", ""); response.Code != http.StatusOK {
		t.Fatalf("setup schema before setup = %d, %s", response.Code, response.Body.String())
	}
	if response := wizardCall(t, app, http.MethodPost, "admin", `{"email":"admin@example.com","password":"a-password-of-my-own","username":"admin"}`); response.Code != http.StatusOK {
		t.Fatalf("creating the administrator before setup = %d, %s", response.Code, response.Body.String())
	}
	if response := wizardCall(t, app, http.MethodPost, "site", `{"title":"Test Gallery"}`); response.Code != http.StatusOK {
		t.Fatalf("saving the site before setup = %d, %s", response.Code, response.Body.String())
	}
	if response := wizardCall(t, app, http.MethodPost, "complete", ""); response.Code != http.StatusOK {
		t.Fatalf("finishing setup = %d, %s", response.Code, response.Body.String())
	}
	if response := wizardCall(t, app, http.MethodPost, "admin", `{"email":"admin@example.com","password":"attacker-password","username":"admin"}`); response.Code != http.StatusForbidden {
		t.Fatalf("creating an administrator after setup = %d, want it refused", response.Code)
	}
}

// An unauthenticated setup endpoint is worth guessing at, so one minute's worth
// of attempts is bounded even before the first step has finished.
func TestTheWizardBoundsHowOftenItIsTried(t *testing.T) {
	app := newTestApp(t)
	for i := 0; i < wizardAttemptLimit; i++ {
		response := wizardCall(t, app, http.MethodGet, "schema?namespace=admin", "")
		if response.Code != http.StatusOK {
			t.Fatalf("setup request %d = %d, %s", i+1, response.Code, response.Body.String())
		}
	}
	if response := wizardCall(t, app, http.MethodGet, "schema?namespace=admin", ""); response.Code != http.StatusTooManyRequests {
		t.Fatalf("one attempt past the setup limit = %d, want 429", response.Code)
	}
}
