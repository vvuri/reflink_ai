// Package appcfg loads the service configuration from environment variables.
// Defaults are suitable for local development; production values usually
// come from the systemd unit file (see DEPLOY.md).
package appcfg

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Config carries every tunable of the service.
type Config struct {
	// Port is the TCP port the HTTP server listens on.
	Port int
	// MaxUploadMB is the per-request image size limit in mebibytes.
	MaxUploadMB int
	// LogFile is where upload facts are appended, one line per request.
	LogFile string
	// StaticDir is the directory served under /static/ (and for index.html).
	StaticDir string
	// TrustProxy: when true the client IP is taken from the first
	// X-Forwarded-For entry. Enable only when Caddy (or another trusted
	// reverse proxy) sits in front of the service.
	TrustProxy bool
}

// Load reads configuration from the environment, applying defaults.
func Load() (*Config, error) {
	cfg := &Config{
		Port:        8080,
		MaxUploadMB: 20,
		LogFile:     "uploads.log",
		StaticDir:   "static",
	}

	if v := strings.TrimSpace(os.Getenv("PORT")); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 65535 {
			return nil, fmt.Errorf("invalid PORT %q", v)
		}
		cfg.Port = n
	}

	if v := strings.TrimSpace(os.Getenv("MAX_UPLOAD_MB")); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 512 {
			return nil, fmt.Errorf("invalid MAX_UPLOAD_MB %q", v)
		}
		cfg.MaxUploadMB = n
	}

	if v := strings.TrimSpace(os.Getenv("LOG_FILE")); v != "" {
		cfg.LogFile = v
	}

	if v := strings.TrimSpace(os.Getenv("STATIC_DIR")); v != "" {
		cfg.StaticDir = v
	}

	switch strings.ToLower(strings.TrimSpace(os.Getenv("TRUST_PROXY"))) {
	case "", "0", "false", "no", "off":
		// leave false
	case "1", "true", "yes", "on":
		cfg.TrustProxy = true
	default:
		return nil, fmt.Errorf("invalid TRUST_PROXY value %q", os.Getenv("TRUST_PROXY"))
	}

	return cfg, nil
}
