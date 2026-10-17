package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// TestDumpDashboardFixtures is a one-off capture, not a test of behaviour: it
// writes the settings descriptors the dashboard renders from, so the dev-only
// mock API in web/mock can serve the real shapes without the Go server running.
// It is skipped unless MEMORA_FIXTURE_DIR names where to put them.
func TestDumpDashboardFixtures(t *testing.T) {
	dir := os.Getenv("MEMORA_FIXTURE_DIR")
	if dir == "" {
		t.Skip("MEMORA_FIXTURE_DIR is not set")
	}

	app := newTestApp(t)

	namespaces := []string{
		"app", "map", "location", "privacy", "system", "analytics", "storage",
	}
	fields := map[string]any{}
	for _, namespace := range namespaces {
		fields[namespace] = map[string]any{
			"namespace": namespace,
			"fields":    app.settingFields(namespace),
		}
	}

	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	write := func(name string, value any) {
		t.Helper()
		encoded, err := json.MarshalIndent(value, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, name), append(encoded, '\n'), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	write("settings-fields.json", fields)
	write("settings-schema.json", app.settingSchema())
}
