package docker

import (
	"context"
	"fmt"
	"net"
	"path/filepath"
	"strings"
	"time"

	"github.com/YooLeon/container-debug-online/internal/config"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/client"
	"go.uber.org/zap"
)

type Monitor struct {
	client        *client.Client
	ctx           context.Context
	cancel        context.CancelFunc
	logger        *zap.Logger
	interval      time.Duration
	composeConfig *config.ComposeConfig
	status        *MonitorStatus
}

type ContainerInfo struct {
	ID      string              `json:"id"`
	Name    string              `json:"name"`
	Status  string              `json:"status"`
	Labels  map[string]string   `json:"labels"`
	Service string              `json:"service"`
	Inspect types.ContainerJSON `json:"inspect"`
}

// NewMonitor 创建新的 Docker 监控器
func NewMonitor(
	client *client.Client,
	logger *zap.Logger,
	interval time.Duration,
	composeConfig *config.ComposeConfig,
) *Monitor {
	ctx, cancel := context.WithCancel(context.Background())

	return &Monitor{
		client:        client,
		ctx:           ctx,
		cancel:        cancel,
		logger:        logger,
		interval:      interval,
		composeConfig: composeConfig,
		status: &MonitorStatus{
			Containers: make(map[string]*ContainerStatus),
			Services:   make(map[string]*ServiceStatus),
		},
	}
}

// Client 返回 Docker 客户端
func (m *Monitor) Client() *client.Client {
	return m.client
}

// Context 返回上下文
func (m *Monitor) Context() context.Context {
	return m.ctx
}

// ResizeExecTTY 调整终端大小
func (m *Monitor) ResizeExecTTY(execID string, height, width uint) error {
	return m.client.ContainerExecResize(m.ctx, execID, types.ResizeOptions{
		Height: height,
		Width:  width,
	})
}

// Close 关闭 Docker 客户端连接
func (m *Monitor) Close() error {
	if m.cancel != nil {
		m.cancel()
	}
	if m.client != nil {
		return m.client.Close()
	}
	return nil
}

// GetComposePath 返回当前的 compose 文件路径
func (m *Monitor) GetComposePath() string {
	return m.composeConfig.Path
}

// 检查端口是否正常监听
func (m *Monitor) checkPortHealth(containerID string, port string) bool {
	inspect, err := m.client.ContainerInspect(m.ctx, containerID)
	if err != nil {
		return false
	}

	// 获取容器IP
	containerIP := inspect.NetworkSettings.IPAddress
	if containerIP == "" {
		// 如果没有默认网络IP，尝试其他网络
		for _, network := range inspect.NetworkSettings.Networks {
			if network.IPAddress != "" {
				containerIP = network.IPAddress
				break
			}
		}
	}

	if containerIP == "" {
		return false
	}

	// 尝试连接端口
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("%s:%s", containerIP, port), 2*time.Second)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

// UpdateStatus 更新监控状态
func (m *Monitor) UpdateStatus() error {
	m.status.Lock()
	defer m.status.Unlock()

	// 清理旧状态
	newContainers := make(map[string]*ContainerStatus)
	newServices := make(map[string]*ServiceStatus)

	// 获取所有容器
	containers, err := m.client.ContainerList(m.ctx, types.ContainerListOptions{All: true})
	if err != nil {
		return err
	}

	// 如果指定了compose文件，获取其绝对路径
	var targetComposePath string
	if m.composeConfig.Path != "" {
		targetComposePath, err = filepath.Abs(m.composeConfig.Path)
		if err != nil {
			return fmt.Errorf("failed to get absolute path for compose file: %v", err)
		}
	}

	for _, container := range containers {
		// 如果指定了compose文件，检查容器是否属于该compose项目
		if m.composeConfig.Path != "" {
			configFile := container.Labels["com.docker.compose.project.config_files"]
			workDir := container.Labels["com.docker.compose.project.working_dir"]
			serviceName := container.Labels["com.docker.compose.service"]

			if configFile == "" || workDir == "" || serviceName == "" {
				continue // 跳过非compose容器
			}

			containerComposePath := configFile
			if !filepath.IsAbs(configFile) {
				containerComposePath = filepath.Join(workDir, configFile)
			}

			absContainerComposePath, err := filepath.Abs(containerComposePath)
			if err != nil {
				m.logger.Warn("Failed to get absolute path for container compose file",
					zap.String("containerID", container.ID),
					zap.Error(err))
				continue
			}

			if absContainerComposePath != targetComposePath {
				continue // 跳过不属于目标compose项目的容器
			}
			if _, exists := m.composeConfig.Services[serviceName]; !exists {
				continue // 跳过不属于目标compose项目的容器
			}
		}

		// 获取容器详细信息
		inspect, err := m.client.ContainerInspect(m.ctx, container.ID)
		if err != nil {
			m.logger.Warn("Failed to inspect container",
				zap.String("containerID", container.ID),
				zap.Error(err))
			continue
		}

		// 检查端口健康状态
		portsHealthy := make(map[string]bool)
		for port := range inspect.Config.ExposedPorts {
			portsHealthy[port.Port()] = m.checkPortHealth(container.ID, port.Port())
		}

		// 创建容器状态
		containerStatus := &ContainerStatus{
			Info: ContainerInfo{
				ID:      container.ID[:12],
				Name:    strings.TrimPrefix(container.Names[0], "/"),
				Status:  container.State,
				Labels:  container.Labels,
				Service: container.Labels["com.docker.compose.service"],
				Inspect: inspect,
			},
			PortsHealthy: portsHealthy,
			LastCheck:    time.Now(),
		}

		newContainers[container.ID] = containerStatus

		// 更新服务状态
		if serviceName := container.Labels["com.docker.compose.service"]; serviceName != "" {
			service, exists := newServices[serviceName]
			if !exists {
				service = &ServiceStatus{
					Name:        serviceName,
					ContainerID: container.ID,
					PortStatus:  make(map[string]bool),
					Healthy:     false,
					LastCheck:   time.Now(),
				}
				newServices[serviceName] = service
			}
			service.ContainerID = container.ID

			// 更新服务的端口状态
			for port, healthy := range portsHealthy {
				if existingHealth, ok := service.PortStatus[port]; !ok {
					service.PortStatus[port] = healthy
				} else {
					service.PortStatus[port] = existingHealth && healthy
				}
			}
		}
	}

	// 更新服务的健康状态
	for _, service := range newServices {
		service.Healthy = service.ContainerID != ""
		for _, healthy := range service.PortStatus {
			service.Healthy = service.Healthy && healthy
		}
	}

	m.status.Containers = newContainers
	m.status.Services = newServices
	m.status.LastUpdate = time.Now()

	m.logger.Debug("Status updated",
		zap.Int("containers", len(newContainers)),
		zap.Int("services", len(newServices)),
		zap.String("compose_path", m.composeConfig.Path))

	return nil
}

// GetAllStatus 获取所有状态
func (m *Monitor) GetAllStatus() *MonitorStatus {
	m.status.RLock()
	defer m.status.RUnlock()

	return m.status
}
