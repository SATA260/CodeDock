package agent

import (
	"context"
	"encoding/json"
	"time"

	"codedock/pkg/agent/tool"
	einomodel "github.com/cloudwego/eino/components/model"
)

// ModelStreamEventType 标识模型流增量。
type ModelStreamEventType string

const (
	ModelStreamStarted   ModelStreamEventType = "started"
	ModelStreamTextDelta ModelStreamEventType = "text_delta"
	ModelStreamToolDelta ModelStreamEventType = "tool_delta"
	ModelStreamCompleted ModelStreamEventType = "completed"
)

// Chat 是为一次 Turn 装配的模型调用。
type Chat struct {
	SessionID       string
	RunID           string
	TurnID          string
	Model           ModelConfig
	SystemPrompt    string
	Messages        []Message
	Tools           []tool.Definition
	MaxInputTokens  int64
	MaxOutputTokens int64
}

// ModelStreamEvent 是一条模型流事件。
type ModelStreamEvent struct {
	Type       ModelStreamEventType
	MessageID  string
	Delta      json.RawMessage
	OccurredAt time.Time
}

// ProviderUsage 是归一化前的供应商用量。
type ProviderUsage struct {
	RequestID                string
	Provider                 string
	Model                    string
	CacheCreationInputTokens int64
	CacheReadInputTokens     int64
	OutputTokens             int64
	ReasoningTokens          int64
	TotalTokens              int64
	Estimated                bool
	Raw                      json.RawMessage
}

// ModelStreamResult 是完成组装后的助手响应。
type ModelStreamResult struct {
	Message   Message
	ToolCalls []tool.Call
	Usage     ProviderUsage
}

// ModelStream 暴露模型增量与最终结果。
type ModelStream interface {
	Events() <-chan ModelStreamEvent
	Result(ctx context.Context) (ModelStreamResult, error)
	Close() error
}

// Stream 使用 Eino ToolCallingChatModel 发起流式调用。
// TODO: 创建 Eino 模型并发起流式调用。
func Stream(_ context.Context, _ Chat) (ModelStream, error) {
	var chat einomodel.ToolCallingChatModel
	_ = chat
	return nil, nil
}
