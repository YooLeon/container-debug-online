package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"docker-monitor/internal/config"
	"docker-monitor/internal/docker"
	"docker-monitor/internal/web"

	"github.com/docker/docker/client"
	"go.uber.org/zap"
)

func main() {
	// 加载配置
	cfg := config.LoadConfig()

	// 设置日志格式
	log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)

	// 创建 Docker 客户端
	dockerClient, err := client.NewClientWithOpts(client.FromEnv)
	if err != nil {
		log.Fatalf("Failed to create Docker client: %v", err)
	}

	// 初始化日志
	logger, _ := zap.NewDevelopment()

	// 初始化 Docker 监控器
	monitor := docker.NewMonitor(
		dockerClient,
		logger,
		cfg.MonitorInterval,
		cfg.ComposePath,
	)
	defer monitor.Close()

	// 创建 HTTP handler
	handler := web.NewHandler(monitor)

	// 设置路由
	mux := http.NewServeMux()
	mux.HandleFunc("/ws", handler.TerminalHandler)
	mux.HandleFunc("/containers", handler.ContainersHandler)
	mux.Handle("/", http.FileServer(http.Dir("static")))

	// 创建 HTTP 服务器
	server := &http.Server{
		Addr:    cfg.ServerPort,
		Handler: mux,
	}

	// 优雅关闭通道
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	// 启动监控服务
	go func() {
		ticker := time.NewTicker(cfg.MonitorInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				containers, err := monitor.ListContainers()
				if err != nil {
					log.Printf("Error listing containers: %v", err)
					continue
				}

				for _, container := range containers {
					status, err := monitor.CheckContainerStatus(container.ID)
					if err != nil {
						log.Printf("Error checking container %s (%s): %v", container.Name, container.ID, err)
						continue
					}
					log.Printf("Container %s (%s) status: %s", container.Name, container.ID, status)
				}
			case <-stop:
				return
			}
		}
	}()

	// 启动 HTTP 服务器
	go func() {
		log.Printf("Server starting on http://localhost%s", cfg.ServerPort)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server failed to start: %v", err)
		}
	}()

	// 等待中断信号
	<-stop
	log.Println("Shutting down server...")

	// 创建关闭上下文
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// 优雅关闭服务器
	if err := server.Shutdown(ctx); err != nil {
		log.Printf("Server forced to shutdown: %v", err)
	}

	log.Println("Server exited")
}
