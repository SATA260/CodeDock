package agent

import (
	"context"

	"codedock/internal/events"
	pkgagent "codedock/pkg/agent"
)

// Transition 消费模型流，并把 LLM 收到的增量通过事件总线分发出去。本阶段为空实现。
func Transition(_ context.Context, bus *events.Bus, _ pkgagent.ModelStream) error {
	if bus == nil {
		return nil
	}
	bus.Publish(events.Event{})
	return nil
}
