package docker

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/client"
	"go.uber.org/zap"
)

type Monitor struct {
	client      *client.Client
	ctx         context.Context
	cancel      context.CancelFunc
	logger      *zap.Logger
	interval    time.Duration
	composePath string
}

type ContainerInfo struct {
	ID      string            `json:"id"`
	Name    string            `json:"name"`
	Status  string            `json:"status"`
	Labels  map[string]string `json:"labels"`
	Service string            `json:"service"`
}

// NewMonitor 创建新的 Docker 监控器
func NewMonitor(
	client *client.Client,
	logger *zap.Logger,
	interval time.Duration,
	composePath string,
) *Monitor {
	ctx, cancel := context.WithCancel(context.Background())

	return &Monitor{
		client:      client,
		ctx:         ctx,
		cancel:      cancel,
		logger:      logger,
		interval:    interval,
		composePath: composePath,
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

// CheckContainerStatus 检查容器状态
// 参数可以是容器ID（完整或短ID）或服务名
func (m *Monitor) CheckContainerStatus(idOrName string) (string, error) {
	containers, err := m.client.ContainerList(m.ctx, types.ContainerListOptions{All: true})
	if err != nil {
		return "", fmt.Errorf("failed to list containers: %v", err)
	}

	// 尝试多种方式匹配容器
	for _, container := range containers {
		// 1. 完整ID匹配
		if container.ID == idOrName {
			return container.State, nil
		}
		// 2. 短ID匹配
		if strings.HasPrefix(container.ID, idOrName) {
			return container.State, nil
		}
		// 3. 名称匹配
		for _, name := range container.Names {
			// 移除开头的 "/"
			cleanName := strings.TrimPrefix(name, "/")
			if cleanName == idOrName {
				return container.State, nil
			}
		}
		// 4. 服务名匹配（如果有）
		if serviceName, ok := container.Labels["com.docker.compose.service"]; ok && serviceName == idOrName {
			return container.State, nil
		}
	}

	return "", fmt.Errorf("container not found: %s", idOrName)
}

// GetContainerIDByService 通过服务名或容器ID获取完整容器ID
func (m *Monitor) GetContainerIDByService(idOrName string) (string, error) {
	containers, err := m.client.ContainerList(m.ctx, types.ContainerListOptions{})
	if err != nil {
		return "", fmt.Errorf("failed to list containers: %v", err)
	}

	// 尝试多种方式匹配容器
	for _, container := range containers {
		// 1. 完整ID匹配
		if container.ID == idOrName {
			return container.ID, nil
		}
		// 2. 短ID匹配
		if strings.HasPrefix(container.ID, idOrName) {
			return container.ID, nil
		}
		// 3. 名称匹配
		for _, name := range container.Names {
			cleanName := strings.TrimPrefix(name, "/")
			if cleanName == idOrName {
				return container.ID, nil
			}
		}
		// 4. 服务名匹配
		if serviceName, ok := container.Labels["com.docker.compose.service"]; ok && serviceName == idOrName {
			return container.ID, nil
		}
	}

	return "", fmt.Errorf("container not found: %s", idOrName)
}

// ListContainers 返回所有容器信息
func (m *Monitor) ListContainers() ([]ContainerInfo, error) {
	if m.composePath != "" {
		return m.listComposeContainers()
	}

	return m.listAllContainers()
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

// ListContainersByCompose 返回指定 compose 文件启动的容器信息
func (m *Monitor) ListContainersByCompose(composePath string) ([]ContainerInfo, error) {
	containers, err := m.client.ContainerList(m.ctx, types.ContainerListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list containers: %v", err)
	}

	// 获取完整的绝对路径
	absComposePath, err := filepath.Abs(composePath)
	if err != nil {
		return nil, fmt.Errorf("failed to get absolute path: %v", err)
	}

	var containerInfos []ContainerInfo
	for _, container := range containers {
		// 获取容器的详细信息
		inspect, err := m.client.ContainerInspect(m.ctx, container.ID)
		if err != nil {
			continue
		}

		labels := inspect.Config.Labels
		workDir := labels["com.docker.compose.project.working_dir"]
		configFile := labels["com.docker.compose.project.config_files"]

		// 如果容器不是由 compose 启动的，跳过
		if workDir == "" || configFile == "" {
			continue
		}

		// 构建容器的 compose 文件完整路径
		containerComposePath := filepath.Join(workDir, configFile)
		absContainerComposePath, err := filepath.Abs(containerComposePath)
		if err != nil {
			continue
		}

		// 只有当容器是由指定的 compose 文件启动时才添加
		if absContainerComposePath == absComposePath {
			name := strings.TrimPrefix(container.Names[0], "/")
			serviceName := container.Labels["com.docker.compose.service"]
			if serviceName == "" {
				serviceName = name
			}

			containerInfos = append(containerInfos, ContainerInfo{
				ID:      container.ID[:12],
				Name:    name,
				Status:  container.State,
				Labels:  container.Labels,
				Service: serviceName,
			})
		}
	}

	return containerInfos, nil
}

func (m *Monitor) listComposeContainers() ([]ContainerInfo, error) {
	// 首先检查 composePath 是否存在
	if _, err := os.Stat(m.composePath); os.IsNotExist(err) {
		return nil, fmt.Errorf("compose file not found: %s", m.composePath)
	}

	containers, err := m.client.ContainerList(m.ctx, types.ContainerListOptions{All: true}) // 添加 All: true 以显示所有容器
	if err != nil {
		return nil, fmt.Errorf("failed to list containers: %v", err)
	}

	absComposePath, err := filepath.Abs(m.composePath)
	if err != nil {
		return nil, fmt.Errorf("failed to get absolute path: %v", err)
	}

	m.logger.Debug("Looking for containers with compose file",
		zap.String("composePath", absComposePath))

	var containerInfos []ContainerInfo
	for _, container := range containers {
		inspect, err := m.client.ContainerInspect(m.ctx, container.ID)
		if err != nil {
			m.logger.Warn("Failed to inspect container",
				zap.String("containerID", container.ID),
				zap.Error(err))
			continue
		}

		labels := inspect.Config.Labels
		workDir := labels["com.docker.compose.project.working_dir"]
		configFile := labels["com.docker.compose.project.config_files"]

		if workDir == "" || configFile == "" {
			continue
		}

		// 处理配置文件路径
		var containerComposePath string
		if filepath.IsAbs(configFile) {
			// 如果配置文件是绝对路径，直接使用
			containerComposePath = configFile
		} else {
			// 如果是相对路径，则与 workDir 拼接
			containerComposePath = filepath.Join(workDir, configFile)
		}

		absContainerComposePath, err := filepath.Abs(containerComposePath)
		if err != nil {
			m.logger.Warn("Failed to get absolute path for container compose file",
				zap.String("containerID", container.ID),
				zap.String("composePath", containerComposePath),
				zap.Error(err))
			continue
		}

		m.logger.Debug("Comparing compose paths",
			zap.String("container", absContainerComposePath),
			zap.String("target", absComposePath))

		if absContainerComposePath == absComposePath {
			name := strings.TrimPrefix(container.Names[0], "/")
			serviceName := container.Labels["com.docker.compose.service"]
			if serviceName == "" {
				serviceName = name
			}

			containerInfos = append(containerInfos, ContainerInfo{
				ID:      container.ID[:12],
				Name:    name,
				Status:  container.State,
				Labels:  container.Labels,
				Service: serviceName,
			})
		}
	}

	m.logger.Debug("Found containers",
		zap.Int("count", len(containerInfos)),
		zap.String("composePath", absComposePath))

	return containerInfos, nil
}

func (m *Monitor) listAllContainers() ([]ContainerInfo, error) {
	containers, err := m.client.ContainerList(m.ctx, types.ContainerListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list containers: %v", err)
	}

	var containerInfos []ContainerInfo
	for _, container := range containers {
		// 获取容器名称（移除开头的 "/"）
		name := strings.TrimPrefix(container.Names[0], "/")

		// 获取服务名（如果存在）
		serviceName := container.Labels["com.docker.compose.service"]
		if serviceName == "" {
			serviceName = name // 如果没有服务名，使用容器名
		}

		containerInfos = append(containerInfos, ContainerInfo{
			ID:      container.ID[:12], // 使用短ID
			Name:    name,
			Status:  container.State,
			Labels:  container.Labels,
			Service: serviceName,
		})
	}

	return containerInfos, nil
}
