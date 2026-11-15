package main

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func envFrom(values map[string]string) func(string) string {
	return func(key string) string { return values[key] }
}

func TestTheSessionSecretComesFromTheEnvironmentFirst(t *testing.T) {
	const (
		memoraKey   = "memora-key-000000000000000000000"
		upstreamKey = "upstream-key-111111111111111111"
		legacyKey   = "legacy-key-22222222222222222222"
	)
	cases := []struct {
		name string
		env  map[string]string
		want string
	}{
		{name: "memora name", env: map[string]string{"MEMORA_SESSION_SECRET": memoraKey}, want: memoraKey},
		{name: "upstream name", env: map[string]string{"NUXT_SESSION_PASSWORD": upstreamKey}, want: upstreamKey},
		{name: "old fork name", env: map[string]string{"CFRAME_SESSION_SECRET": legacyKey}, want: legacyKey},
		{
			name: "memora wins over both",
			env: map[string]string{
				"MEMORA_SESSION_SECRET": memoraKey,
				"NUXT_SESSION_PASSWORD": upstreamKey,
				"CFRAME_SESSION_SECRET": legacyKey,
			},
			want: memoraKey,
		},
		{
			name: "upstream wins over the old fork name",
			env: map[string]string{
				"NUXT_SESSION_PASSWORD": upstreamKey,
				"CFRAME_SESSION_SECRET": legacyKey,
			},
			want: upstreamKey,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			got, err := resolveSessionSecret(dir, envFrom(tc.env))
			if err != nil {
				t.Fatalf("resolveSessionSecret: %v", err)
			}
			if string(got) != tc.want {
				t.Fatalf("secret = %q, want %q", got, tc.want)
			}
			if _, err := os.Stat(filepath.Join(dir, sessionSecretFileName)); !os.IsNotExist(err) {
				t.Fatalf("a variable was set but a secret file was written")
			}
		})
	}
}

func TestTheSessionSecretIsGeneratedOnceAndReused(t *testing.T) {
	dir := t.TempDir()
	first, err := resolveSessionSecret(dir, envFrom(nil))
	if err != nil {
		t.Fatalf("first resolveSessionSecret: %v", err)
	}
	if len(first) < minSessionSecretLength {
		t.Fatalf("generated secret is %d characters, want at least %d", len(first), minSessionSecretLength)
	}
	decoded, err := base64.RawURLEncoding.DecodeString(string(first))
	if err != nil {
		t.Fatalf("generated secret is not base64url: %v", err)
	}
	if len(decoded) != sessionSecretSize {
		t.Fatalf("generated secret decodes to %d bytes, want %d", len(decoded), sessionSecretSize)
	}

	stored, err := os.ReadFile(filepath.Join(dir, sessionSecretFileName))
	if err != nil {
		t.Fatalf("read stored secret: %v", err)
	}
	if strings.TrimSpace(string(stored)) != string(first) {
		t.Fatalf("stored secret %q differs from the one in use %q", stored, first)
	}

	second, err := resolveSessionSecret(dir, envFrom(nil))
	if err != nil {
		t.Fatalf("second resolveSessionSecret: %v", err)
	}
	if string(second) != string(first) {
		t.Fatalf("a restart generated a new secret: %q then %q", first, second)
	}
}

func TestTheSessionSecretReplacesAStoredFileThatCannotSign(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, sessionSecretFileName)
	if err := os.WriteFile(path, []byte("too-short\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := resolveSessionSecret(dir, envFrom(nil))
	if err != nil {
		t.Fatalf("resolveSessionSecret: %v", err)
	}
	if string(got) == "too-short" || len(got) < minSessionSecretLength {
		t.Fatalf("a short stored file was kept as %q", got)
	}
	stored, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(stored)) != string(got) {
		t.Fatalf("the replacement was not written back: file %q, key %q", stored, got)
	}
}

func TestTheSessionSecretKeepsAConfiguredValueEvenWhenItIsShort(t *testing.T) {
	dir := t.TempDir()
	got, err := resolveSessionSecret(dir, envFrom(map[string]string{"MEMORA_SESSION_SECRET": "short"}))
	if err != nil {
		t.Fatalf("resolveSessionSecret: %v", err)
	}
	if string(got) != "short" {
		t.Fatalf("secret = %q, want the configured value", got)
	}
}

func TestTheSessionSecretIgnoresBlankVariables(t *testing.T) {
	dir := t.TempDir()
	got, err := resolveSessionSecret(dir, envFrom(map[string]string{
		"MEMORA_SESSION_SECRET": "   ",
		"NUXT_SESSION_PASSWORD": "",
	}))
	if err != nil {
		t.Fatalf("resolveSessionSecret: %v", err)
	}
	if len(got) < minSessionSecretLength {
		t.Fatalf("blank variables should fall through to a generated secret, got %q", got)
	}
}
