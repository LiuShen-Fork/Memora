package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// setupPayload is a complete first-run setup, with the password this test is
// about.
func setupPayload(app *App, password string) []byte {
	payload := map[string]any{
		"admin": map[string]any{"email": "admin@example.com", "password": password, "username": "admin"},
		"site":  map[string]any{"title": "Test Gallery"},
		"storage": map[string]any{
			"name":   "Local files",
			"config": map[string]any{"provider": "local", "basePath": app.cfg.LocalPath, "baseUrl": "/storage", "prefix": "photos/"},
		},
		"map": map[string]any{"provider": "maplibre", "token": "map-token", "style": ""},
	}
	data, _ := json.Marshal(payload)
	return data
}

func submitSetup(t *testing.T, app *App, password string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(http.MethodPost, "/api/wizard/submit", bytes.NewReader(setupPayload(app, password)))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	app.ServeHTTP(response, request)
	return response
}

// The password ChronoFrame created its first administrator with is printed in
// that project's guides, so a site set up with it is open to anybody who has
// read one. Setup refuses it, and accepts a password of the person's own.
func TestSetupRefusesThePasswordChronoFrameShippedAsDefault(t *testing.T) {
	app := newTestApp(t)
	response := submitSetup(t, app, formerDefaultPassword)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("setting the site up with the former default password = %d, want it refused", response.Code)
	}
	var users int
	if err := app.db.QueryRow(`SELECT COUNT(*) FROM users`).Scan(&users); err != nil {
		t.Fatal(err)
	}
	if users != 0 {
		t.Fatalf("an administrator was created with the refused password: %d users", users)
	}

	response = submitSetup(t, app, "a-password-of-my-own")
	if response.Code != http.StatusOK {
		t.Fatalf("setting the site up with a password of one's own = %d, %s", response.Code, response.Body.String())
	}
}

// The step that only creates the administrator refuses it too.
func TestTheAdministratorStepRefusesTheFormerDefaultPassword(t *testing.T) {
	app := newTestApp(t)
	body, _ := json.Marshal(map[string]string{"email": "admin@example.com", "password": formerDefaultPassword, "username": "admin"})
	request := httptest.NewRequest(http.MethodPost, "/api/wizard/admin", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	app.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("creating the administrator with the former default password = %d, want it refused", response.Code)
	}
	body, _ = json.Marshal(map[string]string{"email": "admin@example.com", "password": "a-password-of-my-own", "username": "admin"})
	request = httptest.NewRequest(http.MethodPost, "/api/wizard/admin", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	response = httptest.NewRecorder()
	app.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("creating the administrator with a password of one's own = %d, %s", response.Code, response.Body.String())
	}
}
