package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// aPhotoToReactTo adds one photo anyone may see, which is what lets the public
// reaction routes answer for it at all.
func aPhotoToReactTo(t *testing.T, app *App) string {
	t.Helper()
	if _, err := app.db.Exec(`INSERT INTO photos(id,title,storage_key) VALUES('photo','A photo','photos/photo.jpg')`); err != nil {
		t.Fatal(err)
	}
	return "photo"
}

func reactionRowCount(t *testing.T, app *App) int {
	t.Helper()
	var count int
	if err := app.db.QueryRow(`SELECT count(*) FROM photo_reactions`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

// A database that predates the setting, or one where it was never touched,
// keeps reactions on: the switch exists to turn them off, not to require being
// turned on first.
func TestReactionsStayOnUntilTheyAreTurnedOff(t *testing.T) {
	app := newTestApp(t)
	id := aPhotoToReactTo(t, app)

	if _, err := app.db.Exec(`DELETE FROM settings WHERE namespace='app' AND key='reactions.enabled'`); err != nil {
		t.Fatal(err)
	}

	react := httptest.NewRecorder()
	app.ServeHTTP(react, anonymous(http.MethodPost, "/api/photos/"+id+"/reactions", []byte(`{"reactionType":"like"}`)))
	if react.Code != http.StatusOK {
		t.Fatalf("reacting without the setting = %d, %s", react.Code, react.Body.String())
	}

	read := httptest.NewRecorder()
	app.ServeHTTP(read, anonymous(http.MethodGet, "/api/photos/"+id+"/reactions", nil))
	if read.Code != http.StatusOK || !strings.Contains(read.Body.String(), `"like":1`) {
		t.Fatalf("reading reactions without the setting = %d, %s", read.Code, read.Body.String())
	}
}

// Turning reactions off closes every public reaction route and takes the counts
// out of the gallery, but the rows stay where they are: the switch hides what
// was said without forgetting it.
func TestTurningReactionsOffClosesTheRoutesAndKeepsTheRows(t *testing.T) {
	app := newTestApp(t)
	id := aPhotoToReactTo(t, app)

	react := httptest.NewRecorder()
	app.ServeHTTP(react, anonymous(http.MethodPost, "/api/photos/"+id+"/reactions", []byte(`{"reactionType":"love"}`)))
	if react.Code != http.StatusOK {
		t.Fatalf("reacting before the switch = %d, %s", react.Code, react.Body.String())
	}
	before := reactionRowCount(t, app)

	app.setSetting("app", "reactions.enabled", false)

	for _, method := range []string{http.MethodGet, http.MethodPost, http.MethodDelete} {
		var body []byte
		if method == http.MethodPost {
			body = []byte(`{"reactionType":"fire"}`)
		}
		response := httptest.NewRecorder()
		app.ServeHTTP(response, anonymous(method, "/api/photos/"+id+"/reactions", body))
		if response.Code != http.StatusForbidden {
			t.Fatalf("%s with reactions off = %d, want 403", method, response.Code)
		}
	}

	// The gallery asks for counts in one request; a visitor gets none to draw.
	list := httptest.NewRecorder()
	app.ServeHTTP(list, anonymous(http.MethodGet, "/api/photos/reactions?ids="+id, nil))
	if body := strings.TrimSpace(list.Body.String()); body != "{}" {
		t.Fatalf("counts with reactions off came back as %s, want {}", body)
	}

	// Whoever looks after the site still sees what was said, because that is
	// what they are deciding on when they look at the switch.
	managed := httptest.NewRecorder()
	app.ServeHTTP(managed, adminRequest(t, app, http.MethodGet, "/api/photos/reactions?ids="+id, nil))
	if body := managed.Body.String(); !strings.Contains(body, `"love":1`) {
		t.Fatalf("counts an administrator asked for with reactions off = %s, want them kept", body)
	}

	if after := reactionRowCount(t, app); after != before {
		t.Fatalf("reaction rows with reactions off = %d, want the %d left in place", after, before)
	}

	// Turning them back on returns the reaction that was there before.
	app.setSetting("app", "reactions.enabled", true)

	read := httptest.NewRecorder()
	app.ServeHTTP(read, anonymous(http.MethodGet, "/api/photos/"+id+"/reactions", nil))
	if read.Code != http.StatusOK {
		t.Fatalf("reading after turning reactions back on = %d, %s", read.Code, read.Body.String())
	}
	if body := read.Body.String(); !strings.Contains(body, `"love":1`) || !strings.Contains(body, `"userReaction":"love"`) {
		t.Fatalf("reactions after turning them back on = %s, want the earlier reaction", body)
	}
}

// A photo out of sight is still answered as absent when reactions are off, so
// the refusal never names a photo the visitor may not see.
func TestReactionsOffStillHidesWhatAnAlbumHides(t *testing.T) {
	resetUnlockLimits()
	app := newTestApp(t)
	aLockedAlbum(t, app, "hunter2")
	app.setSetting("app", "reactions.enabled", false)

	response := httptest.NewRecorder()
	app.ServeHTTP(response, anonymous(http.MethodGet, "/api/photos/secret/reactions", nil))
	if response.Code != http.StatusNotFound {
		t.Fatalf("reactions of a locked photo with reactions off = %d, want 404", response.Code)
	}
}
