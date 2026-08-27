package main

import "testing"

func TestThumbnailQualityMatchesLegacySharpPipeline(t *testing.T) {
	if got := thumbnailQuality(5 * 1024 * 1024); got != 100 {
		t.Fatalf("quality at 5 MiB = %d, want 100", got)
	}
	if got := thumbnailQuality(5*1024*1024 + 1); got != 85 {
		t.Fatalf("quality above 5 MiB = %d, want 85", got)
	}
}
