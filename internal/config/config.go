package config

import (
	"flag"
	"time"
)

type Config struct {
	ServerPort      int
	ServerHost      string
	ComposePath     string
	MonitorInterval time.Duration
	Password        string
}

func LoadConfig() *Config {
	serverPort := flag.Int("port", 14264, "Server port")
	serverHost := flag.String("host", "0.0.0.0", "Server host")
	composePath := flag.String("compose", "", "Path to docker-compose.yml")
	monitorInterval := flag.Duration("interval", 5*time.Second, "Monitor interval")
	password := flag.String("password", "", "Authentication password")

	flag.Parse()

	return &Config{
		ServerPort:      *serverPort,
		ServerHost:      *serverHost,
		ComposePath:     *composePath,
		MonitorInterval: *monitorInterval,
		Password:        *password,
	}
}
