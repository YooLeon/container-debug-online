package docker

import (
	"sync"
	"time"

	"github.com/docker/docker/api/types"
)

// ServiceStatus 表示服务状态
type ServiceStatus struct {
	Name        string          `json:"name"`
	ContainerID string          `json:"container_id"` // 容器ID
	PortStatus  map[string]bool `json:"port_status"`  // 端口状态
	Healthy     bool            `json:"healthy"`      // 服务整体健康状态
	LastCheck   time.Time       `json:"last_check"`
}

// ContainerStatus 表示容器详细状态
type ContainerStatus struct {
	Info         ContainerInfo       `json:"info"`
	Inspect      types.ContainerJSON `json:"inspect"`
	PortsHealthy map[string]bool     `json:"ports_healthy"`
	LastCheck    time.Time           `json:"last_check"`
}

// MonitorStatus 存储监控状态
type MonitorStatus struct {
	sync.RWMutex
	Containers map[string]*ContainerStatus `json:"containers"` // key: containerID
	Services   map[string]*ServiceStatus   `json:"services"`   // key: serviceName
	LastUpdate time.Time                   `json:"last_update"`
}
