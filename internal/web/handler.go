package web

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"path/filepath"
	"strings"
	"sync"

	"docker-monitor/internal/docker"

	"github.com/docker/docker/api/types"

	"bufio"

	"github.com/docker/docker/client"
	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
	"go.uber.org/zap"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

type Handler struct {
	monitor      *docker.Monitor
	mu           sync.Mutex
	conns        map[*websocket.Conn]bool
	dockerClient *client.Client
	logger       *zap.Logger
}

func NewHandler(monitor *docker.Monitor) *Handler {
	return &Handler{
		monitor:      monitor,
		conns:        make(map[*websocket.Conn]bool),
		dockerClient: monitor.Client(),
		logger:       zap.NewNop(),
	}
}

type Terminal struct {
	ws            *websocket.Conn
	dockerMonitor *docker.Monitor
	containerID   string
	done          chan struct{}
	execID        string
	stdinPipe     io.Writer
	sizeChan      chan struct{}
}

func NewTerminal(ws *websocket.Conn, dockerMonitor *docker.Monitor, containerID string) *Terminal {
	return &Terminal{
		ws:            ws,
		dockerMonitor: dockerMonitor,
		containerID:   containerID,
		done:          make(chan struct{}),
		sizeChan:      make(chan struct{}),
	}
}

// getValidContainerID 获取有效的容器ID
func (h *Handler) getValidContainerID(inputID string) (string, error) {
	// 首先检查是否为完整或短容器ID
	containers, err := h.monitor.ListContainers()
	if err != nil {
		return "", fmt.Errorf("failed to list containers: %v", err)
	}

	// 尝试多种方式匹配容器
	for _, container := range containers {
		// 1. 完整ID匹配
		if container.ID == inputID {
			return container.ID, nil
		}
		// 2. 短ID匹配（前12位）
		if strings.HasPrefix(container.ID, inputID) {
			return container.ID, nil
		}
		// 3. 服务名匹配
		if container.Service == inputID {
			return container.ID, nil
		}
		// 4. 容器名称匹配（去除开头的/）
		containerName := strings.TrimPrefix(container.Name, "/")
		if containerName == inputID {
			return container.ID, nil
		}
	}

	return "", fmt.Errorf("no valid container found for input: %s", inputID)
}

// Message 定义消息结构
type Message struct {
	Type string `json:"type"`
	Cols uint   `json:"cols"`
	Rows uint   `json:"rows"`
}

// handleMessage 处理WebSocket消息
func (t *Terminal) handleMessage(message []byte) error {
	// 尝试解析为JSON消息
	var msg Message
	if err := json.Unmarshal(message, &msg); err == nil && msg.Type == "resize" {
		// 是resize消息，处理终端大小调整
		if err := t.dockerMonitor.ResizeExecTTY(t.execID, msg.Rows, msg.Cols); err != nil {
			return fmt.Errorf("failed to resize terminal: %v", err)
		}
		return nil
	}

	// 不是JSON消息或不是resize消息，直接写入到容器
	_, err := t.stdinPipe.Write(message)
	if err != nil {
		return fmt.Errorf("failed to write to container: %v", err)
	}
	return nil
}

func (t *Terminal) Start() {
	defer func() {
		t.ws.Close()
		close(t.done)
	}()

	// 创建执行配置
	execConfig := types.ExecConfig{
		AttachStdin:  true,
		AttachStdout: true,
		AttachStderr: true,
		Tty:          true,
		Cmd:          []string{"/bin/bash"},
		Env: []string{
			"TERM=xterm-256color",
		},
	}

	// 创建执行实例
	exec, err := t.dockerMonitor.Client().ContainerExecCreate(t.dockerMonitor.Context(), t.containerID, execConfig)
	if err != nil {
		log.Printf("Error creating exec: %v", err)
		t.writeError(fmt.Sprintf("Failed to create exec: %v", err))
		return
	}
	t.execID = exec.ID

	// 附加到执行实例
	resp, err := t.dockerMonitor.Client().ContainerExecAttach(t.dockerMonitor.Context(), exec.ID, types.ExecStartCheck{
		Detach: false,
		Tty:    true,
	})
	if err != nil {
		log.Printf("Error attaching to exec: %v", err)
		t.writeError(fmt.Sprintf("Failed to attach to exec: %v", err))
		return
	}
	defer resp.Close()

	t.stdinPipe = resp.Conn

	// 建错误通道
	errChan := make(chan error, 2)

	// 处理输入的消息
	go func() {
		for {
			select {
			case <-t.done:
				return
			default:
				_, message, err := t.ws.ReadMessage()
				if err != nil {
					errChan <- fmt.Errorf("error reading from websocket: %v", err)
					return
				}

				// 检查是否是resize消息
				if len(message) > 0 && message[0] == '{' {
					var msg struct {
						Type string `json:"type"`
						Cols uint   `json:"cols"`
						Rows uint   `json:"rows"`
					}
					if err := json.Unmarshal(message, &msg); err == nil && msg.Type == "resize" {
						if err := t.dockerMonitor.ResizeExecTTY(t.execID, msg.Rows, msg.Cols); err != nil {
							log.Printf("Error resizing terminal: %v", err)
						}
						continue
					}
				}

				// 普通输入消息，直接写入到容器
				_, err = t.stdinPipe.Write(message)
				if err != nil {
					errChan <- fmt.Errorf("error writing to container: %v", err)
					return
				}
			}
		}
	}()

	// 处理输出
	go func() {
		buffer := make([]byte, 1024)
		for {
			select {
			case <-t.done:
				return
			default:
				n, err := resp.Reader.Read(buffer)
				if err != nil {
					if err != io.EOF {
						errChan <- fmt.Errorf("error reading from container: %v", err)
					}
					return
				}

				if n > 0 {
					// 直接发送到WebSocket，不做额外处理
					err = t.ws.WriteMessage(websocket.TextMessage, buffer[:n])
					if err != nil {
						errChan <- fmt.Errorf("error writing to websocket: %v", err)
						return
					}
				}
			}
		}
	}()

	// 等待错误或终止信号
	select {
	case err := <-errChan:
		log.Printf("Terminal error: %v", err)
		t.writeError(fmt.Sprintf("Terminal error: %v", err))
	case <-t.done:
	}
}

// writeError 向 WebSocket 写入错误消息
func (t *Terminal) writeError(message string) {
	log.Printf("Terminal error: %s", message)
	err := t.ws.WriteMessage(websocket.TextMessage, []byte("\r\n\x1b[31m"+message+"\x1b[0m\r\n"))
	if err != nil {
		log.Printf("Error writing error message to websocket: %v", err)
	}
}

func (h *Handler) TerminalHandler(w http.ResponseWriter, r *http.Request) {
	containerID := r.URL.Query().Get("container")
	if containerID == "" {
		http.Error(w, "Container ID is required", http.StatusBadRequest)
		return
	}

	log.Printf("Received terminal connection request for container: %s", containerID)

	// 升到 WebSocket 连接
	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("Error upgrading to websocket: %v", err)
		return
	}

	// 创建终端会话
	terminal := NewTerminal(ws, h.monitor, containerID)

	// 启动终端会话
	go terminal.Start()
}

func (h *Handler) ContainersHandler(w http.ResponseWriter, r *http.Request) {
	containers, err := h.monitor.ListContainers()
	if err != nil {
		log.Printf("Error listing containers: %v", err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{
			"error": fmt.Sprintf("Failed to list containers: %v", err),
		})
		return
	}

	// 确保即使没有容器也返回空数组而不是 null
	if containers == nil {
		containers = []docker.ContainerInfo{}
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(containers); err != nil {
		log.Printf("Error encoding container list: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{
			"error": fmt.Sprintf("Failed to encode container list: %v", err),
		})
	}
}

func (h *Handler) getContainer(ctx context.Context, composePath string, containerID string) (*types.Container, error) {
	// 获取完整的绝对路径
	absComposePath, err := filepath.Abs(composePath)
	if err != nil {
		return nil, fmt.Errorf("failed to get absolute path: %w", err)
	}

	h.logger.Debug("Looking for container",
		zap.String("absComposePath", absComposePath),
		zap.String("containerID", containerID))

	containers, err := h.dockerClient.ContainerList(ctx, types.ContainerListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list containers: %w", err)
	}

	for _, container := range containers {
		inspect, err := h.dockerClient.ContainerInspect(ctx, container.ID)
		if err != nil {
			continue
		}

		labels := inspect.Config.Labels
		workDir := labels["com.docker.compose.project.working_dir"]
		configFile := labels["com.docker.compose.project.config_files"]
		projectName := labels["com.docker.compose.project"]

		// 打印调试信息
		h.logger.Debug("Checking container",
			zap.String("containerID", container.ID),
			zap.String("workDir", workDir),
			zap.String("configFile", configFile),
			zap.String("projectName", projectName),
			zap.Any("labels", labels))

		// 构建容器的 compose 文件完整路径
		containerComposePath := filepath.Join(workDir, configFile)
		absContainerComposePath, err := filepath.Abs(containerComposePath)
		if err != nil {
			continue
		}

		h.logger.Debug("Comparing paths",
			zap.String("absContainerComposePath", absContainerComposePath),
			zap.String("absComposePath", absComposePath))

		// 严格匹配完整路径和容器ID
		if absContainerComposePath == absComposePath && container.ID == containerID {
			h.logger.Debug("Found matching container",
				zap.String("containerID", container.ID))
			return &container, nil
		}
	}

	return nil, fmt.Errorf("container not found")
}

// ContainerLogsHandler 处理容器日志的 WebSocket 连接
func (h *Handler) ContainerLogsHandler(w http.ResponseWriter, r *http.Request) {
	// 升级 HTTP 连接为 WebSocket
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("Failed to upgrade logs connection: %v", err)
		return
	}
	defer conn.Close()

	// 获取容器ID
	containerID := r.URL.Query().Get("container")
	if containerID == "" {
		conn.WriteMessage(websocket.TextMessage, []byte("Error: Missing container ID"))
		return
	}

	// 设置日志选项
	options := types.ContainerLogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Follow:     true,
		Timestamps: true,
		Tail:       "100",
		// 添加 TTY 选项，处理终端大小问题
		Details: false,
	}

	// 获取容器信息，检查是否使用了 TTY
	inspect, err := h.dockerClient.ContainerInspect(r.Context(), containerID)
	if err != nil {
		log.Printf("Error inspecting container: %v", err)
		return
	}

	// 获取容器日志流
	ctx := r.Context()
	logReader, err := h.dockerClient.ContainerLogs(ctx, containerID, options)
	if err != nil {
		conn.WriteMessage(websocket.TextMessage, []byte(fmt.Sprintf("Error getting logs: %v", err)))
		return
	}
	defer logReader.Close()

	// 根据容器是否使用 TTY 选择不同的处理方式
	if inspect.Config.Tty {
		// 容器使用 TTY，直接读取日志
		reader := bufio.NewReader(logReader)
		for {
			select {
			case <-ctx.Done():
				return
			default:
				line, err := reader.ReadString('\n')
				if err != nil {
					if err != io.EOF {
						log.Printf("Error reading log line: %v", err)
					}
					return
				}
				if err := conn.WriteMessage(websocket.TextMessage, []byte(line)); err != nil {
					log.Printf("Error sending log message: %v", err)
					return
				}
			}
		}
	} else {
		// 容器未使用 TTY，需要处理 stdout/stderr 流
		hdr := make([]byte, 8)
		for {
			select {
			case <-ctx.Done():
				return
			default:
				// 读取 Docker 日志头信息
				_, err := io.ReadFull(logReader, hdr)
				if err != nil {
					if err != io.EOF {
						log.Printf("Error reading log header: %v", err)
					}
					return
				}

				// 获取消息大小
				count := binary.BigEndian.Uint32(hdr[4:])
				if count == 0 {
					continue
				}

				// 读取实际的日志内容
				buf := make([]byte, count)
				_, err = io.ReadFull(logReader, buf)
				if err != nil {
					log.Printf("Error reading log message: %v", err)
					return
				}

				// 发送日志内容到客户端
				if err := conn.WriteMessage(websocket.TextMessage, buf); err != nil {
					log.Printf("Error sending log message: %v", err)
					return
				}
			}
		}
	}
}

type HealthResponse struct {
	Code    int               `json:"code"`
	Message string            `json:"message"`
	Details []ContainerStatus `json:"details,omitempty"`
}

type ContainerStatus struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	ID     string `json:"id"`
}

func (h *Handler) HealthCheckHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// 如果没有指定 compose 文件，直接返回成功
	if h.monitor.GetComposePath() == "" {
		json.NewEncoder(w).Encode(HealthResponse{
			Code:    0,
			Message: "No compose file specified",
		})
		return
	}

	// 使用 listComposeContainers 获取指定 compose 文件的容器
	containers, err := h.monitor.ListComposeContainers()
	if err != nil {
		json.NewEncoder(w).Encode(HealthResponse{
			Code:    1,
			Message: fmt.Sprintf("Error checking compose containers: %v", err),
		})
		return
	}

	// 如果没有找到任何容器
	if len(containers) == 0 {
		json.NewEncoder(w).Encode(HealthResponse{
			Code:    1,
			Message: "No containers found for the specified compose file",
		})
		return
	}

	var details []ContainerStatus
	allRunning := true

	for _, container := range containers {
		details = append(details, ContainerStatus{
			Name:   container.Name,
			Status: container.Status,
			ID:     container.ID,
		})
		if container.Status != "running" {
			allRunning = false
		}
	}

	code := 1
	message := "Some containers are not running"
	if allRunning {
		code = 0
		message = "All containers running"
	}

	response := HealthResponse{
		Code:    code,
		Message: message,
		Details: details,
	}

	json.NewEncoder(w).Encode(response)
}
func (h *Handler) RegisterRoutes(r *mux.Router) {
	// ... 其他路由 ...

	// WebSocket 路由
	r.HandleFunc("/container/logs", h.ContainerLogsHandler)
}
