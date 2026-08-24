package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

func (a *App) rewritePhotoMetadata(ctx context.Context, key string, data []byte, updates map[string]any) ([]byte, error) {
	if len(updates) == 0 {
		return data, nil
	}
	dir, err := os.MkdirTemp("", "chronoframe-metadata-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)
	file := filepath.Join(dir, "photo"+filepath.Ext(key))
	if err := os.WriteFile(file, data, 0600); err != nil {
		return nil, err
	}
	args := []string{"-overwrite_original"}
	for field, value := range updates {
		args = append(args, "-"+field+"="+metadataValue(value))
	}
	args = append(args, file)
	cmd := exec.CommandContext(ctx, a.cfg.ExifTool, args...)
	if output, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("exiftool: %w: %s", err, strings.TrimSpace(string(output)))
	}
	updated, err := os.ReadFile(file)
	if err != nil {
		return nil, err
	}
	return updated, nil
}

func metadataValue(value any) string {
	if value == nil {
		return ""
	}
	if values, ok := value.([]string); ok {
		return strings.Join(values, ";")
	}
	return fmt.Sprint(value)
}

func locationExifUpdates(location map[string]any) map[string]any {
	if location == nil {
		return map[string]any{"GPSLatitude": nil, "GPSLatitudeRef": nil, "GPSLongitude": nil, "GPSLongitudeRef": nil, "GPSPosition": nil}
	}
	latitude, _ := location["latitude"].(float64)
	longitude, _ := location["longitude"].(float64)
	return map[string]any{
		"GPSLatitude":     strconv.FormatFloat(abs(latitude), 'f', 8, 64),
		"GPSLatitudeRef":  hemisphere(latitude, "N", "S"),
		"GPSLongitude":    strconv.FormatFloat(abs(longitude), 'f', 8, 64),
		"GPSLongitudeRef": hemisphere(longitude, "E", "W"),
		"GPSPosition":     fmt.Sprintf("%f %f", latitude, longitude),
	}
}

func abs(value float64) float64 {
	if value < 0 {
		return -value
	}
	return value
}

func hemisphere(value float64, positive, negative string) string {
	if value < 0 {
		return negative
	}
	return positive
}

func (a *App) reverseGeocode(ctx context.Context, photoID string, latitude, longitude float64) error {
	var provider any
	if a.readSetting("location", "provider", &provider) && provider == "amap" {
		var key any
		if !a.readSetting("location", "amap.key", &key) || strings.TrimSpace(fmt.Sprint(key)) == "" {
			// A public AMap key is a useful fallback for installations that only
			// configured the browser map provider.
			_ = a.readSetting("map", "amap.key", &key)
		}
		if token := strings.TrimSpace(fmt.Sprint(key)); token != "" {
			return a.reverseGeocodeAMap(ctx, photoID, token, latitude, longitude)
		}
		return fmt.Errorf("amap reverse geocoding key is not configured")
	}
	base := strings.TrimRight(a.cfg.NominatimURL, "/")
	if base == "" {
		return nil
	}
	query := url.Values{"lat": {strconv.FormatFloat(latitude, 'f', 8, 64)}, "lon": {strconv.FormatFloat(longitude, 'f', 8, 64)}, "format": {"jsonv2"}, "addressdetails": {"1"}}
	lookupCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(lookupCtx, http.MethodGet, base+"/reverse?"+query.Encode(), nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "ChronoFrame/1.0")
	resp, err := externalHTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("reverse geocoding: %s", resp.Status)
	}
	var result struct {
		DisplayName string `json:"display_name"`
		Address     struct {
			Country      string `json:"country"`
			City         string `json:"city"`
			Town         string `json:"town"`
			Village      string `json:"village"`
			Municipality string `json:"municipality"`
		} `json:"address"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&result); err != nil {
		return err
	}
	city := result.Address.City
	if city == "" {
		city = result.Address.Town
	}
	if city == "" {
		city = result.Address.Village
	}
	if city == "" {
		city = result.Address.Municipality
	}
	_, err = a.db.Exec(`UPDATE photos SET country=?,city=?,location_name=? WHERE id=?`, nullIfEmpty(result.Address.Country), nullIfEmpty(city), nullIfEmpty(result.DisplayName), photoID)
	return err
}

func (a *App) reverseGeocodeAMap(ctx context.Context, photoID, key string, latitude, longitude float64) error {
	query := url.Values{
		"key":        {key},
		"location":   {strconv.FormatFloat(longitude, 'f', 8, 64) + "," + strconv.FormatFloat(latitude, 'f', 8, 64)},
		"extensions": {"base"},
		"output":     {"json"},
	}
	lookupCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(lookupCtx, http.MethodGet, "https://restapi.amap.com/v3/geocode/regeo?"+query.Encode(), nil)
	if err != nil {
		return err
	}
	resp, err := externalHTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("amap reverse geocoding: %s", resp.Status)
	}
	var result struct {
		Status string `json:"status"`
		Info   string `json:"info"`
		Regeo  struct {
			FormattedAddress string `json:"formatted_address"`
			Component        struct {
				Country  string `json:"country"`
				Province string `json:"province"`
				City     any    `json:"city"`
				District string `json:"district"`
			} `json:"addressComponent"`
		} `json:"regeocode"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&result); err != nil {
		return err
	}
	if result.Status != "1" {
		return fmt.Errorf("amap reverse geocoding: %s", result.Info)
	}
	city := ""
	switch value := result.Regeo.Component.City.(type) {
	case string:
		city = value
	case []any:
		if len(value) > 0 {
			city, _ = value[0].(string)
		}
	}
	if city == "" {
		city = result.Regeo.Component.Province
	}
	if city == "" {
		city = result.Regeo.Component.District
	}
	_, err = a.db.Exec(`UPDATE photos SET country=?,city=?,location_name=? WHERE id=?`, nullIfEmpty(result.Regeo.Component.Country), nullIfEmpty(city), nullIfEmpty(result.Regeo.FormattedAddress), photoID)
	return err
}

func nullIfEmpty(value string) any {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return value
}

func (a *App) erasePhotoLocation(ctx context.Context, id, key string, data []byte) error {
	updated, err := a.rewritePhotoMetadata(ctx, key, data, locationExifUpdates(nil))
	if err != nil {
		return err
	}
	if _, err := a.storage.Create(ctx, key, updated, "application/octet-stream"); err != nil {
		return err
	}
	exif, _ := extractExifContext(ctx, a.cfg.ExifTool, updated, filepath.Ext(key))
	_, err = a.db.Exec(`UPDATE photos SET latitude=NULL,longitude=NULL,country=NULL,city=NULL,location_name=NULL,exif=?,last_modified=? WHERE id=?`, jsonValue(stripGPS(exif)), metadataTimestamp(), id)
	return err
}

func mergeExif(source, updates map[string]any) map[string]any {
	merged := make(map[string]any, len(source)+len(updates))
	for key, value := range source {
		merged[key] = value
	}
	for key, value := range updates {
		if value == nil {
			delete(merged, key)
			continue
		}
		merged[key] = value
	}
	return merged
}

func metadataTimestamp() string { return time.Now().UTC().Format(time.RFC3339) }
