package agent

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"codedock/pkg/agent/tool"
)

type openaiOptions struct {
	APIKey  string `json:"api_key"`
	BaseURL string `json:"base_url"`
}

type openaiChatRequest struct {
	Model    string              `json:"model"`
	Stream   bool                `json:"stream"`
	Messages []openaiChatMessage `json:"messages"`
	Tools    []openaiTool        `json:"tools,omitempty"`
}

type openaiChatMessage struct {
	Role       string              `json:"role"`
	Content    string              `json:"content,omitempty"`
	ToolCalls  []openaiToolCall    `json:"tool_calls,omitempty"`
	ToolCallID string              `json:"tool_call_id,omitempty"`
}

type openaiTool struct {
	Type     string `json:"type"`
	Function struct {
		Name        string          `json:"name"`
		Description string          `json:"description"`
		Parameters  json.RawMessage `json:"parameters"`
	} `json:"function"`
}

type openaiToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

type openaiStreamChunk struct {
	ID      string `json:"id"`
	Choices []struct {
		Delta struct {
			Content   string           `json:"content"`
			ToolCalls []openaiToolCall `json:"tool_calls"`
		} `json:"delta"`
	} `json:"choices"`
	Usage *struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
}

// streamOpenAI 按 ModelConfig 发起 OpenAI 兼容的流式 chat/completions 请求。
func streamOpenAI(ctx context.Context, chat Chat) (ModelStream, error) {
	var opts openaiOptions
	_ = json.Unmarshal(chat.Model.Options, &opts)
	if opts.APIKey == "" {
		return nil, fmt.Errorf("%w: openai api key is required", ErrNonRetryable)
	}
	base := strings.TrimRight(opts.BaseURL, "/")
	if base == "" {
		base = "https://api.openai.com/v1"
	}

	body, err := json.Marshal(openaiChatRequest{
		Model:    chat.Model.Model,
		Stream:   true,
		Messages: toOpenAIMessages(chat),
		Tools:    toOpenAITools(chat.Tools),
	})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+opts.APIKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 300 {
		payload, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		return nil, fmt.Errorf("openai status %d: %s", resp.StatusCode, payload)
	}

	ctx, cancel := context.WithCancel(ctx)
	stream := &staticStream{
		events: make(chan ModelStreamEvent, 16),
		done:   make(chan struct{}),
		cancel: func() {
			cancel()
			_ = resp.Body.Close()
		},
	}
	go consumeOpenAI(ctx, chat, resp.Body, stream)
	return stream, nil
}

// consumeOpenAI 解析 SSE 增量，拼出最终文本、工具调用和用量后关闭流。
func consumeOpenAI(ctx context.Context, chat Chat, body io.ReadCloser, stream *staticStream) {
	defer close(stream.done)
	defer close(stream.events)
	defer body.Close()

	now := time.Now().UTC()
	stream.events <- ModelStreamEvent{Type: ModelStreamStarted, OccurredAt: now}

	var text strings.Builder
	var calls []tool.Call
	usage := ProviderUsage{Provider: "openai", Model: chat.Model.Model, RequestID: chat.TurnID}
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		if err := ctx.Err(); err != nil {
			stream.err = err
			return
		}
		line := strings.TrimSpace(scanner.Text())
		if line == "" || !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "[DONE]" {
			break
		}
		var chunk openaiStreamChunk
		if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
			continue
		}
		if chunk.ID != "" {
			usage.RequestID = chunk.ID
		}
		for _, choice := range chunk.Choices {
			if choice.Delta.Content != "" {
				text.WriteString(choice.Delta.Content)
				stream.events <- ModelStreamEvent{
					Type:       ModelStreamTextDelta,
					Delta:      MarshalPayload(TextContent{Text: choice.Delta.Content}),
					OccurredAt: time.Now().UTC(),
				}
			}
			for _, item := range choice.Delta.ToolCalls {
				call := tool.Call{
					ID:        item.ID,
					Name:      item.Function.Name,
					Arguments: json.RawMessage(item.Function.Arguments),
					Attempt:   1,
				}
				if call.ID == "" {
					call.ID = fmt.Sprintf("call_%s_%d", chat.TurnID, len(calls)+1)
				}
				if len(call.Arguments) == 0 {
					call.Arguments = json.RawMessage("{}")
				}
				calls = append(calls, call)
				stream.events <- ModelStreamEvent{
					Type:       ModelStreamToolDelta,
					Delta:      MarshalPayload(call),
					OccurredAt: time.Now().UTC(),
				}
			}
		}
		if chunk.Usage != nil {
			usage.CacheReadInputTokens = int64(chunk.Usage.PromptTokens)
			usage.OutputTokens = int64(chunk.Usage.CompletionTokens)
			usage.TotalTokens = int64(chunk.Usage.TotalTokens)
		}
	}
	if err := scanner.Err(); err != nil && stream.err == nil {
		stream.err = err
		return
	}
	if usage.TotalTokens == 0 {
		usage.OutputTokens = CountTokens(text.String())
		usage.TotalTokens = usage.OutputTokens + CountTokens(chat.SystemPrompt)
		usage.Estimated = true
	}
	stream.result = ModelStreamResult{
		Message: Message{
			Role:      RoleAssistant,
			Content:   EncodeText(text.String()),
			ToolCalls: calls,
		},
		ToolCalls: calls,
		Usage:     usage,
	}
	stream.events <- ModelStreamEvent{Type: ModelStreamCompleted, OccurredAt: time.Now().UTC()}
}

// toOpenAIMessages 把系统提示、历史消息和工具结果映射成 OpenAI chat 消息。
func toOpenAIMessages(chat Chat) []openaiChatMessage {
	messages := make([]openaiChatMessage, 0, len(chat.Messages)+1)
	if chat.SystemPrompt != "" {
		messages = append(messages, openaiChatMessage{Role: "system", Content: chat.SystemPrompt})
	}
	for _, msg := range chat.Messages {
		switch msg.Role {
		case RoleAssistant:
			item := openaiChatMessage{Role: "assistant", Content: DecodeText(msg.Content)}
			for _, call := range msg.ToolCalls {
				item.ToolCalls = append(item.ToolCalls, openaiToolCall{
					ID:   call.ID,
					Type: "function",
					Function: struct {
						Name      string `json:"name"`
						Arguments string `json:"arguments"`
					}{Name: call.Name, Arguments: string(call.Arguments)},
				})
			}
			messages = append(messages, item)
		case RoleTool:
			var result ToolResultContent
			_ = json.Unmarshal(msg.Content, &result)
			content := string(result.Output)
			if content == "" {
				content = DecodeText(msg.Content)
			}
			messages = append(messages, openaiChatMessage{Role: "tool", Content: content, ToolCallID: result.CallID})
		case RoleSystem:
			messages = append(messages, openaiChatMessage{Role: "system", Content: DecodeText(msg.Content)})
		default:
			messages = append(messages, openaiChatMessage{Role: "user", Content: DecodeText(msg.Content)})
		}
	}
	return messages
}

// toOpenAITools 把工具定义映射成 OpenAI function tools。
func toOpenAITools(defs []tool.Definition) []openaiTool {
	if len(defs) == 0 {
		return nil
	}
	out := make([]openaiTool, 0, len(defs))
	for _, def := range defs {
		item := openaiTool{Type: "function"}
		item.Function.Name = def.Name
		item.Function.Description = def.Prompt
		item.Function.Parameters = def.ParametersSchema
		if len(item.Function.Parameters) == 0 {
			item.Function.Parameters = json.RawMessage(`{"type":"object","properties":{}}`)
		}
		out = append(out, item)
	}
	return out
}
