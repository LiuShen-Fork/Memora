package main

import (
	"bytes"
	"encoding/hex"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"

	"go.n16f.net/thumbhash"
	_ "golang.org/x/image/webp"
)

// thumbnailPlaceholder returns the same hex-encoded binary ThumbHash format
// consumed by the static client. The image is already bounded by ffmpeg's
// thumbnail stage, so encoding it directly avoids a second full-size decode.
func thumbnailPlaceholder(data []byte) string {
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return ""
	}
	hash := thumbhash.EncodeImage(img)
	return hex.EncodeToString(hash)
}
