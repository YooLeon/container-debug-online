package config

import (
	"flag"
	"time"
)

type Config struct {
	ComposeFile     string
	ComposeFilePath string
	MonitorInterval time.Duration
	ServerPort      string
}

func LoadConfig() *Config {
	config := &Config{}

	flag.StringVar(&config.ComposeFile, "compose-file", "docker-compose.yml", "Docker compose file name")
	flag.StringVar(&config.ComposeFilePath, "compose-path", ".", "Path to docker compose file")
	flag.DurationVar(&config.MonitorInterval, "monitor-interval", 30*time.Second, "Container health check interval")
	flag.StringVar(&config.ServerPort, "port", ":8080", "Server listening port")

	flag.Parse()

	return config
}
