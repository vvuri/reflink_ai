// Package server wires the HTTP surface of RefLink: the single-page UI,
// the /api/analyze endpoint and the health check.
package server

import (
	"net"
	"net/http"
	"path/filepath"
	"strings"

	"reflink/internal/appcfg"
	"reflink/internal/imageinfo"
	"reflink/internal/uploadlog"
)

// Server holds the HTTP handler dependencies.
type Server struct {
	cfg *appcfg.Config
	ul  *uploadlog.Logger
}

// New builds a Server.
func New(cfg *appcfg.Config, ul *uploadlog.Logger) *Server {
	return &Server{cfg: cfg, ul: ul}
}

// maxSlackBytes is added on top of the upload limit, so multipart framing
// overhead does not reject a legal 20 MB image.
const maxSlackBytes = 64 << 10

// Handler builds the root handler with routes and headers middleware.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", s.handleIndex)
	mux.Handle("GET /static/", http.StripPrefix("/static/", s.staticHandler()))
	mux.HandleFunc("GET /api/health", s.handleHealth)
	mux.HandleFunc("POST /api/analyze", s.handleAnalyze)
	mux.HandleFunc("GET /api/analyze", s.handleAnalyzeWrongMethod)
	return s.headers(mux)
}

// handleIndex serves the single page of the app.
func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-cache")
	http.ServeFile(w, r, filepath.Join(s.cfg.StaticDir, "index.html"))
}

// staticHandler serves css/js assets.
func (s *Server) staticHandler() http.Handler {
	fs := http.FileServer(http.Dir(s.cfg.StaticDir))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-cache")
		fs.ServeHTTP(w, r)
	})
}

// headers installs a compact set of hardening headers. The CSP is strict:
// no inline scripts or styles are used anywhere in the static assets.
func (s *Server) headers(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Content-Security-Policy", "default-src 'none'; "+
			"script-src 'self'; style-src 'self'; "+
			"img-src 'self' data: blob:; connect-src 'self'; "+
			"base-uri 'none'; frame-ancestors 'none'")
		next.ServeHTTP(w, r)
	})
}

// clientIP returns the address to log for this request. X-Forwarded-For is
// trusted only when TrustProxy is enabled (service behind Caddy).
func (s *Server) clientIP(r *http.Request) string {
	host := r.RemoteAddr
	if h, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		host = h
	}
	if !s.cfg.TrustProxy {
		return host
	}
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if first := strings.TrimSpace(strings.Split(xff, ",")[0]); first != "" {
			return first
		}
	}
	return host
}

// orderedFormats is the display order for the health endpoint.
func orderedFormats() []string {
	return []string{
		imageinfo.FormatJPEG,
		imageinfo.FormatPNG,
		imageinfo.FormatWebP,
		imageinfo.FormatGIF,
		imageinfo.FormatBMP,
	}
}
