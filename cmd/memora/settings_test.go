package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// A single setting is read by name, which bypasses the public snapshot. It is
// therefore an administrator read, like the namespace read beside it: a secret
// such as the GitHub client secret must not be available by asking for it.
func TestSingleSettingReadRequiresAnAdministrator(t *testing.T) {
	app := newTestApp(t)
	app.setSetting("system", "auth.github.clientSecret", "the-client-secret")
	admin := testUser(t, app, "admin", true)
	visitor := testUser(t, app, "visitor", false)
	path := "/api/system/settings/system/auth.github.clientSecret"

	cases := []struct {
		name    string
		request *http.Request
		status  int
	}{
		{"anonymous", httptest.NewRequest(http.MethodGet, path, nil), http.StatusUnauthorized},
		{"non-administrator", signedRequest(t, app, visitor, http.MethodGet, path, nil), http.StatusForbidden},
		{"administrator", signedRequest(t, app, admin, http.MethodGet, path, nil), http.StatusOK},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			response := httptest.NewRecorder()
			app.ServeHTTP(response, c.request)
			if response.Code != c.status {
				t.Fatalf("reading a single setting = %d, want %d (%s)", response.Code, c.status, response.Body.String())
			}
			if c.status == http.StatusOK {
				if !strings.Contains(response.Body.String(), "the-client-secret") {
					t.Fatalf("an administrator did not get the setting: %s", response.Body.String())
				}
				return
			}
			if strings.Contains(response.Body.String(), "the-client-secret") {
				t.Fatalf("the setting was disclosed to a refused reader: %s", response.Body.String())
			}
		})
	}
}

// The home page opens on the photo wall or on the album list, and the choice
// travels in the public snapshot: a visitor must not be sent to the home page
// the site no longer uses. Only those two views are accepted.
func TestTheHomeViewIsAChoosablePublicSetting(t *testing.T) {
	app := newTestApp(t)
	cases := []struct {
		value  string
		status int
	}{
		{"albums", http.StatusOK},
		{"photos", http.StatusOK},
		{"gallery", http.StatusBadRequest},
	}
	for _, c := range cases {
		t.Run(c.value, func(t *testing.T) {
			request := adminRequest(t, app, http.MethodPut, "/api/system/settings/app/homeView", []byte(`{"value":"`+c.value+`"}`))
			response := httptest.NewRecorder()
			app.ServeHTTP(response, request)
			if response.Code != c.status {
				t.Fatalf("saving homeView = %d, want %d (%s)", response.Code, c.status, response.Body.String())
			}

			// The snapshot is read anonymously, the way a visitor reads it.
			public := httptest.NewRecorder()
			app.ServeHTTP(public, httptest.NewRequest(http.MethodGet, "/api/system/settings/all", nil))
			if public.Code != http.StatusOK {
				t.Fatalf("public settings = %d (%s)", public.Code, public.Body.String())
			}
			var body struct {
				Data struct {
					App map[string]any `json:"app"`
				} `json:"data"`
			}
			if err := json.Unmarshal(public.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			got := body.Data.App["homeView"]
			if c.status == http.StatusBadRequest {
				if got == c.value {
					t.Fatalf("an unsupported home view reached the public snapshot: %v", got)
				}
				return
			}
			if got != c.value {
				t.Fatalf("public homeView = %v, want %q", got, c.value)
			}
		})
	}
}
