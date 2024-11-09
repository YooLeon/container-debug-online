package monitor

import (
	"context"
	"time"
)

type Monitor struct {
	ctx      context.Context
	interval time.Duration
}

func (m *Monitor) StartHealthCheck() {
	// 立即执行第一次健康检查
	m.checkHealth()

	// 然后开始定时检查
	ticker := time.NewTicker(m.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			m.checkHealth()
		case <-m.ctx.Done():
			return
		}
	}
}
