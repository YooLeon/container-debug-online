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

// HealthStatus 表示容器健康状态
type HealthStatus struct {
	Status        string    `json:"status"`       // 健康检查状态：none, starting, healthy, unhealthy
	Log           []string  `json:"log"`          // 最近的健康检查日志
	FailingStreak int       `json:"failing_streak"` // 连续失败次数
	LastCheck     time.Time `json:"last_check"`    // 最后检查时间
}

// ContainerStatus 表示容器详细状态
type ContainerStatus struct {
	Info         ContainerInfo       `json:"info"`
	Inspect      types.ContainerJSON `json:"inspect"`
	PortsHealthy map[string]bool     `json:"ports_healthy"`
	LastCheck    time.Time          `json:"last_check"`
	Health       *HealthStatus      `json:"health"`      // 添加健康状态
	ExitCode     int               `json:"exit_code"`   // 添加退出码
}

// MonitorStatus 存储监控状态
type MonitorStatus struct {
	sync.RWMutex
	Containers map[string]*ContainerStatus `json:"containers"` // key: containerID
	Services   map[string]*ServiceStatus   `json:"services"`   // key: serviceName
	LastUpdate time.Time                   `json:"last_update"`
}
