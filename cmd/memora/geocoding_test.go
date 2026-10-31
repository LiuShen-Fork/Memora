package main

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"math"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
)

func withGeocoder(t *testing.T, handler http.HandlerFunc) {
	t.Helper()
	server := httptest.NewServer(handler)
	previousT := tencentGeocoderURL
	previousD := tiandituGeocoderURL
	tencentGeocoderURL = server.URL
	tiandituGeocoderURL = server.URL
	t.Cleanup(func() {
		tencentGeocoderURL = previousT
		tiandituGeocoderURL = previousD
		server.Close()
	})
}

func insertPhotoForGeocoding(t *testing.T, app *App, id string) {
	t.Helper()
	if _, err := app.db.Exec(`INSERT INTO photos(id,storage_key) VALUES(?,?)`, id, "photos/"+id+".jpg"); err != nil {
		t.Fatal(err)
	}
}

func photoPlace(t *testing.T, app *App, id string) (country, city, name string) {
	t.Helper()
	var countryValue, cityValue, nameValue any
	if err := app.db.QueryRow(`SELECT country,city,location_name FROM photos WHERE id=?`, id).
		Scan(&countryValue, &cityValue, &nameValue); err != nil {
		t.Fatal(err)
	}
	text := func(value any) string {
		s, _ := value.(string)
		return s
	}
	return text(countryValue), text(cityValue), text(nameValue)
}

// The signature is a digest of the path, the parameters in name order, and the
// secret. What this checks is the part that is easy to get wrong and impossible
// to see in a passing request: the order, the fact that values go in unencoded,
// and that the path keeps its leading slash.
func TestTencentSignatureSortsParametersAndLeavesThemRaw(t *testing.T) {
	params := map[string]string{
		"location": "28.7033487,115.8660847",
		"key":      "ABC-DEF",
	}
	// Sorted: the key comes before the location whatever order they are held
	// in, the comma in the coordinate stays a comma, and the path keeps its
	// leading slash.
	expected := md5.Sum([]byte(
		"/ws/geocoder/v1?" +
			"key=" + "ABC-DEF" +
			"&location=" + "28.7033487,115.8660847" +
			"secret"))
	if got, want := tencentSignature("/ws/geocoder/v1", params, "secret"), hex.EncodeToString(expected[:]); got != want {
		t.Fatalf("signature = %s, want %s", got, want)
	}
}

func TestTencentLookupSendsGcj02AndStoresTheRecommendedName(t *testing.T) {
	var received url.Values
	withGeocoder(t, func(w http.ResponseWriter, r *http.Request) {
		received = r.URL.Query()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":0,"message":"Success","result":{
			"address":"北京市东城区西长安街",
			"address_component":{"nation":"中国","province":"北京市","city":"北京市","district":"东城区"},
			"formatted_addresses":{"recommend":"天安门"}}}`))
	})
	app := newTestApp(t)
	insertPhotoForGeocoding(t, app, "tencent-photo")

	if err := app.reverseGeocodeTencent(context.Background(), "tencent-photo", "ABC-DEF", "secret", 39.90923, 116.397428); err != nil {
		t.Fatalf("lookup failed: %v", err)
	}

	// Latitude first, and on the GCJ-02 grid rather than the recorded one.
	wantLng, wantLat := wgs84ToGcj02(116.397428, 39.90923)
	location := received.Get("location")
	parts := strings.Split(location, ",")
	if len(parts) != 2 {
		t.Fatalf("location sent as %q", location)
	}
	sentLat, latErr := strconv.ParseFloat(parts[0], 64)
	sentLng, lngErr := strconv.ParseFloat(parts[1], 64)
	if latErr != nil || lngErr != nil {
		t.Fatalf("location sent as %q", location)
	}
	if difference(sentLng, wantLng) > 1e-6 || difference(sentLat, wantLat) > 1e-6 {
		t.Fatalf("location = %s, want %f,%f (GCJ-02)", location, wantLat, wantLng)
	}
	if received.Get("key") != "ABC-DEF" {
		t.Fatalf("key = %q", received.Get("key"))
	}
	if received.Get("sig") != tencentSignature("/ws/geocoder/v1", map[string]string{
		"key":      "ABC-DEF",
		"location": location,
	}, "secret") {
		t.Fatalf("sig does not match the parameters sent")
	}

	country, city, name := photoPlace(t, app, "tencent-photo")
	if country != "中国" || city != "北京市" || name != "天安门" {
		t.Fatalf("stored place = %q/%q/%q", country, city, name)
	}
}

func TestTencentLookupReportsTheServiceMessage(t *testing.T) {
	withGeocoder(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":311,"message":"key格式错误","result":{}}`))
	})
	app := newTestApp(t)
	insertPhotoForGeocoding(t, app, "tencent-refused")

	err := app.reverseGeocodeTencent(context.Background(), "tencent-refused", "bad", "", 39.9, 116.4)
	if err == nil || !strings.Contains(err.Error(), "key格式错误") {
		t.Fatalf("error = %v, want the service message", err)
	}
}

func TestTiandituLookupSendsTheCoordinateAsRecorded(t *testing.T) {
	var received url.Values
	withGeocoder(t, func(w http.ResponseWriter, r *http.Request) {
		received = r.URL.Query()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"0","msg":"ok","result":{
			"formatted_address":"北京市东城区东长安街",
			"addressComponent":{"city":"北京市","address":"东长安街"}}}`))
	})
	app := newTestApp(t)
	insertPhotoForGeocoding(t, app, "tianditu-photo")

	if err := app.reverseGeocodeTianditu(context.Background(), "tianditu-photo", "tk-value", "sk-value", 39.90923, 116.397428); err != nil {
		t.Fatalf("lookup failed: %v", err)
	}

	// Tianditu draws on WGS-84, so the coordinate must not be shifted.
	postStr := received.Get("postStr")
	if !strings.Contains(postStr, `"lon":116.39742800`) || !strings.Contains(postStr, `"lat":39.90923000`) {
		t.Fatalf("postStr = %q", postStr)
	}
	if received.Get("tk") != "tk-value" || received.Get("sk") != "sk-value" {
		t.Fatalf("keys sent as %q / %q", received.Get("tk"), received.Get("sk"))
	}

	_, city, name := photoPlace(t, app, "tianditu-photo")
	if city != "北京市" || name != "北京市东城区东长安街" {
		t.Fatalf("stored place = %q/%q", city, name)
	}
}

// Tianditu states a refusal in the body rather than in the status line, and
// says so with a number where a successful answer has a string. The words are
// what an administrator can act on — "the key is a browser key" is a different
// problem from "the security key is missing" — so they have to survive as far
// as the queue's error message.
func TestTiandituLookupReportsTheServiceReason(t *testing.T) {
	withGeocoder(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"code":301012,"msg":"权限类型错误","resolve":"Key权限类型为:浏览器端，请使用浏览器访问！"}`))
	})
	app := newTestApp(t)
	insertPhotoForGeocoding(t, app, "tianditu-refused")

	err := app.reverseGeocodeTianditu(context.Background(), "tianditu-refused", "tk", "", 39.9, 116.4)
	if err == nil || !strings.Contains(err.Error(), "浏览器端") {
		t.Fatalf("error = %v, want the service's own explanation", err)
	}
}

// The shift the Chinese services draw on has to match the one the frontend
// applies in web/app/utils/coordinate-transform.ts, or a pin and the place name
// beside it describe two different spots.
func TestTheGcj02ShiftMatchesTheFrontend(t *testing.T) {
	cases := []struct {
		name     string
		lng, lat float64
		wantLng  float64
		wantLat  float64
	}{
		{"Beijing", 116.397428, 39.90923, 116.40367163, 39.91063351},
		{"Shanghai", 121.4737, 31.23037, 121.47822306, 31.22842772},
		{"Guangzhou", 113.2644, 23.1291, 113.26972959, 23.12642334},
	}
	for _, test := range cases {
		lng, lat := wgs84ToGcj02(test.lng, test.lat)
		if difference(lng, test.wantLng) > 1e-7 || difference(lat, test.wantLat) > 1e-7 {
			t.Fatalf("%s: shifted to %f,%f, want %f,%f", test.name, lng, lat, test.wantLng, test.wantLat)
		}
	}
}

// Points the shift does not apply to are handed back untouched: outside China,
// and Taiwan, which Tencent documents as WGS-84 even though it sits inside the
// country's bounding box.
func TestTheGcj02ShiftLeavesTheRestAlone(t *testing.T) {
	for _, point := range [][2]float64{
		{-0.1276, 51.5072},  // London
		{121.5654, 25.0330}, // Taipei
		{139.6917, 35.6895}, // Tokyo
	} {
		lng, lat := wgs84ToGcj02(point[0], point[1])
		if lng != point[0] || lat != point[1] {
			t.Fatalf("%v shifted to %f,%f", point, lng, lat)
		}
	}
}

func difference(a, b float64) float64 { return math.Abs(a - b) }
