package main

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// shellApp answers with a generated frontend whose only file is the shell.
// The page is annotated with a link preview before it goes out, so this needs
// the settings a real instance has.
func shellApp(t *testing.T, shell string) *App {
	t.Helper()
	app := newTestApp(t)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte(shell), 0o644); err != nil {
		t.Fatal(err)
	}
	app.cfg.WebDir = dir
	return app
}

func TestTheShellShowsASkeletonUntilTheAppMounts(t *testing.T) {
	shell := `<!DOCTYPE html><html><head></head><body><div id="__nuxt" class="isolate"></div><div id="teleports"></div></body></html>`
	app := shellApp(t, shell)

	response := assetRequest(t, app, "/some/deep/link", nil)
	if response.Code != http.StatusOK {
		t.Fatalf("shell is %d", response.Code)
	}
	body := response.Body.String()
	if !strings.Contains(body, `id="__nuxt" class="isolate"><div class="cf-splash"`) {
		t.Fatalf("the skeleton is not inside the application root: %s", body)
	}
	// The skeleton has to leave on its own: it lives inside the element Vue
	// empties when it mounts, not beside it.
	if strings.Index(body, "cf-splash") < strings.Index(body, `id="__nuxt"`) {
		t.Fatalf("the skeleton is outside the application root: %s", body)
	}
	if !strings.Contains(body, `/web-app-manifest-192x192.png`) {
		t.Fatalf("the skeleton has no avatar: %s", body)
	}
	// Dark mode has to follow the class the color-mode script puts on <html>,
	// not the operating system: someone who picked dark in the app on a light
	// machine gets a white screen for as long as the bundle takes otherwise.
	if !strings.Contains(body, `html.dark .cf-splash{background:#171717}`) {
		t.Fatalf("the skeleton ignores the theme the app chose: %s", body)
	}
	// The media query is the fallback for a browser that never ran the
	// color-mode script, so it must stay behind the chosen class.
	if strings.Index(body, "prefers-color-scheme:dark") > strings.Index(body, "html.dark .cf-splash") {
		t.Fatalf("the operating system wins over the chosen theme: %s", body)
	}
	if strings.Contains(body, "cf-splash-ring") || strings.Contains(body, "cf-splash-avatar") {
		t.Fatalf("the skeleton still draws the old ring around the icon: %s", body)
	}
	if response.Header().Get("Content-Length") != itoa(int64(len(body))) {
		t.Fatalf("Content-Length does not match the body it describes")
	}
}

func TestAShellWithoutAnApplicationRootIsServedAsItIs(t *testing.T) {
	shell := `<html><body><p>hand-made page</p></body></html>`
	app := shellApp(t, shell)

	response := assetRequest(t, app, "/", nil)
	if response.Code != http.StatusOK {
		t.Fatalf("shell is %d", response.Code)
	}
	if body := response.Body.String(); strings.Contains(body, "cf-splash") {
		t.Fatalf("a shell with nowhere to put the skeleton got one anyway: %s", body)
	}
}
