package config

import (
	"flag"
	"time"
)

type Config struct {
	ServerPort      string
	ComposePath     string
	MonitorInterval time.Duration
	Password        string
}

func LoadConfig() *Config {
	serverPort := flag.String("port", ":8080", "Server port")
	composePath := flag.String("compose", "", "Path to docker-compose.yml")
	monitorInterval := flag.Duration("interval", 5*time.Second, "Monitor interval")
	password := flag.String("password", "", "Authentication password")

	flag.Parse()

	return &Config{
		ServerPort:      *serverPort,
		ComposePath:     *composePath,
		MonitorInterval: *monitorInterval,
		Password:        *password,
	}
}
