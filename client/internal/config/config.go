package config

import (
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// Config is the full client configuration.
type Config struct {
	Server     ServerConfig     `yaml:"server"`
	Auth       AuthConfig       `yaml:"auth"`
	Collectors CollectorsConfig `yaml:"collectors"`
	Offline    OfflineConfig    `yaml:"offline"`
}

type ServerConfig struct {
	URL            string        `yaml:"url"`
	AutoReconnect  bool          `yaml:"auto_reconnect"`
	ReconnectDelay time.Duration `yaml:"reconnect_interval"`
}

type AuthConfig struct {
	Email    string `yaml:"email"`
	Password string `yaml:"password"`
	// Token is set at runtime after login; it is never written to disk.
	Token string `yaml:"-"`
}

type CollectorsConfig struct {
	Intervals struct {
		SystemPolling     time.Duration `yaml:"system_polling"`
		AggregationWindow time.Duration `yaml:"aggregation_window"`
	} `yaml:"intervals"`
	Enabled []string `yaml:"enabled"`
}

type OfflineConfig struct {
	MaxQueueSize  int           `yaml:"max_queue_size"`
	FlushInterval time.Duration `yaml:"flush_interval"`
}

// Load reads config from path. If the file does not exist, Default() is returned.
func Load(path string) (*Config, error) {
	cfg := Default()

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return nil, err
	}

	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

// Default returns a Config populated with sensible defaults.
func Default() *Config {
	cfg := &Config{}

	cfg.Server.URL = "http://localhost:8000"
	cfg.Server.AutoReconnect = true
	cfg.Server.ReconnectDelay = 30 * time.Second

	cfg.Collectors.Intervals.SystemPolling = 2 * time.Second
	cfg.Collectors.Intervals.AggregationWindow = 10 * time.Second
	cfg.Collectors.Enabled = []string{"mouse", "keyboard", "system", "gpu"}

	cfg.Offline.MaxQueueSize = 50_000
	cfg.Offline.FlushInterval = 5 * time.Second

	return cfg
}
