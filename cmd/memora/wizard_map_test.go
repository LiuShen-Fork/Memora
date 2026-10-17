package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// postMapStep runs the last step of the wizard, the one about the map service.
func postMapStep(t *testing.T, app *App, body map[string]any) *httptest.ResponseRecorder {
	t.Helper()
	data, _ := json.Marshal(body)
	request := httptest.NewRequest(http.MethodPost, "/api/wizard/map", bytes.NewReader(data))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	app.ServeHTTP(response, request)
	return response
}

// mapSetting is what is stored for one setting of the map namespace.
func mapSetting(t *testing.T, app *App, key string) string {
	t.Helper()
	var value any
	if !app.readSetting("map", key, &value) {
		t.Fatalf("no map.%s setting", key)
	}
	text, _ := value.(string)
	return text
}

// A map service is optional: a site can be set up without a token, and the globe
// explains that maps stay blank until an administrator adds one.
func TestASiteCanBeSetUpWithoutAMapToken(t *testing.T) {
	app := newTestApp(t)
	response := postMapStep(t, app, map[string]any{"provider": "maplibre", "token": "", "style": ""})
	if response.Code != http.StatusOK {
		t.Fatalf("saving a map service without a token = %d, %s", response.Code, response.Body.String())
	}
	if provider := mapSetting(t, app, "provider"); provider != "maplibre" {
		t.Fatalf("the provider saved without a token = %q, want maplibre", provider)
	}
}

// The provider is the one thing the step insists on, since the site has to know
// which service to ask for a map.
func TestTheMapStepRefusesAnUnknownProvider(t *testing.T) {
	app := newTestApp(t)
	response := postMapStep(t, app, map[string]any{"provider": "google", "token": "a-token"})
	if response.Code != http.StatusBadRequest {
		t.Fatalf("saving a map service of an unknown provider = %d, want it refused", response.Code)
	}
}

// Setting the map service up again without typing the token keeps the one
// already stored, so running the wizard a second time cannot quietly take a
// working map away.
func TestSavingTheMapServiceAgainKeepsWhatItWasNotGiven(t *testing.T) {
	app := newTestApp(t)
	const storedToken = "the-token-that-was-set"
	const storedStyle = "https://example.com/style.json"
	const storedKey = "the-key-that-was-set"
	app.setSetting("map", "maplibre.token", storedToken)
	app.setSetting("map", "maplibre.style", storedStyle)
	app.setSetting("map", "amap.key", storedKey)

	response := postMapStep(t, app, map[string]any{"provider": "maplibre", "token": "", "style": ""})
	if response.Code != http.StatusOK {
		t.Fatalf("saving the map service again without a token = %d, %s", response.Code, response.Body.String())
	}
	if token := mapSetting(t, app, "maplibre.token"); token != storedToken {
		t.Fatalf("the token after a run that left it out = %q, want the stored one kept", token)
	}
	if style := mapSetting(t, app, "maplibre.style"); style != storedStyle {
		t.Fatalf("the style after a run that left it out = %q, want the stored one kept", style)
	}

	response = postMapStep(t, app, map[string]any{"provider": "amap", "amapKey": "", "amapSecurityCode": ""})
	if response.Code != http.StatusOK {
		t.Fatalf("saving the AMap service without a key = %d, %s", response.Code, response.Body.String())
	}
	if key := mapSetting(t, app, "amap.key"); key != storedKey {
		t.Fatalf("the AMap key after a run that left it out = %q, want the stored one kept", key)
	}

	// A token that is given replaces the one stored, without the space around it.
	response = postMapStep(t, app, map[string]any{"provider": "maplibre", "token": "  a-new-token  "})
	if response.Code != http.StatusOK {
		t.Fatalf("saving the map service with a new token = %d, %s", response.Code, response.Body.String())
	}
	if token := mapSetting(t, app, "maplibre.token"); token != "a-new-token" {
		t.Fatalf("the token after a new one was given = %q, want a-new-token", token)
	}
}
