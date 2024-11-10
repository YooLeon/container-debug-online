class TerminalManager {
    constructor() {
        this.terminals = new Map();
        this.ws = new Map();
        this.containers = [];
        this.notificationTimer = null;
    }

    initialize() {
        this.loadContainers();
        setInterval(() => this.loadContainers(), 5000);
        this.initializeTerminalsContainer();
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
            
            const isRunning = container.status.toLowerCase() === 'running';
            if (!isRunning) {
                connectBtn.classList.add('disabled');
                connectBtn.disabled = true;
            }
            
            connectBtn.innerHTML = `<i class="fas fa-terminal"></i> ${this.ws.has(container.id) ? 'Connected' : 'Connect'}`;
            connectBtn.onclick = () => {
                if (isRunning) {
                    this.connectToContainer(container.id, container.name);
                }
            };
            
            const logsBtn = document.createElement('button');
            logsBtn.className = 'action-btn logs-btn';
            logsBtn.innerHTML = '<i class="fas fa-file-alt"></i> Logs';
            logsBtn.onclick = () => this.showContainerLogs(container.id, container.name);
            
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
                if (this.ws.has(containerId)) {
                    this.handleDisconnect(containerId, containerName);
                }
            };

            ws.onerror = () => {
                this.handleConnectionError(containerId, containerName);
            };

            ws.onmessage = (event) => {
                terminal.write(event.data);
            };

        } catch (error) {
            console.error('Failed to connect to container:', error);
            this.handleConnectionError(containerId, containerName);
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

    handleDisconnect(containerId, containerName, showNotification = true) {
        // 防止重复处理
        if (!this.ws.has(containerId)) {
            return;
        }

        // 清理 WebSocket
        const ws = this.ws.get(containerId);
        if (ws.readyState === WebSocket.OPEN) {
            ws.close();
        }
        this.ws.delete(containerId);

        // 只从映射中移除终端，但不清理输出
        if (this.terminals.has(containerId)) {
            this.terminals.delete(containerId);
        }
        
        // 更新UI状态
        this.updateContainerList();

        // 只在需要时显示通知
        if (showNotification) {
            this.showNotification('连接已断开');
        }
    }

    handleConnectionError(containerId, containerName) {
        if (this.ws.has(containerId)) {
            this.ws.delete(containerId);
        }
        if (this.terminals.has(containerId)) {
            this.terminals.delete(containerId);
        }
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
}

window.terminalManager = new TerminalManager();

document.addEventListener('DOMContentLoaded', () => {
    window.terminalManager.initialize();
});