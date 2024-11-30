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
        mainContent.innerHTML = `
            <div class="terminals-container">
                <div class="terminal-tabs"></div>
                <div class="terminal-panels"></div>
            </div>
        `;
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
            name.textContent = container.service;
            name.title = container.service;
            
            const actions = document.createElement('div');
            actions.className = 'container-actions';
            
            const healthStatus = document.createElement('span');
            healthStatus.className = `health-status ${container.healthy ? 'healthy' : 'unhealthy'}`;
            healthStatus.title = this.getHealthStatusTitle(container);
            
            const status = document.createElement('span');
            status.className = `container-status ${container.status.toLowerCase()}`;
            if (!this.isServerConnected) {
                status.className = 'container-status disconnected';
                status.textContent = 'Disconnected';
            } else {
                status.textContent = container.status;
            }
            
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
                connectBtn.onclick = () => this.connectToContainer(container.id, container.service);
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
                logsBtn.onclick = () => this.showContainerLogs(container.id, container.service);
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
        const terminalTabs = terminalsContainer.querySelector('.terminal-tabs');
        const terminalPanels = terminalsContainer.querySelector('.terminal-panels');
        
        // 创建标签
        const tab = document.createElement('div');
        tab.className = 'terminal-tab';
        tab.dataset.containerId = containerId;
        tab.innerHTML = `
            <span class="tab-title">${containerName}</span>
            <button class="tab-close" onclick="event.stopPropagation(); terminalManager.closeTerminal('${containerId}')">&times;</button>
        `;
        
        // 创建终端面板
        const wrapper = document.createElement('div');
        wrapper.className = 'terminal-wrapper';
        wrapper.id = `terminal-wrapper-${containerId}`;
        wrapper.style.display = 'none';
        
        const content = document.createElement('div');
        content.className = 'terminal-content';
        content.id = `terminal-${containerId}`;
        content.style.height = '100%';
        
        wrapper.appendChild(content);
        
        // 添加到DOM
        terminalTabs.appendChild(tab);
        terminalPanels.appendChild(wrapper);
        
        // 设置标签点击事件
        tab.onclick = () => this.switchTerminal(containerId);
        
        const terminal = new Terminal({
            cursorBlink: true,
            theme: {
                background: '#2f2f2f',
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

        const fitAddon = new FitAddon.FitAddon();
        terminal.loadAddon(fitAddon);

        terminal.open(content);
        terminal._containerId = containerId;
        terminal._fitAddon = fitAddon;
        
        // 激活新创建的终端
        this.switchTerminal(containerId);

        // 确保 xterm.js 的元素也填充整个容器
        const xtermElement = content.querySelector('.xterm');
        if (xtermElement) {
            xtermElement.style.height = '100%';
            const xtermScreen = xtermElement.querySelector('.xterm-screen');
            if (xtermScreen) {
                xtermScreen.style.height = '100%';
            }
        }

        terminal.focus();

        // 使用 requestAnimationFrame 确保在下一帧渲染时调整尺寸
        requestAnimationFrame(() => {
            this.fitTerminal(terminal, content);
        });

        // 监听容器大小变化
        const resizeObserver = new ResizeObserver(() => {
            requestAnimationFrame(() => {
                this.fitTerminal(terminal, content);
            });
        });
        resizeObserver.observe(wrapper);

        return { terminal, content };
    }

    fitTerminal(terminal, element) {
        if (!terminal || !element || !terminal._fitAddon) return;

        try {
            // 使用 FitAddon 来自动调整大小
            terminal._fitAddon.fit();
            
            // 获取新的尺寸
            const newCols = terminal.cols;
            const newRows = terminal.rows;
            
            // 发送新的尺寸到服务器
            this.sendTerminalSize(terminal._containerId, newCols, newRows);
            
            // 触发 resize 事件以确保终端内容正确重绘
            terminal.refresh(0, terminal.rows - 1);
        } catch (e) {
            console.error('Failed to fit terminal:', e);
        }
    }

    sendTerminalSize(containerId, cols, rows) {
        const ws = this.ws.get(containerId);
        if (ws && ws.readyState === WebSocket.OPEN) {
            try {
                const resizeMessage = JSON.stringify({
                    type: "resize",
                    cols: cols,
                    rows: rows
                });
                ws.send(resizeMessage);
                console.debug(`Sent terminal resize: ${cols}x${rows} for container ${containerId}`);
            } catch (error) {
                console.error('Failed to send terminal size:', error);
            }
        }
    }

    showNotification(message, type = 'info') {
        const notification = document.createElement('div');
        notification.className = `notification ${type} show`;
        notification.textContent = message;
        
        // 移除旧的通知
        const oldNotification = document.querySelector('.notification');
        if (oldNotification) {
            oldNotification.remove();
        }
        
        document.body.appendChild(notification);
        
        // 只有非错误类型的通知才自动消失
        if (type !== 'error') {
            setTimeout(() => {
                notification.classList.remove('show');
                setTimeout(() => notification.remove(), 300);
            }, 5000);
        }
    }

    // 添加一个新方法用于隐藏通知
    hideNotification() {
        const notification = document.querySelector('.notification');
        if (notification) {
            notification.classList.remove('show');
            setTimeout(() => notification.remove(), 300);
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

                // 立即发送初始终端大小
                requestAnimationFrame(() => {
                    this.fitTerminal(terminal, content);
                });
                
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

        // 移除标签和终端面板
        const tab = document.querySelector(`.terminal-tab[data-container-id="${containerId}"]`);
        const wrapper = document.getElementById(`terminal-wrapper-${containerId}`);
        
        if (tab) {
            // 如果关闭的是当前活动的标签，切换到其他标签
            if (tab.classList.contains('active')) {
                const nextTab = tab.nextElementSibling || tab.previousElementSibling;
                if (nextTab) {
                    this.switchTerminal(nextTab.dataset.containerId);
                }
            }
            tab.remove();
        }
        if (wrapper) {
            wrapper.remove();
        }

        this.updateContainerList();
    }

    showContainerLogs(containerId, serviceName) {
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
                    <div class="modal-header-actions">
                        <button class="download-logs-btn" title="Download logs">
                            <i class="fas fa-download"></i>
                        </button>
                        <span class="close">&times;</span>
                    </div>
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
        const downloadBtn = modal.querySelector('.download-logs-btn');
        
        logsContent.textContent = 'Loading logs...';
        title.textContent = `${serviceName} Logs`;
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
        downloadBtn.onclick = async () => {
            try {
                const response = await fetch(`/container/logs/download?container=${containerId}`);
                if (!response.ok) throw new Error('Failed to download logs');
                
                const blob = await response.blob();
                const url = window.URL.createObjectURL(blob);
                const a = document.createElement('a');
                a.href = url;
                a.download = `${serviceName}.log`;
                document.body.appendChild(a);
                a.click();
                window.URL.revokeObjectURL(url);
                document.body.removeChild(a);
            } catch (error) {
                console.error('Error downloading logs:', error);
            }
        };

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
                this.hideNotification();  // 使用新方法隐藏通知
                this.loadContainers();
            }
        } catch (error) {
            this.handleServerDisconnect();
        }
    }

    handleServerDisconnect() {
        if (this.isServerConnected) {
            this.isServerConnected = false;
            
            // 显示断开连接通知
            this.showNotification('服务器连接已断开', 'error');
            
            // 更新所有容器状态为断开连接
            this.containers.forEach(container => {
                container.status = 'Disconnected';
                container.healthy = false;
            });
            
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

    // 添加新方法：切换终端
    switchTerminal(containerId) {
        // 隐藏所有终端和取消激活所有标签
        const allTabs = document.querySelectorAll('.terminal-tab');
        const allWrappers = document.querySelectorAll('.terminal-wrapper');
        
        allTabs.forEach(tab => tab.classList.remove('active'));
        allWrappers.forEach(wrapper => wrapper.style.display = 'none');
        
        // 显示选中的终端和激活对应标签
        const selectedTab = document.querySelector(`.terminal-tab[data-container-id="${containerId}"]`);
        const selectedWrapper = document.getElementById(`terminal-wrapper-${containerId}`);
        
        if (selectedTab && selectedWrapper) {
            selectedTab.classList.add('active');
            selectedWrapper.style.display = 'block';
            
            // 重新适应终端大小
            const terminalData = this.terminals.get(containerId);
            if (terminalData) {
                // 等待 DOM 更新完成后再调整尺寸
                setTimeout(() => {
                    this.fitTerminal(terminalData.terminal, terminalData.element);
                    terminalData.terminal.focus();
                }, 0);
            }
        }
    }
}
let terminalManager = null;
// 初始化
document.addEventListener('DOMContentLoaded', () => {
    terminalManager = new TerminalManager();
});
