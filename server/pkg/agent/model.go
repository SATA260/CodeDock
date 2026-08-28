package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"codedock/pkg/agent/tool"
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
	Attempt         int
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

// Stream 使用 ModelConfig 在本函数内创建模型并发起流式调用。
// fake 走脚本；openai 走兼容 HTTP SSE。不由 Runtime 注入模型实例。
func Stream(ctx context.Context, chat Chat) (ModelStream, error) {
	if chat.Attempt <= 0 {
		chat.Attempt = 1
	}
	switch strings.ToLower(chat.Model.Provider) {
	case "", "fake":
		return streamFake(ctx, chat)
	case "openai":
		return streamOpenAI(ctx, chat)
	default:
		return nil, fmt.Errorf("%w: unsupported model provider %q", ErrNonRetryable, chat.Model.Provider)
	}
}

type staticStream struct {
	events chan ModelStreamEvent
	done   chan struct{}
	result ModelStreamResult
	err    error
	cancel context.CancelFunc
}

// Events 返回模型流增量通道。
func (s *staticStream) Events() <-chan ModelStreamEvent {
	return s.events
}

// Result 等待流结束并返回组装后的助手消息与用量。
func (s *staticStream) Result(ctx context.Context) (ModelStreamResult, error) {
	select {
	case <-ctx.Done():
		return ModelStreamResult{}, ctx.Err()
	case <-s.done:
		return s.result, s.err
	}
}

// Close 取消底层流，可重复调用。
func (s *staticStream) Close() error {
	if s.cancel != nil {
		s.cancel()
	}
	return nil
}

// emitStream 把一段完整输出转成可取消的 ModelStream。
// hang 为真时只等 ctx 取消，用于测试取消路径；否则依次发出 started / delta / completed。
func emitStream(ctx context.Context, chat Chat, text string, calls []tool.Call, usage ProviderUsage, hang bool) ModelStream {
	ctx, cancel := context.WithCancel(ctx)
	stream := &staticStream{
		events: make(chan ModelStreamEvent, 8),
		done:   make(chan struct{}),
		cancel: cancel,
	}
	go func() {
		defer close(stream.done)
		defer close(stream.events)
		if hang {
			select {
			case <-ctx.Done():
				stream.err = ctx.Err()
			}
			return
		}
		now := time.Now().UTC()
		stream.events <- ModelStreamEvent{Type: ModelStreamStarted, OccurredAt: now}
		if text != "" {
			stream.events <- ModelStreamEvent{
				Type:       ModelStreamTextDelta,
				Delta:      MarshalPayload(TextContent{Text: text}),
				OccurredAt: now,
			}
		}
		for _, call := range calls {
			stream.events <- ModelStreamEvent{
				Type:       ModelStreamToolDelta,
				Delta:      MarshalPayload(call),
				OccurredAt: now,
			}
		}
		stream.events <- ModelStreamEvent{Type: ModelStreamCompleted, OccurredAt: now}
		stream.result = ModelStreamResult{
			Message: Message{
				Role:      RoleAssistant,
				Content:   EncodeText(text),
				ToolCalls: calls,
			},
			ToolCalls: calls,
			Usage:     usage,
		}
	}()
	return stream
}
