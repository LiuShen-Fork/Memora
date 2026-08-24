package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"mime"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type processedPhoto struct {
	raw         []byte
	image       []byte
	jpegKey     string
	width       int
	height      int
	object      Object
	contentType string
}

func (a *App) preprocessPhoto(ctx context.Context, key string, raw []byte) (processedPhoto, error) {
	result := processedPhoto{raw: raw, image: raw, contentType: mime.TypeByExtension(filepath.Ext(key))}
	if result.contentType == "" {
		result.contentType = "application/octet-stream"
	}
	if object, err := a.storage.Meta(ctx, key); err == nil {
		result.object = object
	} else {
		result.object = Object{Key: key, Size: int64(len(raw)), ModTime: time.Now()}
	}

	ext := strings.ToLower(filepath.Ext(key))
	if ext == ".heic" || ext == ".heif" || ext == ".hif" || ext == ".bmp" {
		converted, err := ffmpegJPEGContext(ctx, a.cfg.FFmpeg, raw)
		if err != nil {
			return processedPhoto{}, fmt.Errorf("convert %s: %w", ext, err)
		}
		result.image = converted
		if ext == ".heic" || ext == ".heif" || ext == ".hif" {
			jpegKey := strings.TrimSuffix(key, filepath.Ext(key)) + ".jpeg"
			object, err := a.storage.Create(ctx, jpegKey, converted, "image/jpeg")
			if err != nil {
				return processedPhoto{}, fmt.Errorf("store converted jpeg: %w", err)
			}
			result.jpegKey = object.Key
		}
	}
	result.width, result.height = imageSize(result.image)
	if result.width == 0 || result.height == 0 {
		result.width, result.height = probeSizeContext(ctx, a.cfg.FFprobe, result.image)
	}
	if result.width == 0 || result.height == 0 {
		return processedPhoto{}, errors.New("unable to read image dimensions")
	}
	return result, nil
}

func ffmpegJPEG(ffmpeg string, data []byte) ([]byte, error) {
	return ffmpegJPEGContext(context.Background(), ffmpeg, data)
}

func ffmpegJPEGContext(ctx context.Context, ffmpeg string, data []byte) ([]byte, error) {
	cmd := exec.CommandContext(ctx, ffmpeg, "-hide_banner", "-loglevel", "error", "-i", "pipe:0", "-frames:v", "1", "-f", "image2", "-c:v", "mjpeg", "-q:v", "2", "pipe:1")
	cmd.Stdin = bytes.NewReader(data)
	return cmd.Output()
}

func photoInfoFromExif(key string, exif map[string]any) (title, description any, tags []string) {
	base := strings.TrimSuffix(filepath.Base(key), filepath.Ext(key))
	title = base
	for _, name := range []string{"Title", "XPTitle", "ObjectName", "Headline"} {
		if value := exifText(exif[name]); value != "" {
			title = value
			break
		}
	}
	for _, name := range []string{"Description", "ImageDescription", "Caption-Abstract", "XPComment", "UserComment"} {
		if value := exifText(exif[name]); value != "" {
			description = value
			break
		}
	}
	tags = exifTags(exif)
	return title, description, tags
}

func exifText(value any) string {
	if value == nil {
		return ""
	}
	if values, ok := value.([]any); ok {
		parts := make([]string, 0, len(values))
		for _, item := range values {
			if text := strings.TrimSpace(fmt.Sprint(item)); text != "" {
				parts = append(parts, text)
			}
		}
		return strings.Join(parts, ", ")
	}
	return strings.TrimSpace(fmt.Sprint(value))
}

func exifFloat(value any) (float64, bool) {
	switch v := value.(type) {
	case float64:
		return v, true
	case float32:
		return float64(v), true
	case int:
		return float64(v), true
	case int64:
		return float64(v), true
	case jsonNumber:
		f, err := strconv.ParseFloat(string(v), 64)
		return f, err == nil
	case string:
		f, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
		return f, err == nil
	default:
		return 0, false
	}
}

// jsonNumber is kept local to avoid coupling the EXIF parser to decoder types.
type jsonNumber string

func gpsCoordinates(exif map[string]any) (latitude, longitude float64, ok bool) {
	latitude, latOK := exifFloat(exif["GPSLatitude"])
	longitude, lonOK := exifFloat(exif["GPSLongitude"])
	if !latOK || !lonOK {
		return 0, 0, false
	}
	if strings.EqualFold(exifText(exif["GPSLatitudeRef"]), "S") {
		latitude = -abs(latitude)
	}
	if strings.EqualFold(exifText(exif["GPSLongitudeRef"]), "W") {
		longitude = -abs(longitude)
	}
	if latitude < -90 || latitude > 90 || longitude < -180 || longitude > 180 {
		return 0, 0, false
	}
	return latitude, longitude, true
}

func (a *App) pairedLiveVideo(ctx context.Context, imageKey string) string {
	base := strings.TrimSuffix(imageKey, filepath.Ext(imageKey))
	for _, ext := range []string{".mov", ".MOV", ".mp4", ".MP4"} {
		candidate := base + ext
		if _, err := a.storage.Meta(ctx, candidate); err == nil {
			return candidate
		}
	}
	return ""
}

func publicMediaURL(storage Storage, key string) string {
	if key == "" {
		return ""
	}
	if value := storage.PublicURL(key); value != "" {
		return value
	}
	return "/image/" + strings.TrimLeft(key, "/")
}

func metadataDate(value string) string {
	if value == "" {
		return ""
	}
	if parsed, err := time.Parse(time.RFC3339, value); err == nil {
		return parsed.UTC().Format(time.RFC3339)
	}
	return value
}
