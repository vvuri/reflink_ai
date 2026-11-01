package server

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"mime"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"

	"reflink/internal/exif"
	"reflink/internal/imageinfo"
	"reflink/internal/uploadlog"
)

// healthResponse is the JSON answer of GET /api/health. The limits travel
// with the health check so the frontend can stay in sync without templates.
type healthResponse struct {
	Status    string   `json:"status"`
	MaxFileMB int      `json:"max_file_mb"`
	Formats   []string `json:"formats"`
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, healthResponse{
		Status:    "ok",
		MaxFileMB: s.cfg.MaxUploadMB,
		Formats:   orderedFormats(),
	})
}

// analyzeResponse is the JSON answer of POST /api/analyze.
type analyzeResponse struct {
	File  fileView  `json:"file"`
	Image imageView `json:"image"`
	EXIF  exifView  `json:"exif"`
	GPS   gpsView   `json:"gps"`
}

type fileView struct {
	Name      string `json:"name"`
	SizeBytes int64  `json:"size_bytes"`
	SizeHuman string `json:"size_human"`
	SHA256    string `json:"sha256"`
}

type imageView struct {
	Format          string  `json:"format"`
	Width           int     `json:"width"`
	Height          int     `json:"height"`
	Megapixels      float64 `json:"megapixels"`
	AspectRatio     string  `json:"aspect_ratio"`
	OrientationCode int     `json:"orientation_code,omitempty"` // EXIF 1..8, 0 unknown
	Orientation     string  `json:"orientation,omitempty"`
}

type settingsView struct {
	ISO           int    `json:"iso,omitempty"`
	Exposure      string `json:"exposure,omitempty"`
	Aperture      string `json:"aperture,omitempty"`
	FocalLength   string `json:"focal_length,omitempty"`
	FocalLength35 string `json:"focal_length_35mm,omitempty"`
	Flash         string `json:"flash,omitempty"`
	Metering      string `json:"metering,omitempty"`
	WhiteBalance  string `json:"white_balance,omitempty"`
	Software      string `json:"software,omitempty"`
}

type exifView struct {
	Present   bool         `json:"present"`
	Make      string       `json:"make,omitempty"`
	Model     string       `json:"model,omitempty"`
	Lens      string       `json:"lens,omitempty"`
	DateTaken string       `json:"date_taken,omitempty"`
	Settings  settingsView `json:"settings"`
}

type gpsView struct {
	Present   bool     `json:"present"`
	Latitude  float64  `json:"latitude,omitempty"`
	Longitude float64  `json:"longitude,omitempty"`
	AltitudeM *float64 `json:"altitude_m,omitempty"`
	DMS       string   `json:"dms,omitempty"`
	MapURL    string   `json:"map_url,omitempty"`
}

// handleAnalyze accepts one image in the "file" multipart field, decodes
// everything interesting from it in memory and answers with JSON.
// The bytes are never written to disk.
func (s *Server) handleAnalyze(w http.ResponseWriter, r *http.Request) {
	ip := s.clientIP(r)
	maxFile := int64(s.cfg.MaxUploadMB) << 20
	r.Body = http.MaxBytesReader(w, r.Body, maxFile+maxSlackBytes)

	data, name, readErr := readUpload(r, maxFile)
	if readErr != nil {
		switch {
		case errors.Is(readErr, errTooLarge):
			s.ul.Upload(uploadEntry(ip, name, int64(len(data)), "too-large"))
			writeJSON(w, http.StatusRequestEntityTooLarge,
				errorBody(fmt.Sprintf("The file exceeds the %d MB limit.", s.cfg.MaxUploadMB)))
		case errors.Is(readErr, errNoFile):
			s.ul.Upload(uploadEntry(ip, name, int64(len(data)), "no-file"))
			writeJSON(w, http.StatusBadRequest,
				errorBody("Upload the image in the 'file' multipart field."))
		default:
			s.ul.Upload(uploadEntry(ip, name, int64(len(data)), "malformed-request"))
			writeJSON(w, http.StatusBadRequest,
				errorBody("Malformed multipart request: expected 'file' with an image."))
		}
		return
	}
	if name == "" {
		name = "unnamed"
	}

	info, parseErr := imageinfo.Parse(data)
	if parseErr != nil {
		if errors.Is(parseErr, imageinfo.ErrUnsupported) {
			s.ul.Upload(uploadEntry(ip, name, int64(len(data)), "unsupported-format"))
			writeJSON(w, http.StatusUnsupportedMediaType,
				errorBody("Unsupported image format. Supported: JPEG, PNG, WebP, GIF, BMP."))
			return
		}
		s.ul.Upload(uploadEntry(ip, name, int64(len(data)), "malformed-image"))
		writeJSON(w, http.StatusBadRequest,
			errorBody("The file was recognized as an image but is damaged or truncated."))
		return
	}

	resp := s.buildResponse(name, data, info)

	s.ul.Upload(uploadlog.Entry{
		IP:      ip,
		File:    name,
		Size:    int64(len(data)),
		Format:  info.Format,
		Width:   info.Width,
		Height:  info.Height,
		HasEXIF: resp.EXIF.Present,
		HasGPS:  resp.GPS.Present,
	})

	writeJSON(w, http.StatusOK, resp)
}

// handleAnalyzeWrongMethod answers GET / HEAD on the analyze endpoint with a
// JSON 405 instead of the mux's plain-text response.
func (s *Server) handleAnalyzeWrongMethod(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Allow", "POST")
	writeJSON(w, http.StatusMethodNotAllowed,
		errorBody("Use POST with a 'file' multipart field to analyze an image."))
}

// buildResponse assembles the JSON view from the parsed image payload
// and its EXIF data.
func (s *Server) buildResponse(name string, data []byte, info *imageinfo.Result) analyzeResponse {
	sum := sha256.Sum256(data)
	resp := analyzeResponse{
		File: fileView{
			Name:      name,
			SizeBytes: int64(len(data)),
			SizeHuman: humanBytes(int64(len(data))),
			SHA256:    hex.EncodeToString(sum[:]),
		},
		Image: imageView{
			Format:      info.Format,
			Width:       info.Width,
			Height:      info.Height,
			Megapixels:  megapixels(info.Width, info.Height),
			AspectRatio: aspectRatio(info.Width, info.Height),
		},
		GPS: gpsView{Present: false},
	}

	if len(info.EXIF) == 0 {
		return resp
	}
	ex, err := exif.Parse(info.EXIF)
	if err != nil || !ex.Present {
		return resp // damaged or empty metadata: report nothing but basics
	}

	if code := ex.Orientation; code != 0 {
		resp.Image.OrientationCode = code
		resp.Image.Orientation = exif.OrientationText(code)
	}

	resp.EXIF.Present = true
	resp.EXIF.Make = ex.Make
	resp.EXIF.Model = ex.Model
	resp.EXIF.Lens = lensLabel(ex)
	resp.EXIF.DateTaken = formatEXIFDate(ex.DateTimeOriginal, ex.DateTime)
	resp.EXIF.Settings = settingsView{
		ISO:           ex.ISO,
		Exposure:      exif.FormatExposure(ex.ExposureTime),
		Aperture:      exif.FormatAperture(ex.FNumber),
		FocalLength:   exif.FormatFocalLength(ex.FocalLength),
		FocalLength35: focal35(ex.FocalLength35),
		Flash:         exif.FlashText(ex.Flash),
		Metering:      exif.MeteringText(ex.MeteringMode),
		WhiteBalance:  exif.WhiteBalanceText(ex.WhiteBalance),
		Software:      ex.Software,
	}

	if ex.GPS.Present && !(ex.GPS.Latitude == 0 && ex.GPS.Longitude == 0) {
		resp.GPS.Present = true
		resp.GPS.Latitude = ex.GPS.Latitude
		resp.GPS.Longitude = ex.GPS.Longitude
		resp.GPS.DMS = exif.FormatCoordPair(ex.GPS.Latitude, ex.GPS.Longitude)
		resp.GPS.MapURL = openStreetMap(ex.GPS.Latitude, ex.GPS.Longitude)
		if ex.GPS.HasAltitude {
			alt := ex.GPS.Altitude
			if ex.GPS.AltitudeBelow {
				alt = -alt
			}
			resp.GPS.AltitudeM = &alt
		}
	}
	return resp
}

// upload log rejected entries use the compact uploadlog.Entry builder.
func uploadEntry(ip, name string, size int64, reason string) uploadlog.Entry {
	return uploadlog.Entry{IP: ip, File: name, Size: size, Denied: reason}
}

// reject reasons returned by readUpload.
var (
	errNoFile   = errors.New("no 'file' part found")
	errTooLarge = errors.New("file exceeds the size limit")
)

// readUpload consumes the multipart "file" field with a hard cap on the
// request body (installed by the caller) and returns file bytes plus the
// client-side filename. Nothing is written to disk: parts are streamed into
// a memory buffer instead of r.ParseMultipartForm, which would spill to
// temporary files.
func readUpload(r *http.Request, maxFile int64) ([]byte, string, error) {
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || !strings.HasPrefix(mediaType, "multipart/") {
		return nil, "", errNoFile
	}
	mr, err := r.MultipartReader()
	if err != nil {
		if tooLarge(err) {
			return nil, "", errTooLarge
		}
		return nil, "", fmt.Errorf("malformed multipart body: %w", err)
	}

	var (
		data     []byte
		filename string
	)
	for {
		part, perr := mr.NextPart()
		if perr == io.EOF {
			break
		}
		if perr != nil {
			if tooLarge(perr) {
				return nil, "", errTooLarge
			}
			return nil, "", fmt.Errorf("malformed multipart body: %w", perr)
		}
		if part.FormName() != "file" {
			if _, cerr := io.Copy(io.Discard, part); cerr != nil && tooLarge(cerr) {
				return nil, "", errTooLarge
			}
			continue
		}
		if data == nil {
			var buf bytes.Buffer
			if _, cerr := io.Copy(&buf, part); cerr != nil {
				if tooLarge(cerr) {
					return nil, "", errTooLarge
				}
				return nil, "", fmt.Errorf("upload read failed: %w", cerr)
			}
			if int64(buf.Len()) > maxFile {
				return nil, "", errTooLarge
			}
			data = buf.Bytes()
			filename = sanitizeName(part.FileName())
		} else if _, cerr := io.Copy(io.Discard, part); cerr != nil && tooLarge(cerr) {
			return nil, "", errTooLarge
		}
	}
	if data == nil {
		return nil, "", errNoFile
	}
	return data, filename, nil
}

// tooLarge unwraps http.MaxBytesError emitted by http.MaxBytesReader.
func tooLarge(err error) bool {
	var mbe *http.MaxBytesError
	return errors.As(err, &mbe)
}

// sanitizeName keeps only the base filename, without control characters.
func sanitizeName(name string) string {
	name = strings.ReplaceAll(name, "\\", "/")
	name = filepath.Base(name)
	name = strings.TrimSpace(name)
	return strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7F {
			return -1
		}
		return r
	}, name)
}

// writeJSON sends v with the status line attached.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// errorBody is the JSON error facet of the API: {"error": "..."}.
func errorBody(msg string) map[string]string {
	return map[string]string{"error": msg}
}

// --- small display helpers ------------------------------------------------

// humanBytes renders 3821945 as "3.6 MB".
func humanBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for m := n / unit; m >= unit; m /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(n)/float64(div), "KMGTPE"[exp])
}

// megapixels renders 4032×3024 as 12.19 MP, rounded to two decimals.
func megapixels(w, h int) float64 {
	if w <= 0 || h <= 0 {
		return 0
	}
	mp := float64(w) * float64(h) / 1e6
	return math.Round(mp*100) / 100
}

// aspectRatio reduces W:H via GCD, e.g. 4032:3024 → "4:3".
func aspectRatio(w, h int) string {
	if w <= 0 || h <= 0 {
		return ""
	}
	g := gcd(w, h)
	return fmt.Sprintf("%d:%d", w/g, h/g)
}

func gcd(a, b int) int {
	for b != 0 {
		a, b = b, a%b
	}
	if a < 0 {
		return -a
	}
	return a
}

// lensLabel joins lens make/model ("Apple · iPhone 15 Pro back camera").
func lensLabel(ex *exif.Data) string {
	switch {
	case ex.LensMake != "" && ex.LensModel != "":
		return ex.LensMake + " · " + ex.LensModel
	}
	return ex.LensModel
}

// focal35 renders the 35mm-equivalent focal length, e.g. "26 mm".
func focal35(mm int) string {
	if mm <= 0 {
		return ""
	}
	return strconv.Itoa(mm) + " mm"
}

// formatEXIFDate prefers DateTimeOriginal and rewrites the EXIF date
// separator ("2006:07:14 18:32:01" → "2006-07-14 18:32:01").
func formatEXIFDate(original, ifd0 string) string {
	raw := original
	if raw == "" {
		raw = ifd0
	}
	if raw == "" {
		return ""
	}
	if len(raw) >= 11 && raw[4] == ':' && raw[7] == ':' {
		raw = raw[:4] + "-" + raw[5:7] + "-" + raw[8:]
	}
	return raw
}

// openStreetMap builds a zoomed link to the coordinates.
func openStreetMap(lat, lon float64) string {
	return fmt.Sprintf(
		"https://www.openstreetmap.org/?mlat=%.6f&mlon=%.6f#map=15/%.6f/%.6f",
		lat, lon, lat, lon)
}
