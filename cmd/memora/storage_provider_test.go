package main

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
)

// configure turns a fresh configuration into the active one the way the
// settings page does: it saves the id as a JSON number.
func configure(t *testing.T, app *App, name string) int64 {
	t.Helper()
	id, err := app.insertStorageConfig(name, "s3", map[string]any{
		"provider": "s3", "endpoint": "https://s3.example.com", "bucket": "photos", "region": "us-east-1",
		"accessKeyId": "key", "secretAccessKey": "secret", "prefix": "photos/",
	})
	if err != nil {
		t.Fatal(err)
	}
	body := []byte(`{"value":` + strconv.FormatInt(id, 10) + `}`)
	response := httptest.NewRecorder()
	app.ServeHTTP(response, adminRequest(t, app, http.MethodPut, "/api/system/settings/storage/provider", body))
	if response.Code != http.StatusOK {
		t.Fatalf("switching the default storage = %d, %s", response.Code, response.Body.String())
	}
	return id
}

// Switching the default storage happens from the settings page, and the server
// has to read from the new place without a restart.
func TestSwitchingTheDefaultStorageTakesEffectAtOnce(t *testing.T) {
	app := newTestApp(t)
	configure(t, app, "Archive bucket")
	if active := dashboardStats(t, app).Storage.ActiveProvider; active == nil || active.Provider != "s3" || active.Name != "Archive bucket" {
		t.Fatalf("activeProvider = %+v, want the configured bucket", active)
	}
	if _, ok := app.storage.(*S3Storage); !ok {
		t.Fatalf("storage = %T, want the server to be reading the bucket", app.storage)
	}
}

// The configuration in use is the one the site writes to, so it is the one a
// delete request may not take away.
func TestTheStorageInUseCannotBeDeleted(t *testing.T) {
	app := newTestApp(t)
	id := configure(t, app, "Archive bucket")
	response := httptest.NewRecorder()
	target := "/api/system/settings/storage-config/" + strconv.FormatInt(id, 10)
	app.ServeHTTP(response, adminRequest(t, app, http.MethodDelete, target, nil))
	if response.Code != http.StatusConflict {
		t.Fatalf("deleting the storage in use = %d, want it refused", response.Code)
	}
	var rows int
	if err := app.db.QueryRow(`SELECT count(*) FROM settings_storage_providers WHERE id=?`, id).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if rows != 1 {
		t.Fatalf("the storage in use is gone: %d rows left", rows)
	}
}

// A site that chose its storage before this build has the id stored as a real
// number, and the choice has to keep working after an upgrade.
func TestTheStorageChosenEarlierIsStillRead(t *testing.T) {
	app := newTestApp(t)
	id, err := app.insertStorageConfig("Archive bucket", "s3", map[string]any{
		"provider": "s3", "endpoint": "https://s3.example.com", "bucket": "photos", "region": "us-east-1",
		"accessKeyId": "key", "secretAccessKey": "secret", "prefix": "photos/",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := app.db.Exec(`UPDATE settings SET value=? WHERE namespace='storage' AND key='provider'`, float64(id)); err != nil {
		t.Fatal(err)
	}
	if active := dashboardStats(t, app).Storage.ActiveProvider; active == nil || active.Name != "Archive bucket" {
		t.Fatalf("activeProvider = %+v, want the bucket the site had chosen", active)
	}
	app.storage = app.loadStorage()
	if _, ok := app.storage.(*S3Storage); !ok {
		t.Fatalf("storage = %T, want the server to read the bucket it had chosen", app.storage)
	}
}
