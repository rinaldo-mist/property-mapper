// Package config loads and validates process configuration from the environment.
//
// It is hand-written rather than struct-tag driven: there are about a dozen
// variables, and a tag DSL tends to hide which of them are genuinely required.
// Load reports *every* problem at once, so a misconfigured deploy fails on boot
// with the full list instead of one variable per restart.
//
// There is no .env loading here. The Taskfile injects .env for local commands,
// and Cloud Run injects real environment variables in production; a library that
// silently reads a file in one environment and not the other is a debugging trap.
package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	DatabaseURL string

	Port     string
	AppEnv   string
	LogLevel string

	CORSAllowedOrigins []string

	JWTSecret    []byte
	CookieSecure bool
	SessionTTL   time.Duration
}

const (
	EnvDevelopment = "development"
	EnvProduction  = "production"

	// minSecretLen is the floor for an HS256 signing key: a shorter secret
	// weakens the signature regardless of how the token is used.
	minSecretLen = 32
)

func (c Config) IsProduction() bool { return c.AppEnv == EnvProduction }

// Load reads the full server configuration.
func Load() (Config, error) {
	var problems []string
	fail := func(format string, args ...any) {
		problems = append(problems, fmt.Sprintf(format, args...))
	}

	cfg := Config{
		DatabaseURL: os.Getenv("DATABASE_URL"),
		// Cloud Run injects PORT; a hardcoded listen address fails to start there.
		Port:     envOr("PORT", "8080"),
		AppEnv:   envOr("APP_ENV", EnvDevelopment),
		LogLevel: envOr("LOG_LEVEL", "info"),
	}

	if cfg.DatabaseURL == "" {
		fail("DATABASE_URL is required")
	}
	if cfg.AppEnv != EnvDevelopment && cfg.AppEnv != EnvProduction {
		fail("APP_ENV must be %q or %q, got %q", EnvDevelopment, EnvProduction, cfg.AppEnv)
	}
	switch cfg.LogLevel {
	case "debug", "info", "warn", "error":
	default:
		fail("LOG_LEVEL must be one of debug|info|warn|error, got %q", cfg.LogLevel)
	}

	secret := os.Getenv("AUTH_JWT_SECRET")
	switch {
	case secret == "":
		fail("AUTH_JWT_SECRET is required (generate with: openssl rand -base64 48)")
	case len(secret) < minSecretLen:
		fail("AUTH_JWT_SECRET must be at least %d bytes, got %d", minSecretLen, len(secret))
	default:
		cfg.JWTSecret = []byte(secret)
	}

	secure, err := envBool("AUTH_COOKIE_SECURE", false)
	if err != nil {
		fail("%v", err)
	}
	cfg.CookieSecure = secure

	ttl, err := envDuration("AUTH_SESSION_TTL", 12*time.Hour)
	if err != nil {
		fail("%v", err)
	}
	cfg.SessionTTL = ttl

	cfg.CORSAllowedOrigins = splitList(os.Getenv("CORS_ALLOWED_ORIGINS"))

	// Serving a session cookie without Secure in production would let any
	// downgrade attack read it, so refuse to start rather than warn.
	if cfg.AppEnv == EnvProduction && !cfg.CookieSecure {
		fail("AUTH_COOKIE_SECURE must be true when APP_ENV=production")
	}

	if len(problems) > 0 {
		return Config{}, fmt.Errorf("invalid configuration:\n  - %s", strings.Join(problems, "\n  - "))
	}
	return cfg, nil
}

// LoadDatabaseURL reads only the database DSN, for CLI subcommands that never
// serve HTTP and so have no business demanding a JWT secret.
func LoadDatabaseURL() (string, error) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		return "", errors.New("DATABASE_URL is required")
	}
	return dsn, nil
}

func envOr(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

func envBool(key string, fallback bool) (bool, error) {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback, nil
	}
	v, err := strconv.ParseBool(raw)
	if err != nil {
		return false, fmt.Errorf("%s must be a boolean, got %q", key, raw)
	}
	return v, nil
}

func envDuration(key string, fallback time.Duration) (time.Duration, error) {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback, nil
	}
	v, err := time.ParseDuration(raw)
	if err != nil {
		return 0, fmt.Errorf("%s must be a duration such as 12h, got %q", key, raw)
	}
	if v <= 0 {
		return 0, fmt.Errorf("%s must be positive, got %q", key, raw)
	}
	return v, nil
}

func splitList(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
