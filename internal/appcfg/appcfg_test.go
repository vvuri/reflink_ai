package appcfg_test

import (
	"testing"

	"reflink/internal/appcfg"
)

func defaults(t *testing.T) *appcfg.Config {
	t.Helper()
	for _, k := range []string{"PORT", "MAX_UPLOAD_MB", "LOG_FILE", "STATIC_DIR", "TRUST_PROXY"} {
		t.Setenv(k, "")
	}
	cfg, err := appcfg.Load()
	if err != nil {
		t.Fatalf("Load with defaults: %v", err)
	}
	return cfg
}

func TestDefaults(t *testing.T) {
	cfg := defaults(t)
	if cfg.Port != 8080 || cfg.MaxUploadMB != 20 ||
		cfg.LogFile != "uploads.log" || cfg.StaticDir != "static" || cfg.TrustProxy {
		t.Fatalf("unexpected defaults: %+v", cfg)
	}
}

func TestOverrides(t *testing.T) {
	t.Setenv("PORT", "9001")
	t.Setenv("MAX_UPLOAD_MB", "5")
	t.Setenv("LOG_FILE", "/var/log/x.log")
	t.Setenv("STATIC_DIR", "www")
	t.Setenv("TRUST_PROXY", "1")
	cfg, err := appcfg.Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Port != 9001 || cfg.MaxUploadMB != 5 ||
		cfg.LogFile != "/var/log/x.log" || cfg.StaticDir != "www" || !cfg.TrustProxy {
		t.Fatalf("overrides ignored: %+v", cfg)
	}
}

func TestInvalid(t *testing.T) {
	cases := []struct{ key, val string }{
		{"PORT", "nonsense"},
		{"PORT", "70000"},
		{"PORT", "0"},
		{"MAX_UPLOAD_MB", "0"},
		{"MAX_UPLOAD_MB", "513"},
		{"TRUST_PROXY", "maybe"},
	}
	for i, c := range cases {
		for _, k := range []string{"PORT", "MAX_UPLOAD_MB", "LOG_FILE", "STATIC_DIR", "TRUST_PROXY"} {
			t.Setenv(k, "")
		}
		t.Setenv(c.key, c.val)
		if _, err := appcfg.Load(); err == nil {
			t.Fatalf("case %d (%s=%s): expected error", i, c.key, c.val)
		}
	}
}
