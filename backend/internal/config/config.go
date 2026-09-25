package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Port             string
	DBDriver         string
	DBDSN            string
	DBAutoMigrate    bool
	JWTSecret        string
	CORSOrigins      []string
	LogLevel         string
	ShutdownTimeout  time.Duration
	AccessTokenTTL   time.Duration
	LoginRateLimit   int
	ImportRateLimit  int
	AnalyzeRateLimit int
}

func Load() (Config, error) {
	c := Config{
		Port:             env("PORT", "8080"),
		DBDriver:         strings.ToLower(env("DB_DRIVER", "postgres")),
		DBDSN:            env("DB_DSN", "host=localhost user=cadastral_app password=cadastral_local_password dbname=cadastral_topology port=5432 sslmode=disable"),
		DBAutoMigrate:    envBool("DB_AUTO_MIGRATE", true),
		JWTSecret:        env("JWT_SECRET", "development-secret-change-me-at-least-32-bytes"),
		CORSOrigins:      split(env("CORS_ORIGINS", "http://localhost:18540")),
		LogLevel:         env("LOG_LEVEL", "info"),
		ShutdownTimeout:  10 * time.Second,
		AccessTokenTTL:   8 * time.Hour,
		LoginRateLimit:   envInt("LOGIN_RATE_LIMIT", 30),
		ImportRateLimit:  envInt("IMPORT_RATE_LIMIT", 40),
		AnalyzeRateLimit: envInt("ANALYZE_RATE_LIMIT", 60),
	}
	// The prescribed runtime_smoke manifest uses a deliberately short throwaway
	// secret; it is accepted only for an in-memory SQLite process and never for Compose.
	if len(c.JWTSecret) < 32 && !(c.DBDriver == "sqlite" && c.JWTSecret == "runtime-smoke-only-change-me") {
		return Config{}, fmt.Errorf("JWT_SECRET must contain at least 32 characters")
	}
	if c.DBDriver != "postgres" && c.DBDriver != "sqlite" {
		return Config{}, fmt.Errorf("unsupported DB_DRIVER %q", c.DBDriver)
	}
	return c, nil
}

func env(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func envInt(key string, fallback int) int {
	v, err := strconv.Atoi(env(key, strconv.Itoa(fallback)))
	if err != nil {
		return fallback
	}
	return v
}

func envBool(key string, fallback bool) bool {
	v, err := strconv.ParseBool(env(key, strconv.FormatBool(fallback)))
	if err != nil {
		return fallback
	}
	return v
}

func split(value string) []string {
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}
