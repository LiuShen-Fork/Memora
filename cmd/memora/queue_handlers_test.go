package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// The queue is a dashboard surface: listing it, retrying it, clearing it and
// enqueueing work are all administrator actions. A signed-in visitor must not
// be able to do any of them just because the session is valid.
func TestQueueManagementRequiresAnAdministrator(t *testing.T) {
	app := newTestApp(t)
	admin := testUser(t, app, "admin", true)
	visitor := testUser(t, app, "visitor", false)

	cases := []struct {
		name        string
		method      string
		path        string
		body        string
		adminStatus int
	}{
		{"enqueue", http.MethodPost, "/api/queue/add-task", `{}`, http.StatusBadRequest},
		{"list", http.MethodGet, "/api/queue/task/list", "", http.StatusOK},
		{"recent", http.MethodGet, "/api/queue/recent", "", http.StatusOK},
		{"stats", http.MethodGet, "/api/queue/stats", "", http.StatusOK},
		{"clear", http.MethodDelete, "/api/queue/task/clear", "", http.StatusOK},
		{"retry", http.MethodPost, "/api/queue/task/retry", `{"taskId":999}`, http.StatusNotFound},
		{"retry batch", http.MethodPost, "/api/queue/task/retry-batch", `{"retryAll":true}`, http.StatusOK},
		{"delete failed", http.MethodDelete, "/api/queue/failed/999", "", http.StatusNotFound},
		{"concurrency", http.MethodGet, "/api/queue/concurrency", "", http.StatusOK},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			body := func() io.Reader {
				if c.body == "" {
					return nil
				}
				return strings.NewReader(c.body)
			}
			anonymous := httptest.NewRecorder()
			app.ServeHTTP(anonymous, httptest.NewRequest(c.method, c.path, body()))
			if anonymous.Code != http.StatusUnauthorized {
				t.Fatalf("anonymous %s %s = %d, want 401", c.method, c.path, anonymous.Code)
			}

			refused := httptest.NewRecorder()
			app.ServeHTTP(refused, signedRequest(t, app, visitor, c.method, c.path, body()))
			if refused.Code != http.StatusForbidden {
				t.Fatalf("visitor %s %s = %d, want 403", c.method, c.path, refused.Code)
			}

			allowed := httptest.NewRecorder()
			app.ServeHTTP(allowed, signedRequest(t, app, admin, c.method, c.path, body()))
			if allowed.Code != c.adminStatus {
				t.Fatalf("administrator %s %s = %d, want %d (%s)", c.method, c.path, allowed.Code, c.adminStatus, allowed.Body.String())
			}
		})
	}
}
