package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// administrator creates the first administrator of a fresh site the way the
// setup wizard does, and answers with the id the session will name.
func administrator(t *testing.T, app *App, password string) int64 {
	t.Helper()
	if err := app.upsertWizardAdmin("admin@example.com", password, "admin"); err != nil {
		t.Fatal(err)
	}
	var id int64
	if err := app.db.QueryRow(`SELECT id FROM users WHERE email='admin@example.com'`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func changePassword(t *testing.T, app *App, id int64, current, next string) *httptest.ResponseRecorder {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"currentPassword": current, "newPassword": next})
	request := httptest.NewRequest(http.MethodPut, "/api/profile/password", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	signedIn := httptest.NewRecorder()
	app.setSession(signedIn, id)
	request.AddCookie(signedIn.Result().Cookies()[0])
	response := httptest.NewRecorder()
	app.ServeHTTP(response, request)
	return response
}

func storedPassword(t *testing.T, app *App, id int64) string {
	t.Helper()
	var hash string
	if err := app.db.QueryRow(`SELECT COALESCE(password,'') FROM users WHERE id=?`, id).Scan(&hash); err != nil {
		t.Fatal(err)
	}
	return hash
}

// A password of one's own is taken, and the one that was in use is asked for
// first so a borrowed session cannot take the account over.
func TestChangingAPasswordNeedsTheOneInUse(t *testing.T) {
	app := newTestApp(t)
	id := administrator(t, app, "first-password")

	if response := changePassword(t, app, id, "not-the-password", "second-password"); response.Code != http.StatusForbidden {
		t.Fatalf("changing the password with the wrong current one = %d, want it refused", response.Code)
	}
	if !verifyPassword(storedPassword(t, app, id), "first-password") {
		t.Fatal("the password in use was replaced by a request that did not know it")
	}

	if response := changePassword(t, app, id, "first-password", "second-password"); response.Code != http.StatusOK {
		t.Fatalf("changing the password with the one in use = %d, %s", response.Code, response.Body.String())
	}
	hash := storedPassword(t, app, id)
	if verifyPassword(hash, "first-password") {
		t.Fatal("the old password still signs in after a change")
	}
	if !verifyPassword(hash, "second-password") {
		t.Fatal("the new password does not sign in")
	}
	if !strings.HasPrefix(hash, "$2") {
		t.Fatal("the new password was stored unhashed")
	}
}

// A site whose administrator was created with the password ChronoFrame used to
// ship can be taken off it from the dashboard.
func TestChangingOffTheFormerDefaultPassword(t *testing.T) {
	app := newTestApp(t)
	id := administrator(t, app, formerDefaultPassword)

	if response := changePassword(t, app, id, formerDefaultPassword, "third-password"); response.Code != http.StatusOK {
		t.Fatalf("leaving the former default password behind = %d, %s", response.Code, response.Body.String())
	}
	if verifyPassword(storedPassword(t, app, id), formerDefaultPassword) {
		t.Fatal("the former default password still signs in")
	}
}

// It is refused just as firmly when chosen as a new password, and a too short
// one is refused before anything is written.
func TestRefusedNewPasswordsLeaveTheOldOneInPlace(t *testing.T) {
	app := newTestApp(t)
	id := administrator(t, app, "first-password")
	before := storedPassword(t, app, id)

	if response := changePassword(t, app, id, "first-password", formerDefaultPassword); response.Code != http.StatusBadRequest {
		t.Fatalf("choosing the former default password as a new one = %d, want it refused", response.Code)
	}
	if response := changePassword(t, app, id, "first-password", "short"); response.Code != http.StatusBadRequest {
		t.Fatalf("choosing a password shorter than %d characters = %d, want it refused", minPasswordLength, response.Code)
	}
	if storedPassword(t, app, id) != before {
		t.Fatal("a refused password was written anyway")
	}
}

// Nobody who is not signed in can change a password, and the route only takes
// the method it is meant to.
func TestChangingAPasswordNeedsASession(t *testing.T) {
	app := newTestApp(t)
	id := administrator(t, app, "first-password")

	body, _ := json.Marshal(map[string]string{"currentPassword": "first-password", "newPassword": "second-password"})
	request := httptest.NewRequest(http.MethodPut, "/api/profile/password", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	app.ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("changing a password without a session = %d, want it refused", response.Code)
	}
	if !verifyPassword(storedPassword(t, app, id), "first-password") {
		t.Fatal("an anonymous request changed the password")
	}
}
