package config

import (
	"os"
	"time"
)

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

func mustEnv(key, defaultVal string) string {
	return getEnv(key, defaultVal)
}
