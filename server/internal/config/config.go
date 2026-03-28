package config

import (
	"os"
	"time"
)

// Config holds all server configuration loaded from environment variables.
type Config struct {
	Server   ServerConfig
	Database DatabaseConfig
	Auth     AuthConfig
}

type ServerConfig struct {
	Port string
}

type DatabaseConfig struct {
	URL string
}

type AuthConfig struct {
	JWTSecret     string
	TokenDuration time.Duration
}

// Load reads configuration from environment variables, using defaults where absent.
func Load() *Config {
	return &Config{
		Server: ServerConfig{
			Port: getEnv("PORT", "8000"),
		},
		Database: DatabaseConfig{
			URL: mustEnv("DATABASE_URL", "postgres://postgres:postgres@localhost:5432/game_metrics?sslmode=disable"),
		},
		Auth: AuthConfig{
			JWTSecret:     mustEnv("JWT_SECRET", "change-me-in-production"),
			TokenDuration: 24 * time.Hour,
		},
	}
}

func getEnv(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}

// mustEnv returns the env value if set, otherwise falls back to defaultVal.
// The name "must" signals that production deployments are expected to override these.
func mustEnv(key, defaultVal string) string {
	return getEnv(key, defaultVal)
}
