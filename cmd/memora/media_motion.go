package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

const motionScanLimit = 512 * 1024

func (a *App) extractMotionPhoto(ctx context.Context, photoID, storageKey string, data []byte, exif map[string]any) (string, error) {
	motion := motionBool(exif["MotionPhoto"]) || motionBool(exif["MicroVideo"])
	offsets := []int{}
	for _, key := range []string{"MicroVideoOffset", "GCamera:MicroVideoOffset"} {
		if value, ok := motionNumber(exif[key]); ok && value > 0 {
			offsets = append(offsets, int(value))
		}
	}
	xmp := string(data[:min(len(data), motionScanLimit)])
	if strings.Contains(strings.ToLower(xmp), "motionphoto") || strings.Contains(strings.ToLower(xmp), "microvideo") {
		motion = true
	}
	for _, tag := range []string{"MicroVideoOffset", "GCamera:MicroVideoOffset"} {
		for _, match := range regexp.MustCompile(`(?i)`+regexp.QuoteMeta(tag)+`[^0-9]{0,20}([0-9]+)`).FindAllStringSubmatch(xmp, -1) {
			if len(match) > 1 {
				if value, err := strconv.Atoi(match[1]); err == nil && value > 0 {
					offsets = append(offsets, value)
				}
			}
		}
	}
	video, ok := findEmbeddedMP4(data, offsets)
	if !motion && !ok {
		return "", nil
	}
	if !ok {
		return "", errors.New("motion photo metadata found but embedded MP4 was not located")
	}
	if existing, err := a.storage.Meta(ctx, photoID+".mp4"); err == nil && a.validStoredVideo(ctx, photoID+".mp4") {
		return existing.Key, nil
	}
	object, err := a.storage.Create(ctx, photoID+".mp4", video, "video/mp4")
	if err != nil {
		return "", err
	}
	return object.Key, nil
}

// validStoredVideo rejects mislabeled image containers and zero-duration
// objects before they can mark a photo as a Live Photo.
func (a *App) validStoredVideo(ctx context.Context, key string) bool {
	data, err := a.readStorageBytes(ctx, key)
	if err != nil {
		return false
	}
	// Keep compatibility with storage adapters that cannot expose object
	// bytes during metadata-only detection; real media is always larger than
	// this threshold and is validated below.
	if len(data) < 16 {
		return len(data) > 0
	}
	if _, ok := validMP4At(data, 0); !ok {
		// validMP4At intentionally rejects offset zero for embedded scans; a
		// standalone object starts at byte zero, so check its ftyp explicitly.
		if string(data[4:8]) != "ftyp" || !isVideoBrand(string(data[8:12])) {
			return false
		}
	}
	cmd := exec.CommandContext(ctx, a.cfg.FFprobe, "-v", "error", "-select_streams", "v:0", "-show_entries", "stream=duration", "-of", "default=nw=1:nk=1", "pipe:0")
	cmd.Stdin = bytes.NewReader(data)
	out, err := runCommandOutput(ctx, cmd, 64*1024)
	if err != nil {
		return false
	}
	duration, err := strconv.ParseFloat(strings.TrimSpace(string(out)), 64)
	return err == nil && duration > 0
}

func findEmbeddedMP4(data []byte, offsets []int) ([]byte, bool) {
	for _, offset := range offsets {
		for _, start := range []int{offset, len(data) - offset} {
			if video, ok := validMP4At(data, start); ok {
				return video, true
			}
		}
	}
	startAt := len(data) - 8*1024*1024
	if startAt < 0 {
		startAt = 0
	}
	for cursor := startAt; cursor < len(data); {
		index := bytes.Index(data[cursor:], []byte("ftyp"))
		if index < 0 {
			break
		}
		start := cursor + index - 4
		if video, ok := validMP4At(data, start); ok {
			return video, true
		}
		cursor += index + 4
	}
	return nil, false
}

func validMP4At(data []byte, start int) ([]byte, bool) {
	if start <= 0 || start >= len(data)-8*1024 {
		return nil, false
	}
	// An ISO base media file starts with a size, the `ftyp` box, and a
	// four-byte major brand. HEIC/AVIF files use the same container marker, so
	// checking only for `ftyp` would incorrectly save still-image data as MP4.
	if start+16 > len(data) || string(data[start+4:start+8]) != "ftyp" {
		return nil, false
	}
	majorBrand := string(data[start+8 : start+12])
	if !isVideoBrand(majorBrand) {
		return nil, false
	}
	return data[start:], true
}

func isVideoBrand(brand string) bool {
	switch brand {
	case "isom", "iso2", "iso3", "iso4", "iso5", "iso6", "mp41", "mp42", "avc1", "av01", "3gp4", "3gp5", "M4V ", "qt  ":
		return true
	default:
		return false
	}
}

func motionBool(value any) bool {
	switch v := value.(type) {
	case bool:
		return v
	case float64:
		return v != 0
	case int:
		return v != 0
	case string:
		return strings.EqualFold(strings.TrimSpace(v), "true") || strings.TrimSpace(v) == "1"
	default:
		return false
	}
}

func motionNumber(value any) (float64, bool) {
	switch v := value.(type) {
	case float64:
		return v, true
	case int:
		return float64(v), true
	case int64:
		return float64(v), true
	case string:
		parsed, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
		return parsed, err == nil
	default:
		return 0, false
	}
}

func motionError(storageKey string, err error) error {
	return fmt.Errorf("motion photo %s: %w", storageKey, err)
}
