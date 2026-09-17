package agent

import (
	"context"

	"codedock/pkg/agent/tool"
)

// NewSlotLimiter 创建容量为 n 的占槽器。n<=0 表示不限制，返回 nil。
func NewSlotLimiter(n int) tool.Gate {
	if n <= 0 {
		return nil
	}
	return &slotLimiter{slots: make(chan struct{}, n)}
}

type slotLimiter struct {
	slots chan struct{}
}

// Acquire 领取一个槽；取消或超时则返回 ctx 错误。
func (s *slotLimiter) Acquire(ctx context.Context) error {
	if s == nil {
		return nil
	}
	select {
	case s.slots <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Release 归还一个槽。重复调用不会阻塞。
func (s *slotLimiter) Release() {
	if s == nil {
		return
	}
	select {
	case <-s.slots:
	default:
	}
}
