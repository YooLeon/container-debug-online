package config

import (
	"flag"
	"time"
)

type Config struct {
	ServerPort      string
	MonitorInterval time.Duration
	ComposePath     string
}

func LoadConfig() *Config {
	cfg := &Config{
		ServerPort:      ":8080",
		MonitorInterval: 30 * time.Second,
	}

	flag.StringVar(&cfg.ComposePath, "compose", "", "Path to docker-compose.yml file")
	flag.Parse()

	return cfg
}
