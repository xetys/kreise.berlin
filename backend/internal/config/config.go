// Package config loads and validates application configuration from environment variables.
package config

import (
	"encoding/base64"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"
)

type Config struct {
	Env      string
	Addr     string
	LogLevel slog.Level

	DatabaseURL string

	SessionCookieName string
	SessionTTL        time.Duration

	MailFrom               string
	MailFromDisplayName    string
	MailUnsubscribeAddress string
	AWSRegion              string
	SESConfigurationSet    string

	StorageEndpoint     string
	StorageRegion       string
	StorageBucket       string
	StorageAccessKey    string
	StorageSecretKey    string
	StorageUsePathStyle bool

	// TokenSigningKey is base64-decoded random bytes (32+ recommended) used
	// to HMAC-sign QR + view tokens. Required outside dev.
	TokenSigningKey []byte

	PublicBaseURL string
}

func Load() (*Config, error) {
	cfg := &Config{
		Env:                    getenv("APP_ENV", "dev"),
		Addr:                   getenv("APP_ADDR", ":8080"),
		DatabaseURL:            os.Getenv("DATABASE_URL"),
		SessionCookieName:      getenv("SESSION_COOKIE_NAME", "tg_session"),
		MailFrom:               getenv("MAIL_FROM", "no-reply@tickets.local"),
		MailFromDisplayName:    os.Getenv("MAIL_FROM_DISPLAY_NAME"),
		MailUnsubscribeAddress: os.Getenv("MAIL_UNSUBSCRIBE_ADDRESS"),
		AWSRegion:              getenv("AWS_REGION", "eu-central-1"),
		SESConfigurationSet:    os.Getenv("SES_CONFIGURATION_SET"),
		StorageEndpoint:        os.Getenv("STORAGE_ENDPOINT"),
		StorageRegion:          getenv("STORAGE_REGION", "us-east-1"),
		StorageBucket:          getenv("STORAGE_BUCKET", "tickets"),
		StorageAccessKey:       os.Getenv("STORAGE_ACCESS_KEY"),
		StorageSecretKey:       os.Getenv("STORAGE_SECRET_KEY"),
		StorageUsePathStyle:    getenv("STORAGE_USE_PATH_STYLE", "true") == "true",
		PublicBaseURL:          getenv("PUBLIC_BASE_URL", "http://localhost:3000"),
	}

	// Token signing key — accept base64 (preferred) or raw fallback. In dev,
	// a fixed dev-only key is used if the env is unset.
	rawKey := os.Getenv("TOKEN_SIGNING_KEY")
	if rawKey == "" {
		if cfg.Env != "dev" {
			return nil, fmt.Errorf("TOKEN_SIGNING_KEY is required outside dev")
		}
		rawKey = "ZGV2LW9ubHkta2V5LWRvbnQtdXNlLWluLXByb2QtMzItYnl0ZXM=" // base64 of "dev-only-key-dont-use-in-prod-32-bytes"
	}
	keyBytes, err := base64.StdEncoding.DecodeString(rawKey)
	if err != nil || len(keyBytes) < 16 {
		// Fall back to treating the raw value as the key (e.g. user pasted random bytes directly).
		keyBytes = []byte(rawKey)
	}
	if len(keyBytes) < 16 {
		return nil, fmt.Errorf("TOKEN_SIGNING_KEY must be at least 16 bytes (got %d)", len(keyBytes))
	}
	cfg.TokenSigningKey = keyBytes

	level, err := parseLevel(getenv("LOG_LEVEL", "info"))
	if err != nil {
		return nil, err
	}
	cfg.LogLevel = level

	ttl, err := time.ParseDuration(getenv("SESSION_TTL", "168h")) // 7 days
	if err != nil {
		return nil, fmt.Errorf("SESSION_TTL: %w", err)
	}
	cfg.SessionTTL = ttl

	if cfg.DatabaseURL == "" && cfg.Env != "dev" {
		return nil, fmt.Errorf("DATABASE_URL is required outside dev")
	}

	return cfg, nil
}

func (c *Config) IsProd() bool { return c.Env == "prod" }

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func parseLevel(s string) (slog.Level, error) {
	switch strings.ToLower(s) {
	case "debug":
		return slog.LevelDebug, nil
	case "info":
		return slog.LevelInfo, nil
	case "warn", "warning":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return 0, fmt.Errorf("invalid LOG_LEVEL %q", s)
	}
}
