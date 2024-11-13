# Container Debug Online

一个基于 Web 的容器调试工具，支持在线查看容器状态、日志，并提供交互式终端功能。

A web-based container debugging tool that supports real-time container status monitoring, log viewing, and interactive terminal functionality.

## 功能特性 | Features

- 🔍 实时监控 Docker 容器状态 
  Real-time Docker container status monitoring
- 📝 在线查看容器日志 
  Online container log viewing
- 🖥️ 提供交互式终端（Web TTY）
  Interactive terminal (Web TTY)
- 🔄 支持容器健康检查 
  Container health check support
- 🎯 支持通过容器 ID、名称或服务名进行容器定位 
  Container lookup by ID, name, or service name
- 📊 支持 docker-compose 项目的容器管理 
  Docker Compose project container management

## 快速开始 | Quick Start

### 前置条件 | Prerequisites

- Go 1.16+
- Docker
- Docker Compose (可选 | Optional)

### 安装 | Installation

```bash
git clone https://github.com/YooLeon/container-debug-online.git
cd container-debug-online
go build
```

### 运行 | Running

```bash
# 直接运行
./container-debug-online

# 或者指定端口运行
./container-debug-online -port 8080
```

默认情况下，服务将在 `http://localhost:14264` 启动

By default, the service will start at `http://localhost:14264`

## 配置 | Configuration

### 命令行参数 | Command Line Arguments

```bash
--port int          # 服务端口 (默认: 14264)
                    # Server port (default: 14264)
--host string       # 服务监听地址 (默认: "0.0.0.0")
                    # Server host (default: "0.0.0.0")
--compose string    # docker-compose.yml 文件路径
                    # Path to docker-compose.yml
--interval duration # 容器监控间隔时间 (默认: 5s)
                    # Monitor interval (default: 5s)
--password string   # 认证密码，为空则不启用认证
                    # Authentication password, disabled if empty
```

### 认证 | Authentication

系统支持基本的密码认证机制：

1. 启动时设置密码 | Set password when starting:
```bash
./container-debug-online --password your-secret-password
```

2. 访问受保护的接口时：
   - 需要在请求头中添加 `Authorization` 字段
   - 值为设置的密码
   When accessing protected endpoints:
   - Add `Authorization` header in requests
   - Value should be the configured password

注意：健康检查接口 `/health` 不需要认证
Note: The health check endpoint `/health` doesn't require authentication

### API 路由 | API Routes

```bash
GET    /health                  # 健康检查 | Health check
GET    /containers             # 获取容器列表 | Get container list
GET    /containers/{id}/logs   # 获取容器日志 | Get container logs
GET    /container/logs         # 获取容器日志 | Get container logs
WS     /ws                     # WebSocket 终端连接 | WebSocket terminal connection
```

### 示例 | Examples

1. 指定端口和密码启动 | Start with specific port and password:
```bash
./container-debug-online --port 8080 --password mysecret
```

2. 指定 docker-compose 文件和监控间隔 | Specify docker-compose file and monitor interval:
```bash
./container-debug-online --compose ./docker-compose.yml --interval 10s
```

## 使用方法 | Usage

1. 访问 Web 界面 | Access the web interface
   - 打开浏览器访问 `http://localhost:14264`
   - Open your browser and visit `http://localhost:14264`

2. 容器管理 | Container Management
   - 查看所有运行中的容器 | View all running containers
   - 查看容器详细信息 | View container details
   - 访问容器终端 | Access container terminal

3. 日志查看 | Log Viewing
   - 实时查看容器日志 | Real-time container logs
   - 支持日志过滤和搜索 | Support log filtering and searching

4. 终端操作 | Terminal Operations
   - 支持多终端会话 | Support multiple terminal sessions
   - 命令历史记录 | Command history
   - 自动补全功能 | Auto-completion

## API 接口 | API Endpoints

```bash
GET    /api/containers          # 获取容器列表 | Get container list
GET    /api/containers/:id      # 获取容器详情 | Get container details
GET    /api/containers/:id/logs # 获取容器日志 | Get container logs
POST   /api/containers/:id/exec # 在容器中执行命令 | Execute command in container
```

## 开发 | Development

```bash
# 安装依赖
go mod download

# 运行测试
go test ./...

# 构建
go build
```

### 构建 | Build

项目使用 Go embed 将静态文件打包到二进制文件中，构建时无需额外的静态文件拷贝。

The project uses Go embed to package static files into the binary, no additional static file copying is needed during build.

```bash
# 开发模式构建
go build

# 生产模式构建（启用优化）
go build -ldflags="-s -w"
```

构建后得到的二进制文件可以直接运行，无需额外的静态文件。
The built binary can be run directly without additional static files.

## 贡献 | Contributing

1. Fork 本项目 | Fork this repository
2. 创建特性分支 | Create feature branch
3. 提交变更 | Commit changes
4. 推送分支 | Push branch
5. 创建 Pull Request | Create Pull Request

## 许可证 | License

[MIT License](LICENSE)

## 联系方式 | Contact

- Issues: [github.com/YooLeon/container-debug-online/issues](https://github.com/YooLeon/container-debug-online/issues)

## 致谢 | Acknowledgments

感谢所有贡献者的付出！

Thanks to all contributors!

