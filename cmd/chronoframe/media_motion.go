package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
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
	object, err := a.storage.Create(ctx, photoID+".mp4", video, "video/mp4")
	if err != nil {
		return "", err
	}
	return object.Key, nil
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
	end := start + 32
	if end > len(data) {
		end = len(data)
	}
	if bytes.Index(data[start:end], []byte("ftyp")) < 0 {
		return nil, false
	}
	return data[start:], true
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
