class TerminalManager {
    constructor() {
        this.terminals = new Map();
        this.ws = new Map();
        this.containers = [];
        this.isServerConnected = true;
        this.checkInterval = null;
        this.containerLoadInterval = null;

        // 初始化时加载容器列表
        this.loadContainers();
        // 启动定期更新
        this.startContainerUpdates();
        this.initializeTerminalsContainer();
    }

    startContainerUpdates() {
        // 设置定期更新容器列表
        this.containerLoadInterval = setInterval(() => {
            if (this.isServerConnected) {
                this.loadContainers();
            }
        }, 5000);
    }

    initializeTerminalsContainer() {
        const mainContent = document.querySelector('.main-content');
        mainContent.innerHTML = '<div class="terminals-container"></div>';
    }

    async loadContainers() {
        try {
            const response = await fetch('/containers');
            const containers = await response.json();
            this.containers = containers;
            this.updateContainerList();
        } catch (error) {
            console.error('Failed to load containers:', error);
            this.checkServerConnection();
        }
    }

    updateContainerList() {
        const containerList = document.getElementById('container-list');
        containerList.innerHTML = '';

        this.containers.forEach(container => {
            const item = document.createElement('div');
            item.className = 'container-item';
            
            const name = document.createElement('span');
            name.className = 'container-name';
            name.textContent = container.name;
            name.title = container.name;
            
            const actions = document.createElement('div');
            actions.className = 'container-actions';
            
            const healthStatus = document.createElement('span');
            healthStatus.className = `health-status ${container.healthy ? 'healthy' : 'unhealthy'}`;
            healthStatus.title = this.getHealthStatusTitle(container);
            
            const status = document.createElement('span');
            status.className = `container-status ${container.status.toLowerCase()}`;
            status.textContent = container.status;
            
            const connectBtn = document.createElement('button');
            connectBtn.className = `action-btn connect-btn ${this.ws.has(container.id) ? 'active' : ''}`;
            connectBtn.innerHTML = `<i class="fas fa-terminal"></i> ${this.ws.has(container.id) ? 'Connected' : 'Connect'}`;
            
            // 检查容器是否可连接
            const canConnect = container.status.toLowerCase() === 'running' && container.id;
            
            if (!this.isServerConnected || !canConnect) {
                connectBtn.disabled = true;
                connectBtn.classList.add('disabled');
                // 添加提示信息
                connectBtn.title = !this.isServerConnected ? '服务器未连接' : '容器未运行';
            } else {
                connectBtn.onclick = () => this.connectToContainer(container.id, container.name);
            }
            
            const logsBtn = document.createElement('button');
            logsBtn.className = 'action-btn logs-btn';
            logsBtn.innerHTML = '<i class="fas fa-file-alt"></i> Logs';
            
            // 日志按钮只在容器有 ID 时可用
            if (!this.isServerConnected || !container.id) {
                logsBtn.disabled = true;
                logsBtn.classList.add('disabled');
                logsBtn.title = !this.isServerConnected ? '服务器未连接' : '容器未创建';
            } else {
                logsBtn.onclick = () => this.showContainerLogs(container.id, container.name);
            }
            
            actions.appendChild(healthStatus);
            actions.appendChild(status);
            actions.appendChild(connectBtn);
            actions.appendChild(logsBtn);
            
            item.appendChild(name);
            item.appendChild(actions);
            
            containerList.appendChild(item);
        });
    }

    getHealthStatusTitle(container) {
        let details = [];
        
        if (container.ports_health) {
            for (const [port, healthy] of Object.entries(container.ports_health)) {
                details.push(`Port ${port}: ${healthy ? '✓' : '✗'}`);
            }
        }
        
        if (container.service) {
            details.push(`Service ${container.service}: ${container.healthy ? 'Healthy' : 'Unhealthy'}`);
        }
        
        return details.join('\n') || 'Container Status';
    }

    createTerminal(containerId, containerName) {
        const terminalsContainer = document.querySelector('.terminals-container');
        
        const wrapper = document.createElement('div');
        wrapper.className = 'terminal-wrapper';
        wrapper.id = `terminal-wrapper-${containerId}`;
        
        const header = document.createElement('div');
        header.className = 'terminal-header';
        header.innerHTML = `
            <span class="title">${containerName}</span>
            <button class="close-btn" onclick="terminalManager.closeTerminal('${containerId}')">&times;</button>
        `;
        
        const content = document.createElement('div');
        content.className = 'terminal-content';
        content.id = `terminal-${containerId}`;
        
        wrapper.appendChild(header);
        wrapper.appendChild(content);
        terminalsContainer.appendChild(wrapper);
        
        const terminal = new Terminal({
            cursorBlink: true,
            theme: {
                background: '#2f2f2f',  // 改为深灰色
                foreground: '#ffffff'
            },
            scrollback: 1000,
            fontSize: 14,
            fontFamily: 'Menlo, Monaco, "Courier New", monospace',
            letterSpacing: 0,
            lineHeight: 1,
            allowTransparency: true,
            rendererType: 'canvas'
        });

        // 加载 FitAddon
        const fitAddon = new FitAddon.FitAddon();
        terminal.loadAddon(fitAddon);

        terminal.open(content);
        terminal._containerId = containerId;
        terminal._fitAddon = fitAddon;  // 存储 fitAddon 实例
        terminal.focus();

        // 初始化终端大小
        this.fitTerminal(terminal, content);

        // 监听容器大小变化
        const resizeObserver = new ResizeObserver(() => {
            this.fitTerminal(terminal, content);
        });
        resizeObserver.observe(wrapper);

        return { terminal, content };
    }

    fitTerminal(terminal, element) {
        if (!terminal || !element || !terminal._fitAddon) return;

        try {
            // 使用 FitAddon 来自动调整大小
            terminal._fitAddon.fit();
            
            // 发送新的尺寸到服务器
            this.sendTerminalSize(terminal._containerId, terminal.cols, terminal.rows);
        } catch (e) {
            console.error('Failed to fit terminal:', e);
        }
    }

    sendTerminalSize(containerId, cols, rows) {
        const ws = this.ws.get(containerId);
        if (ws && ws.readyState === WebSocket.OPEN) {
            ws.send(JSON.stringify({
                type: "resize",
                cols: cols,
                rows: rows
            }));
        }
    }

    showNotification(message) {
        const notification = document.getElementById('notification');
        if (!notification) return;

        // 设置消息和关闭按钮
        notification.innerHTML = `
            ${message}
            <button class="notification-close">&times;</button>
        `;
        
        // 显示通知
        notification.style.display = 'block';

        // 只添加关闭按钮的点击事件
        const closeBtn = notification.querySelector('.notification-close');
        if (closeBtn) {
            closeBtn.onclick = () => {
                notification.style.display = 'none';
            };
        }

        // 移除自动关闭的定时器
        if (this.notificationTimer) {
            clearTimeout(this.notificationTimer);
            this.notificationTimer = null;
        }
    }

    async connectToContainer(containerId, containerName) {
        try {
            if (this.terminals.has(containerId)) {
                return;
            }

            const { terminal, content } = this.createTerminal(containerId, containerName);
            
            const ws = new WebSocket(`ws://${window.location.host}/ws?container=${containerId}`);
            
            ws.onopen = () => {
                this.terminals.set(containerId, {
                    terminal: terminal,
                    element: content
                });
                
                this.ws.set(containerId, ws);
                
                // 设置终端事件
                terminal.onData(data => {
                    if (ws.readyState === WebSocket.OPEN) {
                        ws.send(JSON.stringify({
                            type: "input",
                            data: data
                        }));
                    }
                });

                // 初始化终端大小
                this.sendTerminalSize(containerId, terminal.cols, terminal.rows);
                this.updateContainerList();
            };

            ws.onmessage = (event) => {
                const data = event.data;
                if (data instanceof Blob) {
                    // 处理二进制数据
                    const reader = new FileReader();
                    reader.onload = () => {
                        terminal.write(new Uint8Array(reader.result));
                    };
                    reader.readAsArrayBuffer(data);
                } else {
                    // 处理文本数据
                    terminal.write(data);
                }
            };

            ws.onclose = () => {
                this.handleDisconnect(containerId);
            };

            ws.onerror = (error) => {
                console.error('WebSocket error:', error);
                this.handleDisconnect(containerId);
            };

        } catch (error) {
            console.error('Failed to connect to container:', error);
            this.handleDisconnect(containerId);
        }
    }

    closeTerminal(containerId) {
        const terminalData = this.terminals.get(containerId);
        if (terminalData) {
            terminalData.terminal.dispose();
            this.terminals.delete(containerId);
        }

        const ws = this.ws.get(containerId);
        if (ws) {
            ws.close();
            this.ws.delete(containerId);
        }

        const wrapper = document.getElementById(`terminal-wrapper-${containerId}`);
        if (wrapper) {
            wrapper.remove();
        }

        this.updateContainerList();
    }

    showContainerLogs(containerId, containerName) {
        // 先清理之前的 WebSocket 连接
        if (this.logWs) {
            this.logWs.close();
            this.logWs = null;
        }

        // 先移除旧的模态框（如果存在）
        let oldModal = document.getElementById('logs-modal');
        if (oldModal) {
            oldModal.remove();
        }

        // 创建新的模态框
        const modal = document.createElement('div');
        modal.id = 'logs-modal';
        modal.className = 'modal';
        
        modal.innerHTML = `
            <div class="modal-content">
                <div class="modal-header">
                    <h2 id="logs-title"></h2>
                    <span class="close">&times;</span>
                </div>
                <div class="modal-body">
                    <pre id="logs-content"></pre>
                </div>
            </div>
        `;
        
        document.body.appendChild(modal);

        const logsContent = document.getElementById('logs-content');
        const title = document.getElementById('logs-title');
        const closeBtn = modal.querySelector('.close');
        
        logsContent.textContent = 'Loading logs...';
        title.textContent = `${containerName} Logs`;
        modal.style.display = 'block';

        // 自动滚动标志
        let autoScroll = true;

        // 监听滚动事件
        logsContent.addEventListener('scroll', () => {
            // 检查是否滚动到底部
            const scrollBottom = Math.abs(
                logsContent.scrollHeight - 
                logsContent.clientHeight - 
                logsContent.scrollTop
            ) <= 1;

            // 只有当用户主动滚动时才更新 autoScroll
            if (!scrollBottom && autoScroll) {
                autoScroll = false;
            } else if (scrollBottom && !autoScroll) {
                autoScroll = true;
            }
        });

        const ws = new WebSocket(`ws://${window.location.host}/container/logs?container=${containerId}`);
        
        ws.onopen = () => {
            console.log('Log WebSocket connected');
            logsContent.textContent = '';
        };

        ws.onmessage = (event) => {
            const wasScrolledToBottom = autoScroll;
            logsContent.textContent += event.data;
            
            // 只有在之前处于底部时才自动滚动
            if (wasScrolledToBottom) {
                logsContent.scrollTop = logsContent.scrollHeight;
            }
        };

        ws.onerror = (error) => {
            console.error('Log WebSocket error:', error);
            logsContent.textContent = 'Error loading logs';
        };

        ws.onclose = () => {
            console.log('Log WebSocket closed');
        };

        const closeModal = () => {
            ws.close();
            modal.remove();
            this.logWs = null;
        };

        closeBtn.onclick = closeModal;

        window.onclick = (event) => {
            if (event.target === modal) {
                closeModal();
            }
        };

        this.logWs = ws;
    }

    handleDisconnect(containerId) {
        // 清理 WebSocket
        if (this.ws.has(containerId)) {
            const ws = this.ws.get(containerId);
            if (ws.readyState === WebSocket.OPEN) {
                ws.close();
            }
            this.ws.delete(containerId);
        }

        // 从映射中移除终端
        if (this.terminals.has(containerId)) {
            this.terminals.delete(containerId);
        }
        
        // 更新UI状态
        this.updateContainerList();
    }

    setupTerminalEvents(terminal, ws) {
        // 设置终端输入事件
        terminal.onData(data => {
            if (ws.readyState === WebSocket.OPEN) {
                ws.send(JSON.stringify({
                    type: "input",
                    data: data
                }));
            }
        });

        // 初始化时发送终端大小
        this.sendTerminalSize(terminal._containerId, terminal.cols, terminal.rows);
    }

    async checkServerConnection() {
        try {
            const response = await fetch('/health');
            if (!response.ok) {
                this.handleServerDisconnect();
            } else if (!this.isServerConnected) {
                // 服务恢复时
                this.isServerConnected = true;
                this.hideNotification();
                this.loadContainers();
            }
        } catch (error) {
            this.handleServerDisconnect();
        }
    }

    handleServerDisconnect() {
        if (this.isServerConnected) {
            this.isServerConnected = false;
            this.showNotification('服务器连接已断开');
            
            // 禁用所有容器操作按钮
            const buttons = document.querySelectorAll('.action-btn');
            buttons.forEach(btn => {
                btn.disabled = true;
                btn.classList.add('disabled');
            });

            // 清除所有定时器
            if (this.containerLoadInterval) {
                clearInterval(this.containerLoadInterval);
                this.containerLoadInterval = null;
            }

            // 更新UI状态
            this.updateContainerList();
        }
    }
}
let terminalManager = null;
// 初始化
document.addEventListener('DOMContentLoaded', () => {
    terminalManager = new TerminalManager();
});
