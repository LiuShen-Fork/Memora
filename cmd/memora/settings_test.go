package main

import (
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
