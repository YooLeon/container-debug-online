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

    initialize() {
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
            
            const status = document.createElement('span');
            status.className = `container-status ${container.status.toLowerCase()}`;
            status.textContent = container.status;
            
            const connectBtn = document.createElement('button');
            connectBtn.className = `action-btn connect-btn ${this.ws.has(container.id) ? 'active' : ''}`;
            connectBtn.innerHTML = `<i class="fas fa-terminal"></i> ${this.ws.has(container.id) ? 'Connected' : 'Connect'}`;
            
            if (!this.isServerConnected) {
                connectBtn.disabled = true;
                connectBtn.classList.add('disabled');
            } else {
                connectBtn.onclick = () => this.connectToContainer(container.id, container.name);
            }
            
            const logsBtn = document.createElement('button');
            logsBtn.className = 'action-btn logs-btn';
            logsBtn.innerHTML = '<i class="fas fa-file-alt"></i> Logs';
            
            if (!this.isServerConnected) {
                logsBtn.disabled = true;
                logsBtn.classList.add('disabled');
            } else {
                logsBtn.onclick = () => this.showContainerLogs(container.id, container.name);
            }
            
            actions.appendChild(status);
            actions.appendChild(connectBtn);
            actions.appendChild(logsBtn);
            
            item.appendChild(name);
            item.appendChild(actions);
            
            containerList.appendChild(item);
        });
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
            macOptionIsMeta: true,
            scrollback: 1000,
            theme: {
                background: '#1e1e1e',
                foreground: '#cccccc'
            }
        });

        const fitAddon = new FitAddon.FitAddon();
        terminal.loadAddon(fitAddon);
        terminal.loadAddon(new WebLinksAddon.WebLinksAddon());
        
        terminal.open(content);
        fitAddon.fit();
        
        return { terminal, fitAddon };
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

            const ws = new WebSocket(`ws://${window.location.host}/ws?container=${containerId}`);
            
            const { terminal, terminalElement } = this.createTerminal(containerId, containerName);
            
            this.terminals.set(containerId, {
                terminal: terminal,
                element: terminalElement
            });
            
            this.ws.set(containerId, ws);

            ws.onopen = () => {
                this.setupTerminalEvents(terminal, ws);
                this.updateContainerList();
            };

            ws.onclose = () => {
                this.handleDisconnect(containerId);
            };

            ws.onerror = () => {
                this.handleDisconnect(containerId);
            };

            ws.onmessage = (event) => {
                terminal.write(event.data);
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

    sendTerminalSize(containerId) {
        const terminalData = this.terminals.get(containerId);
        if (terminalData && this.ws.has(containerId)) {
            const dimensions = terminalData.fitAddon.proposeDimensions();
            if (dimensions) {
                const size = {
                    type: 'resize',
                    cols: dimensions.cols,
                    rows: dimensions.rows
                };
                this.ws.get(containerId).send(JSON.stringify(size));
            }
        }
    }

    async showContainerLogs(containerId, containerName) {
        try {
            const modal = document.createElement('div');
            modal.className = 'modal';
            modal.innerHTML = `
                <div class="modal-content">
                    <div class="modal-header">
                        <div class="modal-title">Logs: ${containerName}</div>
                        <div class="logs-controls">
                            <label class="auto-refresh">
                                <input type="checkbox" id="auto-refresh-${containerId}">
                                自动刷新
                            </label>
                            <button class="modal-close">&times;</button>
                        </div>
                    </div>
                    <div class="logs-content">
                        <pre></pre>
                    </div>
                </div>
            `;
            
            document.body.appendChild(modal);
            modal.style.display = 'block';

            const updateLogs = async () => {
                const response = await fetch(`/containers/${containerId}/logs`);
                if (!response.ok) throw new Error(`HTTP error! status: ${response.status}`);
                const logs = await response.text();
                modal.querySelector('pre').textContent = logs;
            };

            // 初始加载日志
            await updateLogs();

            // 自动刷新复选框处理
            const autoRefreshCheckbox = modal.querySelector(`#auto-refresh-${containerId}`);
            let intervalId = null;

            autoRefreshCheckbox.addEventListener('change', () => {
                if (autoRefreshCheckbox.checked) {
                    intervalId = setInterval(updateLogs, 2000);
                } else {
                    if (intervalId) {
                        clearInterval(intervalId);
                        intervalId = null;
                    }
                }
            });

            // 关闭模态框时清理
            const cleanup = () => {
                if (intervalId) {
                    clearInterval(intervalId);
                }
                modal.remove();
            };

            modal.querySelector('.modal-close').onclick = cleanup;
            modal.onclick = (e) => {
                if (e.target === modal) cleanup();
            };
        } catch (error) {
            console.error('Failed to fetch container logs:', error);
        }
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
        terminal.onData(data => {
            if (ws.readyState === WebSocket.OPEN) {
                ws.send(data);
            }
        });

        terminal.onResize(size => {
            if (ws.readyState === WebSocket.OPEN) {
                ws.send(JSON.stringify({
                    type: 'resize',
                    cols: size.cols,
                    rows: size.rows
                }));
            }
        });
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

// 初始化
document.addEventListener('DOMContentLoaded', () => {
    new TerminalManager();
});