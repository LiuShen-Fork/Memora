package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// The footer icons link out from every visitor's screen, so only addresses
// that are plainly https to a public hostname are kept.
func TestSocialLinksAcceptOnlyHTTPSProfileURLs(t *testing.T) {
	cases := []struct {
		name  string
		value any
		ok    bool
		want  string
	}{
		{"one link", `["https://github.com/LiuShen"]`, true, `["https://github.com/LiuShen"]`},
		{"four links", `["https://github.com/a","https://x.com/b","https://space.bilibili.com/1","https://weibo.com/u/2"]`, true, `["https://github.com/a","https://x.com/b","https://space.bilibili.com/1","https://weibo.com/u/2"]`},
		{"none", `[]`, true, ""},
		{"empty text", "", true, ""},
		{"missing value", nil, true, ""},
		{"five links", `["https://a.com","https://b.com","https://c.com","https://d.com","https://e.com"]`, false, ""},
		{"http", `["http://github.com/a"]`, false, ""},
		{"no scheme", `["github.com/a"]`, false, ""},
		{"credentials", `["https://user:pass@github.com/a"]`, false, ""},
		{"bare hostname", `["https://localhost/a"]`, false, ""},
		{"ip address", `["https://127.0.0.1/a"]`, false, ""},
		{"padded", `[" https://github.com/a"]`, false, ""},
		{"empty entry", `[""]`, false, ""},
		{"duplicate", `["https://github.com/a","https://github.com/a/"]`, false, ""},
		{"not json", `https://github.com/a`, false, ""},
		{"not strings", `[1,2]`, false, ""},
		{"object", `{"links":["https://github.com/a"]}`, false, ""},
		{"control character", "[\"https://github.com/a\tb\"]", false, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := normalizeSocialLinks(c.value)
			if c.ok && err != nil {
				t.Fatalf("normalizeSocialLinks(%v) = %v, want no error", c.value, err)
			}
			if !c.ok {
				if err == nil {
					t.Fatalf("normalizeSocialLinks(%v) accepted an unusable value: %v", c.value, got)
				}
				return
			}
			if got != c.want {
				t.Fatalf("normalizeSocialLinks(%v) = %q, want %q", c.value, got, c.want)
			}
		})
	}
}

// The links travel in the public snapshot: the wall draws them without a
// signed-in reader. Saving goes through the same validation, and a rejected
// value leaves the previous one in place.
func TestSocialLinksAreSavedPubliclyAndOnlyWhenTheyAreUsable(t *testing.T) {
	app := newTestApp(t)

	readPublic := func() string {
		t.Helper()
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
		value, _ := body.Data.App["socialLinks"].(string)
		return value
	}

	if got := readPublic(); got != "" {
		t.Fatalf("social links start at %q, want empty", got)
	}

	path := "/api/system/settings/app/socialLinks"
	save := func(value string) (int, string) {
		t.Helper()
		encoded, err := json.Marshal(map[string]string{"value": value})
		if err != nil {
			t.Fatal(err)
		}
		response := httptest.NewRecorder()
		app.ServeHTTP(response, adminRequest(t, app, http.MethodPut, path, encoded))
		return response.Code, response.Body.String()
	}

	if status, body := save(`["https://github.com/LiuShen"]`); status != http.StatusOK {
		t.Fatalf("saving social links = %d (%s)", status, body)
	}
	if got := readPublic(); got != `["https://github.com/LiuShen"]` {
		t.Fatalf("public social links = %q", got)
	}

	rejected := []string{
		`["http://github.com/LiuShen"]`,
		`["https://github.com/a","https://github.com/b","https://github.com/c","https://github.com/d","https://github.com/e"]`,
		`["https://github.com/LiuShen","https://github.com/LiuShen"]`,
		`not a list`,
	}
	for _, value := range rejected {
		if status, body := save(value); status != http.StatusBadRequest {
			t.Fatalf("saving %s = %d, want 400 (%s)", value, status, body)
		}
		if got := readPublic(); got != `["https://github.com/LiuShen"]` {
			t.Fatalf("a rejected value replaced the saved links: %q", got)
		}
	}

	if status, body := save(`[]`); status != http.StatusOK {
		t.Fatalf("clearing social links = %d (%s)", status, body)
	}
	if got := readPublic(); got != "" {
		t.Fatalf("cleared social links = %q, want empty", got)
	}
}

// The settings form lists every app setting; the social links are edited by a
// card of their own, and this keeps the shape the card relies on.
func TestSocialLinksAppearInTheSettingsSchema(t *testing.T) {
	app := newTestApp(t)
	response := httptest.NewRecorder()
	app.ServeHTTP(response, adminRequest(t, app, http.MethodGet, "/api/system/settings/fields?namespace=app", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("setting fields = %d (%s)", response.Code, response.Body.String())
	}
	var body struct {
		Fields []struct {
			Key   string `json:"key"`
			Type  string `json:"type"`
			Label string `json:"label"`
		} `json:"fields"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	for _, field := range body.Fields {
		if field.Key != "socialLinks" {
			continue
		}
		if field.Type != "string" || field.Label != "settings.app.socialLinks.label" {
			t.Fatalf("socialLinks field = %+v", field)
		}
		return
	}
	t.Fatal("the settings schema has no socialLinks field")
}
