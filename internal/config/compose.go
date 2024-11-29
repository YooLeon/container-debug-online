package config

import (
	"fmt"
	"os"
	"sort"

	"gopkg.in/yaml.v2"
)

// ComposeConfig 表示 docker-compose 配置
type ComposeConfig struct {
	Version        string                   `yaml:"version"`
	Services       map[string]ServiceConfig `yaml:"services"`
	Path           string                   `yaml:"-"`
	SortedServices []string                 `yaml:"-"`
}

// ServiceConfig 表示服务配置
type ServiceConfig struct {
	Image          string            `yaml:"image"`
	Container_name string            `yaml:"container_name,omitempty"`
	Command        interface{}       `yaml:"command,omitempty"`
	Environment    map[string]string `yaml:"environment,omitempty"`
	Volumes        []string          `yaml:"volumes,omitempty"`
	Ports          []string          `yaml:"ports,omitempty"`
	Deploy         *DeployConfig     `yaml:"deploy,omitempty"`
}

// DeployConfig 表示部署配置
type DeployConfig struct {
	Resources ResourceConfig `yaml:"resources"`
}

// ResourceConfig 表示资源配置
type ResourceConfig struct {
	Reservations *ReservationConfig `yaml:"reservations,omitempty"`
	Limits       *LimitConfig       `yaml:"limits,omitempty"`
}

// ReservationConfig 表示资源预留配置
type ReservationConfig struct {
	Devices []DeviceConfig `yaml:"devices,omitempty"`
}

// LimitConfig 表示资源限制配置
type LimitConfig struct {
	Memory string `yaml:"memory,omitempty"`
	CPUs   string `yaml:"cpus,omitempty"`
}

// DeviceConfig 表示设备配置
type DeviceConfig struct {
	Driver       string   `yaml:"driver"`
	Count        int      `yaml:"count"`
	Capabilities []string `yaml:"capabilities"`
}

// LoadComposeConfig 加载 docker-compose 配置文件
func LoadComposeConfig(configPath string) (*ComposeConfig, error) {
	// 检查文件是否存在
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("compose file not found: %s", configPath)
	}

	// 读取配置文件
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("error reading compose file: %v", err)
	}

	// 解析 YAML
	config := &ComposeConfig{}
	if err := yaml.Unmarshal(data, config); err != nil {
		return nil, fmt.Errorf("error parsing compose file: %v", err)
	}

	// 验证配置
	if err := validateConfig(config); err != nil {
		return nil, fmt.Errorf("invalid compose configuration: %v", err)
	}
	config.Path = configPath
	config.SortedServices = sortServices(config.Services)

	return config, nil
}

// GetServiceCount 获取服务数量
func (c *ComposeConfig) GetServiceCount() int {
	return len(c.SortedServices)
}

func sortServices(services map[string]ServiceConfig) []string {
	serviceNames := make([]string, 0, len(services))
	for name := range services {
		serviceNames = append(serviceNames, name)
	}
	sort.Strings(serviceNames)
	return serviceNames
}

// validateConfig 验证配置是否有效
func validateConfig(config *ComposeConfig) error {
	if config.Services == nil {
		return fmt.Errorf("no services defined in compose file")
	}

	for name, service := range config.Services {
		if service.Image == "" {
			return fmt.Errorf("service '%s' has no image specified", name)
		}
	}

	return nil
}
