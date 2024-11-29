package web

import (
	"bufio"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/YooLeon/container-debug-online/internal/docker"
	"github.com/docker/docker/api/types"
	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
	"go.uber.org/zap"
)

type Handler struct {
	monitor *docker.Monitor
	logger  *zap.Logger
}

type ContainerResponse struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Status      string            `json:"status"`
	Service     string            `json:"service"`
	PortsHealth map[string]bool   `json:"ports_health"`
	Healthy     bool              `json:"healthy"`
	Labels      map[string]string `json:"labels"`
}

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

func NewHandler(monitor *docker.Monitor) *Handler {
	return &Handler{
		monitor: monitor,
		logger:  zap.L(),
	}
}

func (h *Handler) ContainersHandler(w http.ResponseWriter, r *http.Request) {
	status := h.monitor.GetAllStatus()
	config := h.monitor.GetComposeConfig()

	var response []ContainerResponse
	// 按照 docker-compose 中的服务顺序添加容器
	for _, serviceName := range config.SortedServices {
		if serviceStatus, ok := status.Services[serviceName]; ok {
			// 直接使用 service 中的容器 ID 获取容器状态
			if containerStatus, exists := status.Containers[serviceStatus.ContainerID]; exists {
				healthy := true
				for _, portHealthy := range containerStatus.PortsHealthy {
					if !portHealthy {
						healthy = false
						break
					}
				}
				
				response = append(response, ContainerResponse{
					ID:          serviceStatus.ContainerID,
					Name:        containerStatus.Info.Name,
					Status:      containerStatus.Info.Status,
					Service:     serviceName,
					PortsHealth: containerStatus.PortsHealthy,
					Healthy:     healthy && serviceStatus.Healthy,
					Labels:      containerStatus.Info.Labels,
				})
			} else {
				// 服务存在但容器未找到
				response = append(response, ContainerResponse{
					ID:          "",
					Name:        fmt.Sprintf("%s (not running)", serviceName),
					Status:      "not found",
					Service:     serviceName,
					PortsHealth: make(map[string]bool),
					Healthy:     false,
					Labels:      make(map[string]string),
				})
			}
		} else {
			// 服务配置存在但服务状态未找到
			response = append(response, ContainerResponse{
				ID:          "",
				Name:        fmt.Sprintf("%s (not started)", serviceName),
				Status:      "not started",
				Service:     serviceName,
				PortsHealth: make(map[string]bool),
				Healthy:     false,
				Labels:      make(map[string]string),
			})
		}
	}



	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func (h *Handler) HealthCheckHandler(w http.ResponseWriter, r *http.Request) {
	status := h.monitor.GetAllStatus()

	// 检查所有服务和容器的健康状态
	allHealthy := true
	for _, service := range status.Services {
		if !service.Healthy {
			allHealthy = false
			break
		}
	}

	response := struct {
		Status    string `json:"status"`
		LastCheck string `json:"last_check"`
	}{
		Status:    "healthy",
		LastCheck: status.LastUpdate.Format("2006-01-02 15:04:05"),
	}

	if !allHealthy {
		response.Status = "unhealthy"
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// TerminalHandler 处理终端 WebSocket 连接
func (h *Handler) TerminalHandler(w http.ResponseWriter, r *http.Request) {
	containerID := r.URL.Query().Get("container")
	if containerID == "" {
		http.Error(w, "Missing container ID", http.StatusBadRequest)
		return
	}

	// 升级 HTTP 连接为 WebSocket
	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		h.logger.Error("Failed to upgrade connection", zap.Error(err))
		return
	}
	defer ws.Close()

	// 在容器中创建执行实例
	exec, err := h.monitor.Client().ContainerExecCreate(r.Context(), containerID, types.ExecConfig{
		AttachStdin:  true,
		AttachStdout: true,
		AttachStderr: true,
		Tty:          true,
		Cmd:          []string{"/bin/sh"},
		Env:          []string{"TERM=xterm-256color"},
	})
	if err != nil {
		h.logger.Error("Failed to create exec", zap.Error(err))
		ws.WriteMessage(websocket.TextMessage, []byte(fmt.Sprintf("Error: %v", err)))
		return
	}

	// 附加到执行实例
	resp, err := h.monitor.Client().ContainerExecAttach(r.Context(), exec.ID, types.ExecStartCheck{
		Tty: true,
	})
	if err != nil {
		h.logger.Error("Failed to attach to exec", zap.Error(err))
		ws.WriteMessage(websocket.TextMessage, []byte(fmt.Sprintf("Error: %v", err)))
		return
	}
	defer resp.Close()

	// 处理输入
	go func() {
		for {
			messageType, p, err := ws.ReadMessage()
			if err != nil {
				h.logger.Error("Failed to read message", zap.Error(err))
				return
			}

			if messageType == websocket.TextMessage {
				var msg struct {
					Type string `json:"type"`
					Cols uint   `json:"cols"`
					Rows uint   `json:"rows"`
					Data string `json:"data"`
				}

				if err := json.Unmarshal(p, &msg); err != nil {
					h.logger.Error("Failed to unmarshal message", zap.Error(err))
					continue
				}

				switch msg.Type {
				case "resize":
					if err := h.monitor.ResizeExecTTY(exec.ID, msg.Rows, msg.Cols); err != nil {
						h.logger.Error("Failed to resize terminal", zap.Error(err))
					}
				case "input":
					if _, err := resp.Conn.Write([]byte(msg.Data)); err != nil {
						h.logger.Error("Failed to write to terminal", zap.Error(err))
					}
				}
			}
		}
	}()

	// 处理输出
	for {
		buf := make([]byte, 1024)
		nr, err := resp.Reader.Read(buf)
		if err != nil {
			if err != io.EOF {
				h.logger.Error("Failed to read from exec", zap.Error(err))
			}
			break
		}

		if err := ws.WriteMessage(websocket.BinaryMessage, buf[:nr]); err != nil {
			h.logger.Error("Failed to write message", zap.Error(err))
			break
		}
	}
}

// ContainerLogsHandler 处理容器日志 WebSocket 连接
func (h *Handler) ContainerLogsHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	containerID := vars["id"]
	if containerID == "" {
		containerID = r.URL.Query().Get("container")
	}
	if containerID == "" {
		http.Error(w, "Missing container ID", http.StatusBadRequest)
		return
	}

	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		h.logger.Error("Failed to upgrade connection", zap.Error(err))
		return
	}
	defer ws.Close()
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
	inspect, err := h.monitor.Client().ContainerInspect(r.Context(), containerID)
	if err != nil {
		h.logger.Error("Error inspecting container", zap.Error(err))
		return
	}

	// 获取容器日志流
	ctx := r.Context()
	logReader, err := h.monitor.Client().ContainerLogs(ctx, containerID, options)
	if err != nil {
		ws.WriteMessage(websocket.TextMessage, []byte(fmt.Sprintf("Error getting logs: %v", err)))
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
						h.logger.Error("Error reading log line", zap.Error(err))
					}
					return
				}
				if err := ws.WriteMessage(websocket.TextMessage, []byte(line)); err != nil {
					h.logger.Error("Error sending log message", zap.Error(err))
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
						h.logger.Error("Error reading log header", zap.Error(err))
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
					h.logger.Error("Error reading log message", zap.Error(err))
					return
				}

				// 发送日志内容到客户端
				if err := ws.WriteMessage(websocket.TextMessage, buf); err != nil {
					h.logger.Error("Error sending log message", zap.Error(err))
					return
				}
			}
		}
	}

}
