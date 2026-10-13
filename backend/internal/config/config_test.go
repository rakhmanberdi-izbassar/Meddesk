package config

import (
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func environment(values map[string]string) func(string) (string, bool) {
	return func(key string) (string, bool) { value, ok := values[key]; return value, ok }
}

func TestLoadDefaultsAndOverrides(t *testing.T) {
	cfg, err := LoadFrom(environment(map[string]string{"DATABASE_URL": "postgres://localhost/meddesk?sslmode=disable"}))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Environment != "development" || cfg.HTTPAddress != ":8081" || cfg.RequestTimeout != 10*time.Second || !filepath.IsAbs(cfg.MediaDirectory) {
		t.Fatalf("incorrect defaults: %#v", cfg)
	}
	cfg, err = LoadFrom(environment(map[string]string{
		"DATABASE_URL": "postgres://localhost/meddesk?sslmode=verify-full", "APP_ENV": "production", "HTTP_ADDR": "127.0.0.1:9000",
		"MEDIA_DIR": t.TempDir(), "HTTP_REQUEST_TIMEOUT": "2s", "HTTP_SHUTDOWN_TIMEOUT": "3s",
		"AUTH_ALLOWED_ORIGINS": "https://meddesk.example.invalid", "TRUSTED_PROXY_CIDRS": "127.0.0.1/32",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.RequestTimeout != 2*time.Second || cfg.ShutdownTimeout != 3*time.Second || cfg.HTTPAddress != "127.0.0.1:9000" {
		t.Fatal("overrides were not applied")
	}
	if !cfg.CookieSecure || cfg.SessionTTL != 8*time.Hour || len(cfg.TrustedProxyCIDRs) != 1 {
		t.Fatal("authentication configuration defaults incorrect")
	}
	if cfg.MediaUploadTimeout != 2*time.Minute || cfg.MediaStreamTimeout != 5*time.Minute || cfg.MediaMaxVideoBytes != 100<<20 {
		t.Fatal("media configuration defaults incorrect")
	}
}

func TestInvalidConfigurationRejectsWithoutLeakingSecrets(t *testing.T) {
	for _, tc := range []struct{ name, key, value string }{
		{"missing database", "DATABASE_URL", ""},
		{"invalid database", "DATABASE_URL", "secret-password://bad"},
		{"missing database name", "DATABASE_URL", "postgres://localhost"},
		{"production TLS", "APP_ENV", "production"},
		{"unknown environment", "APP_ENV", "staging"},
		{"address format", "HTTP_ADDR", "8081"},
		{"invalid port", "HTTP_ADDR", ":65536"},
		{"empty media", "MEDIA_DIR", ""},
		{"invalid request duration", "HTTP_REQUEST_TIMEOUT", "secret-password"},
		{"request below bound", "HTTP_REQUEST_TIMEOUT", "1ms"},
		{"request above bound", "HTTP_REQUEST_TIMEOUT", "3m"},
		{"invalid shutdown duration", "HTTP_SHUTDOWN_TIMEOUT", "0s"},
		{"invalid session duration", "SESSION_TTL", "30s"},
		{"oversized session duration", "SESSION_TTL", "25h"},
		{"short upload timeout", "MEDIA_UPLOAD_TIMEOUT", "1s"},
		{"long upload timeout", "MEDIA_UPLOAD_TIMEOUT", "11m"},
		{"long stream timeout", "MEDIA_STREAM_TIMEOUT", "31m"},
		{"invalid video size", "MEDIA_MAX_VIDEO_BYTES", "secret-password"},
		{"oversize video policy", "MEDIA_MAX_VIDEO_BYTES", "524288001"},
		{"cookie boolean", "COOKIE_SECURE", "secret-password"},
		{"wildcard origin", "AUTH_ALLOWED_ORIGINS", "https://*.example.invalid"},
		{"origin with path", "AUTH_ALLOWED_ORIGINS", "https://example.invalid/path"},
		{"origin with empty query", "AUTH_ALLOWED_ORIGINS", "https://example.invalid?"},
		{"origin with empty fragment", "AUTH_ALLOWED_ORIGINS", "https://example.invalid#"},
		{"global proxy trust", "TRUSTED_PROXY_CIDRS", "0.0.0.0/0"},
		{"invalid proxy CIDR", "TRUSTED_PROXY_CIDRS", "secret-password"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			values := map[string]string{"DATABASE_URL": "postgres://user:secret-password@localhost/meddesk?sslmode=disable"}
			values[tc.key] = tc.value
			_, err := LoadFrom(environment(values))
			if err == nil {
				t.Fatal("expected configuration rejection")
			}
			if strings.Contains(err.Error(), "secret-password") {
				t.Fatal("error leaks configuration value")
			}
		})
	}
}

func TestProductionAuthenticationFailClosed(t *testing.T) {
	for _, values := range []map[string]string{
		{},
		{"AUTH_ALLOWED_ORIGINS": "http://example.invalid"},
		{"AUTH_ALLOWED_ORIGINS": "https://example.invalid", "COOKIE_SECURE": "false"},
	} {
		values["APP_ENV"] = "production"
		values["DATABASE_URL"] = "postgres://localhost/meddesk?sslmode=verify-full"
		if _, err := LoadFrom(environment(values)); err == nil {
			t.Fatal("production authentication should reject unsafe defaults")
		}
	}
}
