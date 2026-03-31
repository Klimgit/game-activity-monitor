package config

import (
	"os"
	"time"
)

type Config struct {
	Server            ServerConfig
	Database          DatabaseConfig
	Auth              AuthConfig
	DatasetAutomation DatasetAutomationConfig
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

type DatasetAutomationConfig struct {
	Enabled    bool
	Interval   time.Duration
	OutputDir  string
	RunOnStart bool
}

func Load() *Config {
	iv, _ := time.ParseDuration(getEnv("DATASET_AUTOMATION_INTERVAL", "24h"))
	if iv < time.Minute {
		iv = 24 * time.Hour
	}
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
		DatasetAutomation: DatasetAutomationConfig{
			Enabled:    getEnv("DATASET_AUTOMATION_ENABLED", "") == "true",
			Interval:   iv,
			OutputDir:  getEnv("DATASET_AUTOMATION_OUTPUT_DIR", ""),
			RunOnStart: getEnv("DATASET_AUTOMATION_RUN_ON_START", "") == "true",
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
