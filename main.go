package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/YooLeon/container-debug-online/internal/config"
	"github.com/YooLeon/container-debug-online/internal/docker"
	"github.com/YooLeon/container-debug-online/internal/middleware"
	"github.com/YooLeon/container-debug-online/internal/web"

	"github.com/docker/docker/client"
	"github.com/gorilla/mux"
	"go.uber.org/zap"
)

func main() {
	// 加载配置
	cfg := config.LoadConfig()

	// 配置日志
	logger, err := zap.NewProduction()
	if err != nil {
		panic(err)
	}
	defer logger.Sync()

	// 替换全局logger
	zap.ReplaceGlobals(logger)

	// 创建 Docker client
	cli, err := client.NewClientWithOpts(client.FromEnv)
	if err != nil {
		zap.L().Fatal("Failed to create docker client", zap.Error(err))
	}

	// 加载 compose 配置
	composeConfig, err := config.LoadComposeConfig(cfg.ComposePath)
	if err != nil {
		zap.L().Fatal("Failed to load compose config", zap.Error(err))
	}

	// 创建 Docker 监控器
	monitor := docker.NewMonitor(cli, zap.L(), cfg.MonitorInterval, composeConfig)
	defer monitor.Close()

	// 创建 HTTP handler
	webHandler := web.NewHandler(monitor)

	// 创建路由器
	router := mux.NewRouter()

	// 健康检查路由（不需要认证）
	router.HandleFunc("/health", webHandler.HealthCheckHandler).Methods("GET")

	// 其他需要认证的路由
	if cfg.Password != "" {
		router.Use(middleware.AuthMiddleware(cfg.Password))
	}

	router.HandleFunc("/ws", webHandler.TerminalHandler)
	router.HandleFunc("/containers", webHandler.ContainersHandler)
	router.HandleFunc("/containers/{id}/logs", webHandler.ContainerLogsHandler)
	router.HandleFunc("/container/logs", webHandler.ContainerLogsHandler)
	router.HandleFunc("/container/logs/download", webHandler.DownloadLogsHandler)

	// 静态文件服务
	router.PathPrefix("/").Handler(http.FileServer(web.GetFileSystem()))

	// 创建 HTTP 服务器
	server := &http.Server{
		Addr:    fmt.Sprintf("%s:%d", cfg.ServerHost, cfg.ServerPort),
		Handler: router,
	}

	// 优雅关闭通道
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	if err := monitor.UpdateStatus(); err != nil {
		zap.L().Error("Failed to update status", zap.Error(err))
	}

	// 修改监控循环
	go func() {
		ticker := time.NewTicker(cfg.MonitorInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				if err := monitor.UpdateStatus(); err != nil {
					zap.L().Error("Failed to update status", zap.Error(err))
				}
			case <-stop:
				return
			}
		}
	}()

	// 启动服务器
	go func() {
		log.Printf("Server starting on %s:%d", cfg.ServerHost, cfg.ServerPort)
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
