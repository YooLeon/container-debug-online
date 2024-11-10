class TerminalManager {
    constructor() {
        this.containers = [];
        this.terminals = new Map();
        this.ws = new Map();
        this.logPollers = new Map();
        this.isConnected = true;
        this.overlay = null;
        this.notification = null;
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
        if (!this.isConnected) {
            return;
        }

        try {
            const response = await fetch('/containers');
            if (response.status === 401) {
                window.location.reload();
                return;
            }
            if (!response.ok) {
                throw new Error(`HTTP error! status: ${response.status}`);
            }
            const containers = await response.json();
            this.containers = Array.isArray(containers) ? containers : [];
            this.updateContainerList();
        } catch (error) {
            console.error('Failed to load containers:', error);
            this.containers = [];
            this.isConnected = false;
            this.showNotification('连接已断开，按回车键重新连接', 'error', true);
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
            name.title = container.name; // 添加工具提示
            
            const actions = document.createElement('div');
            actions.className = 'container-actions';
            
            const status = document.createElement('span');
            status.className = `container-status ${container.status.toLowerCase()}`;
            status.textContent = container.status;
            
            const connectBtn = document.createElement('button');
            connectBtn.className = `action-btn connect-btn ${this.ws.has(container.id) ? 'active' : ''}`;
            connectBtn.innerHTML = `<i class="fas fa-terminal"></i> ${this.ws.has(container.id) ? 'Connected' : 'Connect'}`;
            connectBtn.onclick = () => this.connectToContainer(container.id, container.name);
            
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

    showNotification(message, type = 'info', persistent = false) {
        // 移除现有的通知和遮罩
        this.removeNotification();

        // 创建遮罩
        this.overlay = document.createElement('div');
        this.overlay.className = 'overlay';
        document.body.appendChild(this.overlay);

        // 创建通知
        this.notification = document.createElement('div');
        this.notification.className = `notification ${type}`;
        this.notification.textContent = message;
        document.body.appendChild(this.notification);

        // 显示遮罩和通知
        this.overlay.style.display = 'block';
        
        if (!persistent) {
            setTimeout(() => {
                this.removeNotification();
            }, 5000);
        }
    }

    removeNotification() {
        if (this.overlay) {
            this.overlay.remove();
            this.overlay = null;
        }
        if (this.notification) {
            this.notification.remove();
            this.notification = null;
        }
    }

    async connectToContainer(containerId, containerName, retry = false) {
        if (!this.isConnected && !retry) {
            return;
        }

        try {
            const { terminal, fitAddon } = this.createTerminal(containerId, containerName);
            
            const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
            const ws = new WebSocket(`${protocol}//${window.location.host}/ws?container=${containerId}`);
            
            ws.onopen = () => {
                this.terminals.set(containerId, { terminal, fitAddon });
                this.ws.set(containerId, ws);
                this.isConnected = true;
                this.removeNotification();
                this.loadContainers();
            };

            ws.onclose = () => {
                this.isConnected = false;
                this.showNotification('连接已断开\n按回车键重新连接', 'error', true);
                
                // 添加全局回车键监听
                const handleKeyPress = (event) => {
                    if (event.key === 'Enter') {
                        document.removeEventListener('keypress', handleKeyPress);
                        this.connectToContainer(containerId, containerName, true);
                    }
                };
                document.addEventListener('keypress', handleKeyPress);
            };

            ws.onerror = () => {
                this.isConnected = false;
                this.showNotification('连接已断开，按回车键重新连接', 'error', true);
            };

            ws.onmessage = (event) => {
                terminal.write(event.data);
            };

            terminal.onData(data => {
                if (ws && ws.readyState === WebSocket.OPEN) {
                    ws.send(data);
                }
            });

            window.addEventListener('resize', () => {
                fitAddon.fit();
                this.sendTerminalSize(containerId);
            });

        } catch (error) {
            this.isConnected = false;
            this.showNotification('连接已断开，按回车键重新连接', 'error', true);
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
                        <h3>Logs: ${containerName}</h3>
                        <div class="logs-header">
                            <div class="auto-refresh">
                                <input type="checkbox" id="auto-refresh-${containerId}">
                                <label for="auto-refresh-${containerId}">自动刷新</label>
                            </div>
                            <button class="logs-refresh">
                                <i class="fas fa-sync-alt"></i> 刷新
                            </button>
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

            // 手动刷新按钮
            modal.querySelector('.logs-refresh').onclick = updateLogs;
            
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
            this.showNotification('获取日志失败', 'error');
        }
    }
}

window.terminalManager = new TerminalManager();

document.addEventListener('DOMContentLoaded', () => {
    window.terminalManager.initialize();
});