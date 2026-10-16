package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"regexp"
	"strings"
)

const (
	// avatarPixels is the square the picture is reduced to. The interface shows
	// an avatar at 32 and 96 CSS pixels, so this keeps it sharp on a dense
	// screen while staying a small download for every visitor.
	avatarPixels = 512
	// avatarMaxBytes bounds the file an administrator may hand over. The picture
	// is re-encoded before it is stored, so this only has to cover a
	// camera-sized original.
	avatarMaxBytes = 8 << 20
	// avatarMaxEncodedBytes bounds what FFmpeg may write back.
	avatarMaxEncodedBytes = 4 << 20
)

// avatarCrop keeps the middle of the picture, caps it at avatarPixels and never
// enlarges it: a small logo keeps its own size, a photo comes down to the
// square the interface shows it in.
const avatarCrop = "crop='min(iw,ih)':'min(iw,ih)',scale=w='min(%d,iw)':h='min(%d,ih)'"

// avatarFormats are the encodings an upload is tried in. WebP carries the same
// picture in a fraction of the bytes and keeps transparency, and it is what
// every current browser draws; PNG is built into every FFmpeg and stands in
// when a build without libwebp refuses the first attempt.
var avatarFormats = []avatarFormat{
	{ext: ".webp", contentType: "image/webp", format: "webp", codec: []string{"-c:v", "libwebp", "-quality", "90"}},
	{ext: ".png", contentType: "image/png", format: "image2pipe", codec: []string{"-c:v", "png"}},
}

type avatarFormat struct {
	ext         string
	contentType string
	format      string
	codec       []string
}

// avatarContentTypes are the formats an avatar may arrive in: the ones every
// browser draws. FFmpeg can decode more than this, but a picture no browser
// renders would only be a broken avatar wherever the site shows one.
var avatarContentTypes = map[string]bool{
	"image/jpeg": true,
	"image/png":  true,
	"image/webp": true,
	"image/gif":  true,
	"image/bmp":  true,
	"image/avif": true,
}

// avatarKeyPattern matches the name an upload writes and nothing else, so only
// an object this server stored is ever treated as one it may delete.
var avatarKeyPattern = regexp.MustCompile(`(?:^|/)avatars/(avatar-[0-9a-f]{16}\.(?:webp|png))$`)

// uploadAvatar stores the picture an administrator picked in the settings form
// and points the public avatar setting at it.
//
// The file is re-encoded before it is stored: an avatar is fetched by every
// visitor, so a camera-sized original would be the heaviest request the site
// serves. The stored name carries a hash of the encoded picture, which gives
// the browser a new address exactly when the picture changes and lets the old
// one keep its cache entry until then.
func (a *App) uploadAvatar(w http.ResponseWriter, r *http.Request) {
	if !a.requireAdmin(w, r) {
		return
	}
	// The multipart envelope adds its own headers and a boundary on top of the
	// file, so the request is capped above the file limit and the file itself is
	// measured once it has been read.
	r.Body = http.MaxBytesReader(w, r.Body, avatarMaxBytes+1<<20)
	file, header, err := r.FormFile("file")
	if err != nil {
		errorJSON(w, http.StatusBadRequest, "A multipart image file is required")
		return
	}
	defer file.Close()
	raw, err := io.ReadAll(io.LimitReader(file, avatarMaxBytes+1))
	if err != nil {
		errorJSON(w, http.StatusBadRequest, "The upload could not be read")
		return
	}
	if len(raw) > avatarMaxBytes {
		errorJSON(w, http.StatusRequestEntityTooLarge, fmt.Sprintf("The image must be smaller than %d MB", avatarMaxBytes>>20))
		return
	}
	if avatarContentType(raw, header.Header.Get("Content-Type")) == "" {
		errorJSON(w, http.StatusBadRequest, "Unsupported image format")
		return
	}
	ctx, cancel := a.mediaContext(r.Context())
	defer cancel()
	encoded, format, err := a.encodeAvatar(ctx, raw)
	if err != nil {
		a.logs.Add("storage", fmt.Sprintf("avatar could not be encoded: %v", err))
		errorJSON(w, http.StatusBadRequest, "The image could not be decoded")
		return
	}
	key := "avatars/avatar-" + avatarHash(encoded) + format.ext
	if _, err := a.storage.Create(ctx, key, encoded, format.contentType); err != nil {
		a.logs.Add("storage", fmt.Sprintf("avatar could not be stored: %v", err))
		errorJSON(w, http.StatusInternalServerError, "The avatar could not be stored")
		return
	}
	avatarURL := a.storage.PublicURL(key)
	previous := a.avatarURL()
	if err := a.updateSetting("app", "avatarUrl", avatarURL, nil, false); err != nil {
		errorJSON(w, http.StatusInternalServerError, "The avatar could not be saved")
		return
	}
	// An unchanged picture keeps its address, so a second upload of the same
	// image must not delete what the setting now points at.
	if previous != avatarURL {
		a.deleteAvatarObject(ctx, previous)
	}
	writeJSON(w, http.StatusOK, map[string]any{"avatarUrl": avatarURL})
}

// deleteAvatar drops the stored picture and clears the setting, which leaves
// the site on its default avatar.
func (a *App) deleteAvatar(w http.ResponseWriter, r *http.Request) {
	if !a.requireAdmin(w, r) {
		return
	}
	ctx, cancel := a.mediaContext(r.Context())
	defer cancel()
	a.deleteAvatarObject(ctx, a.avatarURL())
	if err := a.updateSetting("app", "avatarUrl", "", nil, false); err != nil {
		errorJSON(w, http.StatusInternalServerError, "The avatar could not be removed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"avatarUrl": ""})
}

// avatarURL is the address the site currently shows.
func (a *App) avatarURL() string {
	var value any
	if !a.readSetting("app", "avatarUrl", &value) {
		return ""
	}
	address, _ := value.(string)
	return strings.TrimSpace(address)
}

// deleteAvatarObject removes the picture an avatar address points at. An
// address this server did not write — a link typed in by hand — names an
// object the storage provider does not hold, so it is left alone.
func (a *App) deleteAvatarObject(ctx context.Context, avatarURL string) {
	key := avatarStorageKey(avatarURL)
	if key == "" {
		return
	}
	if err := a.storage.Delete(ctx, key); err != nil {
		a.logs.Add("storage", fmt.Sprintf("stored avatar %s could not be deleted: %v", key, err))
	}
}

func avatarStorageKey(avatarURL string) string {
	if index := strings.IndexAny(avatarURL, "?#"); index >= 0 {
		avatarURL = avatarURL[:index]
	}
	match := avatarKeyPattern.FindStringSubmatch(avatarURL)
	if match == nil {
		return ""
	}
	return "avatars/" + match[1]
}

func avatarHash(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:8])
}

// avatarContentType decides what the uploaded bytes are, and reports an empty
// string for anything that is not a picture a browser draws.
//
// The bytes answer first, so a browser that reports nothing or the wrong type
// cannot turn a file into an image. Only a format Go cannot recognise is taken
// from the declared type, which is how AVIF still gets through; a type the
// bytes do name stays authoritative.
func avatarContentType(data []byte, declared string) string {
	sniffed := http.DetectContentType(data)
	if avatarContentTypes[sniffed] {
		return sniffed
	}
	if sniffed != "application/octet-stream" {
		return ""
	}
	declared = strings.TrimSpace(strings.Split(declared, ";")[0])
	if avatarContentTypes[declared] {
		return declared
	}
	return ""
}

// encodeAvatar turns an upload into the stored picture, and reports which
// format came back.
func (a *App) encodeAvatar(ctx context.Context, raw []byte) ([]byte, avatarFormat, error) {
	filter := fmt.Sprintf(avatarCrop, avatarPixels, avatarPixels)
	var lastErr error
	for _, format := range avatarFormats {
		args := []string{"-hide_banner", "-loglevel", "error", "-i", "pipe:0", "-frames:v", "1", "-vf", filter}
		args = append(args, format.codec...)
		args = append(args, "-f", format.format, "pipe:1")
		cmd := exec.CommandContext(ctx, a.cfg.FFmpeg, args...)
		cmd.Stdin = bytes.NewReader(raw)
		encoded, err := runCommandOutput(ctx, cmd, avatarMaxEncodedBytes)
		if err == nil && len(encoded) > 0 {
			return encoded, format, nil
		}
		if err == nil {
			err = errors.New("ffmpeg produced no output")
		}
		lastErr = err
	}
	return nil, avatarFormat{}, lastErr
}
