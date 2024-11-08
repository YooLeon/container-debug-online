package web

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"

	"docker-monitor/internal/docker"

	"github.com/docker/docker/api/types"
	"github.com/gorilla/websocket"
	"golang.org/x/net/context"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

type Handler struct {
	monitor *docker.Monitor
	mu      sync.Mutex
	conns   map[*websocket.Conn]bool
}

func NewHandler(monitor *docker.Monitor) *Handler {
	return &Handler{
		monitor: monitor,
		conns:   make(map[*websocket.Conn]bool),
	}
}

type Terminal struct {
	ws            *websocket.Conn
	dockerMonitor *docker.Monitor
	containerID   string
	done          chan struct{}
}

func NewTerminal(ws *websocket.Conn, monitor *docker.Monitor, containerID string) *Terminal {
	return &Terminal{
		ws:            ws,
		dockerMonitor: monitor,
		containerID:   containerID,
		done:          make(chan struct{}),
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
	}

	ctx := context.Background()

	// 创建执行实例
	exec, err := t.dockerMonitor.Client().ContainerExecCreate(ctx, t.containerID, execConfig)
	if err != nil {
		log.Printf("Error creating exec: %v", err)
		t.writeError(fmt.Sprintf("Failed to create exec: %v", err))
		return
	}

	// 附加到执行实例
	resp, err := t.dockerMonitor.Client().ContainerExecAttach(ctx, exec.ID, types.ExecStartCheck{
		Detach: false,
		Tty:    true,
	})
	if err != nil {
		log.Printf("Error attaching to exec: %v", err)
		t.writeError(fmt.Sprintf("Failed to attach to exec: %v", err))
		return
	}
	defer resp.Close()

	// 创建错误通道
	errChan := make(chan error, 2)

	// 处理输入
	go func() {
		defer func() {
			if r := recover(); r != nil {
				errChan <- fmt.Errorf("panic in input handler: %v", r)
			}
		}()

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

				_, err = resp.Conn.Write(message)
				if err != nil {
					errChan <- fmt.Errorf("error writing to container: %v", err)
					return
				}
			}
		}
	}()

	// 处理输出
	go func() {
		defer func() {
			if r := recover(); r != nil {
				errChan <- fmt.Errorf("panic in output handler: %v", r)
			}
		}()

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

	// 升级到 WebSocket 连接
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
		http.Error(w, "Failed to list containers", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(containers); err != nil {
		log.Printf("Error encoding container list: %v", err)
		http.Error(w, "Failed to encode container list", http.StatusInternalServerError)
	}
}
