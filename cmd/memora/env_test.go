package main

import (
	"bytes"
	"log"
	"strings"
	"testing"
	"time"
)

// captureLog returns everything the standard logger writes while the test runs.
func captureLog(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buffer bytes.Buffer
	previous := log.Writer()
	log.SetOutput(&buffer)
	t.Cleanup(func() { log.SetOutput(previous) })
	return &buffer
}

func TestTheRenamedVariableWinsOverTheOldSpelling(t *testing.T) {
	t.Setenv("MEMORA_TEST_VALUE", "new")
	t.Setenv("CFRAME_TEST_VALUE", "old")

	if value := env("MEMORA_TEST_VALUE", "fallback"); value != "new" {
		t.Fatalf("value is %q, want the renamed variable", value)
	}
}

func TestTheOldSpellingStillAnswers(t *testing.T) {
	t.Setenv("MEMORA_TEST_LEGACY", "")
	t.Setenv("CFRAME_TEST_LEGACY", "old")

	if value := env("MEMORA_TEST_LEGACY", "fallback"); value != "old" {
		t.Fatalf("value is %q, want the old variable", value)
	}
}

func TestTheOldSpellingIsReportedOncePerRun(t *testing.T) {
	t.Setenv("MEMORA_TEST_WARNED", "")
	t.Setenv("CFRAME_TEST_WARNED", "old")
	logs := captureLog(t)

	for range 3 {
		env("MEMORA_TEST_WARNED", "")
	}

	if count := strings.Count(logs.String(), "CFRAME_TEST_WARNED is deprecated"); count != 1 {
		t.Fatalf("the deprecation is reported %d times, want 1 (%q)", count, logs.String())
	}
	if !strings.Contains(logs.String(), "MEMORA_TEST_WARNED") {
		t.Fatalf("the warning does not name the replacement: %q", logs.String())
	}
}

func TestTheRenamedVariableIsNotReported(t *testing.T) {
	t.Setenv("MEMORA_TEST_QUIET", "new")
	logs := captureLog(t)

	if value := env("MEMORA_TEST_QUIET", "fallback"); value != "new" {
		t.Fatalf("value is %q", value)
	}
	if logs.Len() != 0 {
		t.Fatalf("a renamed variable was reported: %q", logs.String())
	}
}

func TestAnUnsetVariableFallsBackToTheDefault(t *testing.T) {
	t.Setenv("MEMORA_TEST_MISSING", "")
	t.Setenv("CFRAME_TEST_MISSING", "   ")

	if value := env("MEMORA_TEST_MISSING", "fallback"); value != "fallback" {
		t.Fatalf("a blank variable answered %q", value)
	}
}

func TestEverySettingStyleReadsTheRenamedVariable(t *testing.T) {
	t.Setenv("MEMORA_TEST_BOOL", "yes")
	t.Setenv("MEMORA_TEST_INT", "7")
	t.Setenv("MEMORA_TEST_MB", "3")
	t.Setenv("MEMORA_TEST_DURATION", "90s")

	if !envBool("MEMORA_TEST_BOOL", false) {
		t.Fatal("envBool did not read the renamed variable")
	}
	if value := envInt("MEMORA_TEST_INT", 1); value != 7 {
		t.Fatalf("envInt read %d", value)
	}
	if value := envBytesMB("MEMORA_TEST_MB", 1); value != 3<<20 {
		t.Fatalf("envBytesMB read %d", value)
	}
	if value := envDuration("MEMORA_TEST_DURATION", time.Second); value != 90*time.Second {
		t.Fatalf("envDuration read %s", value)
	}
}

func TestEverySettingStyleFallsBackToTheOldSpelling(t *testing.T) {
	t.Setenv("CFRAME_TEST_OLD_BOOL", "true")
	t.Setenv("CFRAME_TEST_OLD_INT", "4")
	t.Setenv("CFRAME_TEST_OLD_DURATION", "30s")

	if !envBool("MEMORA_TEST_OLD_BOOL", false) {
		t.Fatal("envBool ignored the old spelling")
	}
	if value := envInt("MEMORA_TEST_OLD_INT", 1); value != 4 {
		t.Fatalf("envInt read %d from the old spelling", value)
	}
	if value := envDuration("MEMORA_TEST_OLD_DURATION", time.Second); value != 30*time.Second {
		t.Fatalf("envDuration read %s from the old spelling", value)
	}
}

func TestTheWizardPrefillReadsTheRenamedAdminVariables(t *testing.T) {
	t.Setenv("MEMORA_ADMIN_EMAIL", "renamed@example.com")
	t.Setenv("CFRAME_ADMIN_EMAIL", "old@example.com")

	if value := env("MEMORA_ADMIN_EMAIL", ""); value != "renamed@example.com" {
		t.Fatalf("the wizard would prefill %q", value)
	}
}
