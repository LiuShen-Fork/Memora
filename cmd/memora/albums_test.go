package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// The album list used to be ordered by created_at DESC, id DESC. The position
// column replaces that with a value the user controls, so the backfill has to
// reproduce the old order exactly, ties included, or every existing install
// would reshuffle on the first start after the upgrade.
func TestAlbumPositionBackfillMatchesPreviousOrder(t *testing.T) {
	app := newSchemaTestApp(t)
	if _, err := app.db.Exec(`CREATE TABLE albums (id INTEGER PRIMARY KEY AUTOINCREMENT, title TEXT NOT NULL, created_at INTEGER NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	// Two albums share a creation second, which is the common case because
	// created_at has second resolution; the id breaks that tie.
	if _, err := app.db.Exec(`INSERT INTO albums(id,title,created_at) VALUES(1,'first',100),(2,'second',100),(3,'newest',200)`); err != nil {
		t.Fatal(err)
	}
	if err := app.applySchemaStep(albumPositionStep); err != nil {
		t.Fatal(err)
	}
	rows, err := app.db.Query(`SELECT id FROM albums ORDER BY position ASC,created_at DESC,id DESC`)
	if err != nil {
		t.Fatal(err)
	}
	order := []int{}
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		order = append(order, id)
	}
	_ = rows.Close()
	if len(order) != 3 || order[0] != 3 || order[1] != 2 || order[2] != 1 {
		t.Fatalf("backfilled order = %v, want [3 2 1]", order)
	}
}

func albumIDs(t *testing.T, app *App) []int {
	t.Helper()
	response := httptest.NewRecorder()
	app.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/albums", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("album list status = %d", response.Code)
	}
	var albums []struct {
		ID int `json:"id"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &albums); err != nil {
		t.Fatal(err)
	}
	ids := []int{}
	for _, album := range albums {
		ids = append(ids, album.ID)
	}
	return ids
}

func TestReorderAlbumsStoresTheGivenOrder(t *testing.T) {
	app := newTestApp(t)
	for _, title := range []string{"a", "b", "c"} {
		response := httptest.NewRecorder()
		app.ServeHTTP(response, adminRequest(t, app, http.MethodPost, "/api/albums", []byte(`{"title":"`+title+`"}`)))
		if response.Code != http.StatusOK {
			t.Fatalf("create %s status = %d body = %s", title, response.Code, response.Body.String())
		}
	}
	// Newest first, which is where createAlbum puts each new album.
	if ids := albumIDs(t, app); len(ids) != 3 || ids[0] != 3 || ids[1] != 2 || ids[2] != 1 {
		t.Fatalf("initial order = %v, want [3 2 1]", ids)
	}

	response := httptest.NewRecorder()
	app.ServeHTTP(response, adminRequest(t, app, http.MethodPut, "/api/albums/reorder", []byte(`{"albumIds":[2,3,1]}`)))
	if response.Code != http.StatusOK {
		t.Fatalf("reorder status = %d body = %s", response.Code, response.Body.String())
	}
	if ids := albumIDs(t, app); len(ids) != 3 || ids[0] != 2 || ids[1] != 3 || ids[2] != 1 {
		t.Fatalf("order after reorder = %v, want [2 3 1]", ids)
	}

	// A new album still lands on top of whatever order the user chose.
	response = httptest.NewRecorder()
	app.ServeHTTP(response, adminRequest(t, app, http.MethodPost, "/api/albums", []byte(`{"title":"d"}`)))
	if response.Code != http.StatusOK {
		t.Fatalf("create d status = %d", response.Code)
	}
	if ids := albumIDs(t, app); len(ids) != 4 || ids[0] != 4 {
		t.Fatalf("order after creating a new album = %v, want 4 first", ids)
	}
}

func TestReorderAlbumsRejectsAnonymousAndDuplicateIDs(t *testing.T) {
	app := newTestApp(t)
	anonymous := httptest.NewRecorder()
	anonymousRequest := httptest.NewRequest(http.MethodPut, "/api/albums/reorder", nil)
	app.ServeHTTP(anonymous, anonymousRequest)
	if anonymous.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous reorder status = %d, want 401", anonymous.Code)
	}
	for _, body := range []string{`{}`, `{"albumIds":[]}`, `{"albumIds":[1,1]}`, `{"albumIds":[0]}`} {
		response := httptest.NewRecorder()
		app.ServeHTTP(response, adminRequest(t, app, http.MethodPut, "/api/albums/reorder", []byte(body)))
		if response.Code != http.StatusBadRequest {
			t.Fatalf("reorder %s status = %d, want 400", body, response.Code)
		}
	}
}
