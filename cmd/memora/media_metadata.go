package main

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

func (a *App) rewritePhotoMetadata(ctx context.Context, key string, data []byte, updates map[string]any) ([]byte, error) {
	if len(updates) == 0 {
		return data, nil
	}
	dir, err := os.MkdirTemp("", "memora-metadata-")
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
	language := a.geocodingLanguage()
	var provider any
	if a.readSetting("location", "provider", &provider) && provider == "amap" {
		var key any
		if !a.readSetting("location", "amap.key", &key) || strings.TrimSpace(fmt.Sprint(key)) == "" {
			// A public AMap key is a useful fallback for installations that only
			// configured the browser map provider.
			_ = a.readSetting("map", "amap.key", &key)
		}
		if token := strings.TrimSpace(fmt.Sprint(key)); token != "" {
			return a.reverseGeocodeAMap(ctx, photoID, token, language, latitude, longitude)
		}
		return fmt.Errorf("amap reverse geocoding key is not configured")
	}
	if provider == "mapbox" {
		var token any
		if !a.readSetting("location", "mapbox.token", &token) || strings.TrimSpace(fmt.Sprint(token)) == "" {
			_ = a.readSetting("map", "mapbox.token", &token)
		}
		if value := strings.TrimSpace(fmt.Sprint(token)); value != "" {
			return a.reverseGeocodeMapbox(ctx, photoID, value, language, latitude, longitude)
		}
		return fmt.Errorf("mapbox reverse geocoding token is not configured")
	}
	if provider == "tencent" {
		// The browser map key is the same key, so an installation that only
		// filled in the map settings still gets place names; what it cannot
		// borrow is the signing secret, which only the location settings hold.
		key := a.geocodingSetting("location", "tencent.key")
		if key == "" {
			key = a.geocodingSetting("map", "tencent.key")
		}
		secret := a.geocodingSetting("location", "tencent.secretKey")
		if key == "" {
			return fmt.Errorf("tencent reverse geocoding key is not configured")
		}
		return a.reverseGeocodeTencent(ctx, photoID, key, secret, latitude, longitude)
	}
	if provider == "tianditu" {
		key := a.geocodingSetting("location", "tianditu.key")
		if key == "" {
			key = a.geocodingSetting("map", "tianditu.key")
		}
		if key == "" {
			return fmt.Errorf("tianditu reverse geocoding key is not configured")
		}
		return a.reverseGeocodeTianditu(ctx, photoID, key, a.geocodingSetting("location", "tianditu.securityKey"), latitude, longitude)
	}
	base := strings.TrimRight(a.cfg.NominatimURL, "/")
	if base == "" {
		return nil
	}
	query := url.Values{
		"lat":             {strconv.FormatFloat(latitude, 'f', 8, 64)},
		"lon":             {strconv.FormatFloat(longitude, 'f', 8, 64)},
		"format":          {"jsonv2"},
		"addressdetails":  {"1"},
		"accept-language": {language},
	}
	lookupCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(lookupCtx, http.MethodGet, base+"/reverse?"+query.Encode(), nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "Memora/1.0")
	req.Header.Set("Accept-Language", language)
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

func (a *App) reverseGeocodeAMap(ctx context.Context, photoID, key, language string, latitude, longitude float64) error {
	query := url.Values{
		"key":        {key},
		"location":   {strconv.FormatFloat(longitude, 'f', 8, 64) + "," + strconv.FormatFloat(latitude, 'f', 8, 64)},
		"extensions": {"base"},
		"output":     {"json"},
	}
	if language == "zh-CN" {
		query.Set("lang", "zh_cn")
	} else if language == "en" {
		query.Set("lang", "en")
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

func (a *App) reverseGeocodeMapbox(ctx context.Context, photoID, token, language string, latitude, longitude float64) error {
	coordinates := strconv.FormatFloat(longitude, 'f', 8, 64) + "," + strconv.FormatFloat(latitude, 'f', 8, 64)
	query := url.Values{"access_token": {token}, "language": {language}, "types": {"country,place,locality"}}
	lookupCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	endpoint := "https://api.mapbox.com/geocoding/v5/mapbox.places/" + url.PathEscape(coordinates) + ".json?" + query.Encode()
	req, err := http.NewRequestWithContext(lookupCtx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	resp, err := externalHTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("mapbox reverse geocoding: %s", resp.Status)
	}
	var result struct {
		Features []struct {
			PlaceName string `json:"place_name"`
			Text      string `json:"text"`
			Context   []struct {
				ID   string `json:"id"`
				Text string `json:"text"`
			} `json:"context"`
		} `json:"features"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&result); err != nil {
		return err
	}
	if len(result.Features) == 0 {
		return fmt.Errorf("mapbox reverse geocoding: no result")
	}
	feature := result.Features[0]
	country, city := "", feature.Text
	for _, item := range feature.Context {
		switch {
		case strings.HasPrefix(item.ID, "country"):
			country = item.Text
		case strings.HasPrefix(item.ID, "place"), strings.HasPrefix(item.ID, "locality"):
			city = item.Text
		}
	}
	_, err = a.db.Exec(`UPDATE photos SET country=?,city=?,location_name=? WHERE id=?`, nullIfEmpty(country), nullIfEmpty(city), nullIfEmpty(feature.PlaceName), photoID)
	return err
}

// geocodingSetting reads one settings row as text, and "" when it is unset.
func (a *App) geocodingSetting(namespace, key string) string {
	var value any
	if !a.readSetting(namespace, key, &value) || value == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(value))
}

// The two lookups that are tested against a local server rather than the real
// one. The address is a variable for that reason and for nothing else.
var (
	tencentGeocoderURL  = "https://apis.map.qq.com/ws/geocoder/v1"
	tiandituGeocoderURL = "https://api.tianditu.gov.cn/geocoder"
)

// tencentSignature is the value Tencent wants in `sig`.
//
// The string signed is the request path, then the parameters sorted by name and
// left exactly as they will be sent — no percent-encoding — then the secret.
// The digest is lowercase hex. The parameters must go out in the same encoded
// form afterwards; Tencent re-sorts and re-joins on its side before checking.
func tencentSignature(path string, params map[string]string, secret string) string {
	names := make([]string, 0, len(params))
	for name := range params {
		names = append(names, name)
	}
	sort.Strings(names)
	pairs := make([]string, 0, len(names))
	for _, name := range names {
		pairs = append(pairs, name+"="+params[name])
	}
	sum := md5.Sum([]byte(path + "?" + strings.Join(pairs, "&") + secret))
	return hex.EncodeToString(sum[:])
}

// reverseGeocodeTencent asks Tencent's place lookup what is at a coordinate.
//
// Tencent draws the mainland, Hong Kong and Macau on GCJ-02 and Taiwan on
// WGS-84, and its `location` is latitude first. Everything else is close to
// what the Mapbox lookup already stores: a country, a city and a line of prose.
func (a *App) reverseGeocodeTencent(ctx context.Context, photoID, key, secret string, latitude, longitude float64) error {
	lng, lat := wgs84ToGcj02(longitude, latitude)
	params := map[string]string{
		"key":      key,
		"location": strconv.FormatFloat(lat, 'f', 6, 64) + "," + strconv.FormatFloat(lng, 'f', 6, 64),
	}
	query := url.Values{}
	for name, value := range params {
		query.Set(name, value)
	}
	if secret != "" {
		query.Set("sig", tencentSignature("/ws/geocoder/v1", params, secret))
	}
	lookupCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(lookupCtx, http.MethodGet, tencentGeocoderURL+"?"+query.Encode(), nil)
	if err != nil {
		return err
	}
	resp, err := externalHTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("tencent reverse geocoding: %s", resp.Status)
	}
	var result struct {
		Status  int    `json:"status"`
		Message string `json:"message"`
		Result  struct {
			Address   string `json:"address"`
			Component struct {
				Nation   string `json:"nation"`
				Province string `json:"province"`
				City     string `json:"city"`
				District string `json:"district"`
			} `json:"address_component"`
			Formatted struct {
				Recommend string `json:"recommend"`
			} `json:"formatted_addresses"`
		} `json:"result"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&result); err != nil {
		return err
	}
	if result.Status != 0 {
		return fmt.Errorf("tencent reverse geocoding: %s", result.Message)
	}
	city := result.Result.Component.City
	if city == "" {
		city = result.Result.Component.Province
	}
	if city == "" {
		city = result.Result.Component.District
	}
	// `recommend` is the human answer ("天安门"), which is what belongs beside a
	// photograph; `address` is the administrative one kept as the fallback.
	name := result.Result.Formatted.Recommend
	if name == "" {
		name = result.Result.Address
	}
	_, err = a.db.Exec(`UPDATE photos SET country=?,city=?,location_name=? WHERE id=?`,
		nullIfEmpty(result.Result.Component.Nation), nullIfEmpty(city), nullIfEmpty(name), photoID)
	return err
}

// reverseGeocodeTianditu asks the national service what is at a coordinate.
//
// Tianditu draws on WGS-84, so the coordinate is sent as recorded. It only
// covers China, and it answers in Chinese. The security key is part of the
// request rather than something the server adds: a key configured with one is
// refused without it.
func (a *App) reverseGeocodeTianditu(ctx context.Context, photoID, key, securityKey string, latitude, longitude float64) error {
	postStr := fmt.Sprintf(`{"lon":%s,"lat":%s,"ver":1}`,
		strconv.FormatFloat(longitude, 'f', 8, 64),
		strconv.FormatFloat(latitude, 'f', 8, 64))
	query := url.Values{"postStr": {postStr}, "type": {"geocode"}, "tk": {key}}
	if securityKey != "" {
		query.Set("sk", securityKey)
	}
	lookupCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(lookupCtx, http.MethodGet, tiandituGeocoderURL+"?"+query.Encode(), nil)
	if err != nil {
		return err
	}
	resp, err := externalHTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		// The refusals that matter here are the specific ones — a key of the
		// wrong type, a missing security key — and they are stated in the body
		// as `resolve`. Reporting only "403 Forbidden" leaves the administrator
		// with nothing to act on.
		var failure struct {
			Resolve string `json:"resolve"`
			Msg     string `json:"msg"`
		}
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
		_ = json.Unmarshal(body, &failure)
		detail := failure.Resolve
		if detail == "" {
			detail = failure.Msg
		}
		if detail == "" {
			detail = resp.Status
		}
		return fmt.Errorf("tianditu reverse geocoding: %s", detail)
	}
	var result struct {
		// Older responses carry the status as a string and the permission
		// errors as a number, so it is read as whatever it is.
		Status  any `json:"status"`
		Msg     any `json:"msg"`
		Resolve any `json:"resolve"`
		Result  struct {
			FormattedAddress string `json:"formatted_address"`
			Component        struct {
				City    string `json:"city"`
				Address string `json:"address"`
			} `json:"addressComponent"`
		} `json:"result"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&result); err != nil {
		return err
	}
	if status := strings.TrimSpace(fmt.Sprint(result.Status)); status != "0" {
		message := fmt.Sprint(result.Msg)
		if message == "" || message == "<nil>" {
			message = fmt.Sprint(result.Resolve)
		}
		return fmt.Errorf("tianditu reverse geocoding: status %s: %s", status, message)
	}
	name := result.Result.FormattedAddress
	if name == "" {
		name = result.Result.Component.Address
	}
	_, err = a.db.Exec(`UPDATE photos SET country=NULL,city=?,location_name=? WHERE id=?`,
		nullIfEmpty(result.Result.Component.City), nullIfEmpty(name), photoID)
	return err
}

// geocodingLanguage maps the app's locale codes to the values accepted by
// reverse-geocoding services. It controls place-name responses, not the UI.
func (a *App) geocodingLanguage() string {
	var value any
	if !a.readSetting("location", "language", &value) {
		return "en"
	}
	switch strings.ToLower(strings.TrimSpace(fmt.Sprint(value))) {
	case "zh", "zh-cn", "zh-hans", "zh_cn":
		return "zh-CN"
	case "zh-tw", "zh-hant", "zh-hant-tw", "zh-hant-hk", "zh_tw":
		return "zh-TW"
	case "ja", "ja-jp":
		return "ja"
	case "ru", "ru-ru":
		return "ru"
	default:
		return "en"
	}
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
