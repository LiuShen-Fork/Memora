package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMigratedWebDirPrefersWorkspaceBundle(t *testing.T) {
	root := t.TempDir()
	legacy := filepath.Join(root, ".output", "public")
	current := filepath.Join(root, "web", ".output", "public")
	if err := os.MkdirAll(legacy, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(current, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{legacy, current} {
		if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("ok"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if got := migratedWebDir(legacy); got != current {
		t.Fatalf("migratedWebDir(%q) = %q, want %q", legacy, got, current)
	}
}

func TestMigratedWebDirLeavesCanonicalWorkspaceAlone(t *testing.T) {
	root := t.TempDir()
	current := filepath.Join(root, "web", ".output", "public")
	if err := os.MkdirAll(filepath.Join(current), 0o755); err != nil {
		t.Fatal(err)
	}
	if got := migratedWebDir(current); got != "" {
		t.Fatalf("migratedWebDir(%q) = %q, want empty", current, got)
	}
}
