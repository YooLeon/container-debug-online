package docker

import (
	"context"
	"fmt"
	"strings"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/client"
)

type Monitor struct {
	client *client.Client
	ctx    context.Context
}

type ContainerInfo struct {
	ID      string            `json:"id"`
	Name    string            `json:"name"`
	Status  string            `json:"status"`
	Labels  map[string]string `json:"labels"`
	Service string            `json:"service"`
}

// NewMonitor 创建新的 Docker 监控器
func NewMonitor() (*Monitor, error) {
	ctx := context.Background()
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, fmt.Errorf("failed to create docker client: %v", err)
	}

	return &Monitor{
		client: cli,
		ctx:    ctx,
	}, nil
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

// Close 关闭 Docker 客户端连接
func (m *Monitor) Close() error {
	if m.client != nil {
		return m.client.Close()
	}
	return nil
}
