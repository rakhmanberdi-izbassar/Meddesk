// Package config reads and validates environment configuration without exposing secrets.
package config

import (
	"errors"
	"fmt"
	"net"
	"net/netip"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Config is application configuration. Never log the full value: DatabaseURL is secret.
type Config struct {
	Environment        string
	DatabaseURL        string
	DatabaseCACert     string
	HTTPAddress        string
	MediaDirectory     string
	RequestTimeout     time.Duration
	ShutdownTimeout    time.Duration
	SessionTTL         time.Duration
	CookieSecure       bool
	AllowedOrigins     []string
	TrustedProxyCIDRs  []netip.Prefix
	MediaUploadTimeout time.Duration
	MediaStreamTimeout time.Duration
	MediaMaxVideoBytes int64
}

// Load reads configuration from the process environment.
func Load() (Config, error) { return LoadFrom(os.LookupEnv) }

// LoadFrom accepts an environment reader so configuration can be tested in isolation.
func LoadFrom(lookup func(string) (string, bool)) (Config, error) {
	get := func(name, fallback string) string {
		if value, ok := lookup(name); ok {
			return strings.TrimSpace(value)
		}
		return fallback
	}
	cfg := Config{
		Environment:    get("APP_ENV", "development"),
		DatabaseURL:    get("DATABASE_URL", ""),
		DatabaseCACert: get("DATABASE_CA_CERT", ""),
		HTTPAddress:    get("HTTP_ADDR", ":8081"),
		MediaDirectory: get("MEDIA_DIR", "./var/media"),
	}
	if cfg.Environment != "development" && cfg.Environment != "test" && cfg.Environment != "production" {
		return Config{}, errors.New("APP_ENV must be development, test, or production")
	}
	parsed, err := url.Parse(cfg.DatabaseURL)
	if err != nil || (parsed.Scheme != "postgres" && parsed.Scheme != "postgresql") || parsed.Hostname() == "" || strings.Trim(parsed.Path, "/") == "" {
		return Config{}, errors.New("DATABASE_URL must be a PostgreSQL URL with a host and database name")
	}
	if cfg.Environment == "production" && parsed.Query().Get("sslmode") != "verify-full" {
		return Config{}, errors.New("production DATABASE_URL requires sslmode=verify-full")
	}
	_, port, err := net.SplitHostPort(cfg.HTTPAddress)
	if err != nil {
		return Config{}, errors.New("HTTP_ADDR must use host:port format")
	}
	n, err := strconv.Atoi(port)
	if err != nil || n < 1 || n > 65535 {
		return Config{}, errors.New("HTTP_ADDR port must be between 1 and 65535")
	}
	if cfg.MediaDirectory == "" {
		return Config{}, errors.New("MEDIA_DIR must not be empty")
	}
	cfg.MediaDirectory, err = filepath.Abs(cfg.MediaDirectory)
	if err != nil {
		return Config{}, errors.New("MEDIA_DIR must resolve to an absolute path")
	}
	cfg.RequestTimeout, err = duration(get("HTTP_REQUEST_TIMEOUT", "10s"), "HTTP_REQUEST_TIMEOUT", 100*time.Millisecond, 2*time.Minute)
	if err != nil {
		return Config{}, err
	}
	cfg.ShutdownTimeout, err = duration(get("HTTP_SHUTDOWN_TIMEOUT", "10s"), "HTTP_SHUTDOWN_TIMEOUT", time.Second, 2*time.Minute)
	if err != nil {
		return Config{}, err
	}
	cfg.MediaUploadTimeout, err = duration(get("MEDIA_UPLOAD_TIMEOUT", "2m"), "MEDIA_UPLOAD_TIMEOUT", 5*time.Second, 10*time.Minute)
	if err != nil {
		return Config{}, err
	}
	cfg.MediaStreamTimeout, err = duration(get("MEDIA_STREAM_TIMEOUT", "5m"), "MEDIA_STREAM_TIMEOUT", 5*time.Second, 30*time.Minute)
	if err != nil {
		return Config{}, err
	}
	cfg.MediaMaxVideoBytes, err = strconv.ParseInt(get("MEDIA_MAX_VIDEO_BYTES", "104857600"), 10, 64)
	if err != nil || cfg.MediaMaxVideoBytes < 1048576 || cfg.MediaMaxVideoBytes > 524288000 {
		return Config{}, errors.New("MEDIA_MAX_VIDEO_BYTES must be between 1048576 and 524288000")
	}
	cfg.SessionTTL, err = duration(get("SESSION_TTL", "8h"), "SESSION_TTL", time.Minute, 24*time.Hour)
	if err != nil {
		return Config{}, err
	}
	cfg.CookieSecure, err = strconv.ParseBool(get("COOKIE_SECURE", strconv.FormatBool(cfg.Environment == "production")))
	if err != nil || (cfg.Environment == "production" && !cfg.CookieSecure) {
		return Config{}, errors.New("COOKIE_SECURE must be boolean and true in production")
	}
	originDefault := "http://localhost:8080,http://127.0.0.1:8080,http://localhost:5173,http://127.0.0.1:5173"
	if cfg.Environment == "production" {
		originDefault = ""
	}
	for _, value := range strings.Split(get("AUTH_ALLOWED_ORIGINS", originDefault), ",") {
		value = strings.TrimSpace(value)
		origin, err := url.Parse(value)
		if err != nil || origin.Hostname() == "" || origin.User != nil || origin.Path != "" || origin.RawQuery != "" || origin.ForceQuery || strings.Contains(value, "#") ||
			(origin.Scheme != "http" && origin.Scheme != "https") || strings.Contains(origin.Host, "*") || (cfg.Environment == "production" && origin.Scheme != "https") {
			return Config{}, errors.New("AUTH_ALLOWED_ORIGINS requires exact HTTP origins; production requires explicit HTTPS origins")
		}
		cfg.AllowedOrigins = append(cfg.AllowedOrigins, value)
	}
	for _, value := range strings.Split(get("TRUSTED_PROXY_CIDRS", ""), ",") {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		prefix, err := netip.ParsePrefix(value)
		if err != nil || prefix.Bits() == 0 {
			return Config{}, errors.New("TRUSTED_PROXY_CIDRS requires explicit, non-global CIDR prefixes")
		}
		cfg.TrustedProxyCIDRs = append(cfg.TrustedProxyCIDRs, prefix.Masked())
	}
	return cfg, nil
}

func duration(value, name string, minimum, maximum time.Duration) (time.Duration, error) {
	d, err := time.ParseDuration(value)
	if err != nil || d < minimum || d > maximum {
		return 0, fmt.Errorf("%s must be a duration between %s and %s", name, minimum, maximum)
	}
	return d, nil
}
