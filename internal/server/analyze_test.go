package server

import (
	"bytes"
	"encoding/json"
	"math"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"reflink/internal/appcfg"
	"reflink/internal/testtif"
	"reflink/internal/uploadlog"
)

// harness bundles the test server with its upload-log logger and path.
type harness struct {
	url     string
	logPath string
	ul      *uploadlog.Logger
}

func newHarness(t *testing.T, maxMB int) harness {
	t.Helper()
	dir := t.TempDir()
	cfg := &appcfg.Config{
		Port:        0, // httptest assigns its own listener
		MaxUploadMB: maxMB,
		LogFile:     filepath.Join(dir, "uploads.log"),
		StaticDir:   "../../static",
	}
	ul, err := uploadlog.New(cfg.LogFile)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ul.Close() })
	ts := httptest.NewServer(New(cfg, ul).Handler())
	t.Cleanup(ts.Close)
	return harness{url: ts.URL, logPath: cfg.LogFile, ul: ul}
}

func post(t *testing.T, url string, build func(w *multipart.Writer)) (int, map[string]any) {
	t.Helper()
	body := new(bytes.Buffer)
	w := multipart.NewWriter(body)
	build(w)
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequest(http.MethodPost, url+"/api/analyze", body)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", w.FormDataContentType())

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var m map[string]any
	raw := new(bytes.Buffer)
	if _, err := raw.ReadFrom(resp.Body); err != nil {
		t.Fatalf("reading body: %v", err)
	}
	if err := json.Unmarshal(raw.Bytes(), &m); err != nil {
		t.Fatalf("decoding response %q (status %d): %v", raw.String(), resp.StatusCode, err)
	}
	return resp.StatusCode, m
}

func postFile(t *testing.T, url, filename string, content []byte) (int, map[string]any) {
	t.Helper()
	return postUpload(t, url, fileBody{filename, content})
}

// fileBody writes a file part through an explicit part handle.
type fileBody struct {
	name    string
	content []byte
}

func postUpload(t *testing.T, url string, f fileBody) (int, map[string]any) {
	t.Helper()
	code, m := post(t, url, func(w *multipart.Writer) {
		fw, err := w.CreateFormFile("file", f.name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := fw.Write(f.content); err != nil {
			t.Fatal(err)
		}
	})
	return code, m
}

// walk(m, "exif", "settings", "iso") returns a nested JSON value.
func walk(t *testing.T, m map[string]any, parts ...string) any {
	t.Helper()
	cur := any(m)
	for _, p := range parts {
		obj, ok := cur.(map[string]any)
		if !ok {
			t.Fatalf("not an object while looking up %q in %v", p, parts)
		}
		cur, ok = obj[p]
		if !ok {
			t.Fatalf("missing key %q while looking up %v", p, parts)
		}
	}
	return cur
}

func num(t *testing.T, m map[string]any, want float64, tol float64, parts ...string) {
	t.Helper()
	got, ok := walk(t, m, parts...).(float64)
	if !ok {
		t.Fatalf("%v: not a number", parts)
	}
	if math.Abs(got-want) > tol {
		t.Fatalf("%v: got %v, want %v (±%v)", parts, got, want, tol)
	}
}

func str(t *testing.T, m map[string]any, want string, parts ...string) {
	t.Helper()
	got, ok := walk(t, m, parts...).(string)
	if !ok {
		t.Fatalf("%v: not a string", parts)
	}
	if got != want {
		t.Fatalf("%v: got %q, want %q", parts, got, want)
	}
}

func TestAnalyzeFull(t *testing.T) {
	h := newHarness(t, 20)
	code, m := postUpload(t, h.url, fileBody{"IMG_1234.jpg", testtif.JPEGWithEXIF(testtif.Phone())})
	if code != http.StatusOK {
		t.Fatalf("status: %d (%v)", code, m)
	}

	str(t, m, "IMG_1234.jpg", "file", "name")
	str(t, m, "JPEG", "image", "format")
	num(t, m, 4, 0, "image", "width")
	num(t, m, 3, 0, "image", "height")
	str(t, m, "4:3", "image", "aspect_ratio")
	num(t, m, 6, 0, "image", "orientation_code")
	str(t, m, "Rotated 90° CW", "image", "orientation")

	if b, ok := walk(t, m, "file", "sha256").(string); !ok || len(b) != 64 {
		t.Fatalf("sha256: %v", walk(t, m, "file", "sha256"))
	}

	str(t, m, "Apple", "exif", "make")
	str(t, m, "iPhone 15 Pro", "exif", "model")
	str(t, m, "Apple · iPhone 15 Pro back camera", "exif", "lens")
	str(t, m, "2026-07-14 18:32:01", "exif", "date_taken")
	num(t, m, 125, 0, "exif", "settings", "iso")
	str(t, m, "1/120 s", "exif", "settings", "exposure")
	str(t, m, "f/1.78", "exif", "settings", "aperture")
	str(t, m, "6.86 mm", "exif", "settings", "focal_length")
	str(t, m, "26 mm", "exif", "settings", "focal_length_35mm")
	str(t, m, "Fired", "exif", "settings", "flash")
	str(t, m, "Pattern", "exif", "settings", "metering")
	str(t, m, "Auto", "exif", "settings", "white_balance")
	str(t, m, "iOS 26.0", "exif", "settings", "software")

	if b, ok := walk(t, m, "gps", "present").(bool); !ok || !b {
		t.Fatal("gps.present must be true")
	}
	num(t, m, 55.75230556, 1e-5, "gps", "latitude")
	num(t, m, 37.61555556, 1e-5, "gps", "longitude")
	num(t, m, 151.5, 1e-9, "gps", "altitude_m")
	str(t, m, "55°45′8.3″N, 37°36′56.0″E", "gps", "dms")
	if s, ok := walk(t, m, "gps", "map_url").(string); !ok ||
		!strings.HasPrefix(s, "https://www.openstreetmap.org/?mlat=55.752306") {
		t.Fatalf("map_url: %v", walk(t, m, "gps", "map_url"))
	}

	// One exact-size sanity check on the human-readable size.
	if size, ok := walk(t, m, "file", "size_bytes").(float64); !ok || size <= 0 {
		t.Fatalf("size_bytes: %v", walk(t, m, "file", "size_bytes"))
	}
}

func TestAnalyzePlainJPEG(t *testing.T) {
	h := newHarness(t, 20)
	code, m := postUpload(t, h.url, fileBody{"cam.jpg", testtif.JPEG(640, 480)})
	if code != http.StatusOK {
		t.Fatalf("status: %d (%v)", code, m)
	}
	str(t, m, "JPEG", "image", "format")
	num(t, m, 640, 0, "image", "width")
	num(t, m, 480, 0, "image", "height")
	if b, ok := walk(t, m, "exif", "present").(bool); ok && b {
		t.Fatal("plain JPEG must have no EXIF")
	}
	if b, ok := walk(t, m, "gps", "present").(bool); ok && b {
		t.Fatal("plain JPEG must have no GPS")
	}
}

func TestUnsupported(t *testing.T) {
	h := newHarness(t, 20)
	code, m := postUpload(t, h.url, fileBody{"notes.txt", []byte("hello, not an image")})
	if code != http.StatusUnsupportedMediaType {
		t.Fatalf("status: %d (%v)", code, m)
	}
	if msg, _ := m["error"].(string); !strings.Contains(msg, "Unsupported") {
		t.Fatalf("error text: %v", msg)
	}
}

func TestTooLarge(t *testing.T) {
	h := newHarness(t, 1)
	// 1 MB limit + 64 KiB multipart slack; exceed both.
	code, m := postUpload(t, h.url, fileBody{"noise.bin", bytes.Repeat([]byte{0}, 1<<20+72<<10)})
	if code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status: %d (%v)", code, m)
	}

	logLine, err := os.ReadFile(h.logPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(logLine), "reason=too-large") {
		t.Fatalf("log missing rejection: %s", logLine)
	}
}

func TestNoFilePart(t *testing.T) {
	h := newHarness(t, 20)
	code, _ := post(t, h.url, func(w *multipart.Writer) {
		part, err := w.CreateFormField("note")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := part.Write([]byte("no file here")); err != nil {
			t.Fatal(err)
		}
	})
	if code != http.StatusBadRequest {
		t.Fatalf("status: %d", code)
	}
}

func TestNotMultipart(t *testing.T) {
	h := newHarness(t, 20)
	req, _ := http.NewRequest(http.MethodPost, h.url+"/api/analyze",
		strings.NewReader("raw body"))
	req.Header.Set("Content-Type", "text/plain")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status: %d", resp.StatusCode)
	}
}

func TestDamagedJPEG(t *testing.T) {
	h := newHarness(t, 20)
	code, _ := postUpload(t, h.url, fileBody{"broken.jpg", []byte{0xFF, 0xD8, 0xFF, 1, 2, 3}})
	if code != http.StatusBadRequest {
		t.Fatalf("status: %d", code)
	}
}

func TestHealthIndexStatic(t *testing.T) {
	h := newHarness(t, 20)

	resp, err := http.Get(h.url + "/api/health")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var health map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&health); err != nil {
		t.Fatal(err)
	}
	if health["status"] != "ok" || health["max_file_mb"].(float64) != 20 {
		t.Fatalf("health: %v", health)
	}
	if fmts, ok := health["formats"].([]any); !ok || len(fmts) != 5 {
		t.Fatalf("health formats: %v", health["formats"])
	}

	resp, err = http.Get(h.url + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	page := new(bytes.Buffer)
	page.ReadFrom(resp.Body)
	if resp.StatusCode != http.StatusOK || !strings.Contains(page.String(), "reflink") {
		t.Fatalf("index: %d %q", resp.StatusCode, page.String())
	}

	resp, err = http.Get(h.url + "/static/js/app.js")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("app.js: %d", resp.StatusCode)
	}

	resp, err = http.Get(h.url + "/static/js/nope.js")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("missing static asset must be 404, got %d", resp.StatusCode)
	}
}

func TestUploadLogContent(t *testing.T) {
	h := newHarness(t, 20)
	_, _ = postUpload(t, h.url, fileBody{"shot.jpg", testtif.JPEG(640, 480)})

	data, err := os.ReadFile(h.logPath)
	if err != nil {
		t.Fatal(err)
	}
	line := strings.TrimRight(string(data), "\n")
	if !strings.HasSuffix(line,
		" fmt=JPEG 640x480 exif=no gps=no status=ok") {
		t.Fatalf("unexpected log line: %q", line)
	}
	if !strings.Contains(line, ` file="shot.jpg" `) {
		t.Fatalf("unexpected file field: %q", line)
	}
	if !strings.Contains(line, " ip=127.0.0.1 ") {
		t.Fatalf("unexpected ip field: %q", line)
	}
}

func TestTrustProxy(t *testing.T) {
	dir := t.TempDir()
	cfg := &appcfg.Config{
		Port:        0,
		MaxUploadMB: 20,
		LogFile:     filepath.Join(dir, "l.log"),
		TrustProxy:  true, // pretend to stand behind Caddy
	}
	ul, err := uploadlog.New(cfg.LogFile)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ul.Close() })
	ts := httptest.NewServer(New(cfg, ul).Handler())
	defer ts.Close()

	body := new(bytes.Buffer)
	w := multipart.NewWriter(body)
	fw, err := w.CreateFormFile("file", "shot.jpg")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fw.Write(testtif.JPEG(320, 240)); err != nil {
		t.Fatal(err)
	}
	w.Close()

	req, err := http.NewRequest(http.MethodPost, ts.URL+"/api/analyze", body)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", w.FormDataContentType())
	req.Header.Set("X-Forwarded-For", "203.0.113.9, 10.0.0.1")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status: %d", resp.StatusCode)
	}

	data, err := os.ReadFile(cfg.LogFile)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), " ip=203.0.113.9 ") {
		t.Fatalf("expected first X-Forwarded-For entry in log: %q", data)
	}
}
