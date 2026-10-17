package main

import (
	"context"
	"image"
	"image/jpeg"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func writeTestJPEG(t *testing.T, path string, width, height int) {
	t.Helper()
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if err := jpeg.Encode(file, image.NewRGBA(image.Rect(0, 0, width, height)), nil); err != nil {
		t.Fatal(err)
	}
}

func exifTag(t *testing.T, tool, path, tag string) string {
	t.Helper()
	out, err := exec.Command(tool, "-s3", "-"+tag, path).Output()
	if err != nil {
		t.Fatalf("read %s from %s: %v", tag, filepath.Base(path), err)
	}
	return strings.TrimSpace(string(out))
}

// A converted HEIC is the only copy of the photo, so the metadata transfer has
// to carry the camera tags across.
func TestTransferPhotoMetadataCopiesTags(t *testing.T) {
	exiftool, err := exec.LookPath("exiftool")
	if err != nil {
		t.Skip("exiftool is not available")
	}
	dir := t.TempDir()
	source := filepath.Join(dir, "source.jpg")
	target := filepath.Join(dir, "converted.jpg")
	writeTestJPEG(t, source, 4, 8)
	writeTestJPEG(t, target, 8, 4)

	if out, err := exec.Command(exiftool, "-overwrite_original", "-Make=TestCam", "-Model=TestModel", source).CombinedOutput(); err != nil {
		t.Fatalf("prepare source: %v: %s", err, out)
	}

	app := &App{cfg: Config{ExifTool: exiftool}, logs: NewLogBuffer()}
	if err := app.transferPhotoMetadata(context.Background(), source, target); err != nil {
		t.Fatalf("transferPhotoMetadata: %v", err)
	}

	if got := exifTag(t, exiftool, target, "Make"); got != "TestCam" {
		t.Errorf("Make = %q, want TestCam", got)
	}
	if got := exifTag(t, exiftool, target, "Model"); got != "TestModel" {
		t.Errorf("Model = %q, want TestModel", got)
	}
}

// FFmpeg bakes the EXIF rotation into the pixels before the JPEG is written,
// so copying the source orientation verbatim would rotate the photo twice.
func TestTransferPhotoMetadataResetsOrientation(t *testing.T) {
	exiftool, err := exec.LookPath("exiftool")
	if err != nil {
		t.Skip("exiftool is not available")
	}
	dir := t.TempDir()
	source := filepath.Join(dir, "source.jpg")
	target := filepath.Join(dir, "converted.jpg")
	writeTestJPEG(t, source, 4, 8)
	writeTestJPEG(t, target, 8, 4)

	if out, err := exec.Command(exiftool, "-overwrite_original", "-Orientation#=6", source).CombinedOutput(); err != nil {
		t.Fatalf("prepare source: %v: %s", err, out)
	}

	app := &App{cfg: Config{ExifTool: exiftool}, logs: NewLogBuffer()}
	if err := app.transferPhotoMetadata(context.Background(), source, target); err != nil {
		t.Fatalf("transferPhotoMetadata: %v", err)
	}

	if got := exifTag(t, exiftool, target, "Orientation"); got != "Horizontal (normal)" {
		t.Errorf("Orientation = %q, want Horizontal (normal)", got)
	}
}

// The metadata tool writes dates its own way, and the interface parses them with
// the browser's own date parser, which reads none of those forms. Whatever comes
// out of here has to be a date the browser understands.
func TestTheDateOfAPhotoIsReadable(t *testing.T) {
	readable := func(value string) bool {
		_, err := time.Parse(time.RFC3339, value)
		return err == nil
	}
	cases := []struct{ reported, want string }{
		{"2024:05:06 07:08:09", "2024-05-06T07:08:09Z"},
		{"2024:05:06 07:08:09+02:00", "2024-05-06T05:08:09Z"},
		{"2024:05:06 07:08:09.123", "2024-05-06T07:08:09Z"},
		{"2024:05:06T07:08:09Z", "2024-05-06T07:08:09Z"},
		{"2024:05:06", "2024-05-06T00:00:00Z"},
		{"2024-05-06", "2024-05-06T00:00:00Z"},
		// A date the tool could not read either, and a file name that only looks
		// like one.
		{"0000:00:00 00:00:00", ""},
		{"", ""},
		{"not a date", ""},
	}
	for _, test := range cases {
		got := normalizeDate(test.reported)
		if got != test.want {
			t.Errorf("normalizeDate(%q) = %q, want %q", test.reported, got, test.want)
			continue
		}
		if got != "" && !readable(got) {
			t.Errorf("normalizeDate(%q) = %q, which is not a date the browser reads", test.reported, got)
		}
	}
}
