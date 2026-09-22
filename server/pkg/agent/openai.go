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

	"log/slog"

	"codedock/pkg/agent/seam"
	"codedock/pkg/agent/tool"
)

type openaiOptions struct {
	APIKey   string `json:"api_key"`
	BaseURL  string `json:"base_url"`
	Thinking string `json:"thinking"`
}

type openaiThinking struct {
	Type string `json:"type"`
}

type openaiChatRequest struct {
	Model               string              `json:"model"`
	Stream              bool                `json:"stream"`
	Messages            []openaiChatMessage `json:"messages"`
	Tools               []openaiTool        `json:"tools,omitempty"`
	MaxTokens           int64               `json:"max_tokens,omitempty"`
	MaxCompletionTokens int64               `json:"max_completion_tokens,omitempty"`
	Thinking            *openaiThinking     `json:"thinking,omitempty"`
}

type openaiChatMessage struct {
	Role             string           `json:"role"`
	Content          string           `json:"content,omitempty"`
	ReasoningContent string           `json:"reasoning_content,omitempty"`
	ToolCalls        []openaiToolCall `json:"tool_calls,omitempty"`
	ToolCallID       string           `json:"tool_call_id,omitempty"`
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
	Index    int    `json:"index"`
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
			Content          string           `json:"content"`
			ReasoningContent string           `json:"reasoning_content"`
			Reasoning        string           `json:"reasoning"`
			ToolCalls        []openaiToolCall `json:"tool_calls"`
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

	reqBody := openaiChatRequest{
		Model:    chat.Model.Model,
		Stream:   true,
		Messages: toOpenAIMessages(chat),
		Tools:    toOpenAITools(chat.Tools),
	}
	applyOutputLimit(&reqBody, chat.Model.Model, chat.MaxOutputTokens)
	if opts.Thinking != "" && supportsThinking(base) {
		reqBody.Thinking = &openaiThinking{Type: opts.Thinking}
	}
	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}
	headers := map[string]string{
		"Authorization": "Bearer " + opts.APIKey,
		"Content-Type":  "application/json",
		"Accept":        "text/event-stream",
	}
	body, headers = applyStreamSeam(ctx, chat, body, headers)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	for key, value := range headers {
		req.Header.Set(key, value)
	}

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

// applyStreamSeam 在真正发出 HTTP 前递请求头和请求体；出错保留原数据。
func applyStreamSeam(ctx context.Context, chat Chat, body json.RawMessage, headers map[string]string) (json.RawMessage, map[string]string) {
	ev, err := seam.Dispatch(ctx, chat.Dispatcher, seam.Envelope{
		Type:      seam.TypeStream,
		SessionID: chat.SessionID,
		RunID:     chat.RunID,
		TurnID:    chat.TurnID,
		Payload:   MarshalPayload(StreamPayload{Headers: headers, Body: body}),
	})
	if err != nil {
		slog.Warn("llm/stream dispatch failed", "run_id", chat.RunID, "error", err)
		return body, headers
	}
	if ev.Type != seam.TypeStream || len(ev.Payload) == 0 {
		return body, headers
	}
	var payload StreamPayload
	if err := json.Unmarshal(ev.Payload, &payload); err != nil {
		return body, headers
	}
	if payload.Headers != nil {
		for key, value := range payload.Headers {
			headers[key] = value
		}
	}
	if len(payload.Body) > 0 {
		body = payload.Body
	}
	return body, headers
}

func applyOutputLimit(req *openaiChatRequest, model string, n int64) {
	if req == nil || n <= 0 {
		return
	}
	if isReasoningModel(model) {
		req.MaxCompletionTokens = n
		return
	}
	req.MaxTokens = n
}

func isReasoningModel(model string) bool {
	m := strings.ToLower(strings.TrimSpace(model))
	return strings.HasPrefix(m, "o1") || strings.HasPrefix(m, "o3") || strings.HasPrefix(m, "o4") || strings.Contains(m, "reasoner")
}

func supportsThinking(base string) bool {
	return strings.Contains(strings.ToLower(base), "deepseek")
}

// developerWireRole 把本轮模式消息映射成网关能接受的 role。
// 官方 OpenAI 用 developer；DeepSeek 等兼容网关只认 system / user / assistant / tool。
func developerWireRole(model ModelConfig) string {
	var opts openaiOptions
	_ = json.Unmarshal(model.Options, &opts)
	base := strings.ToLower(strings.TrimRight(strings.TrimSpace(opts.BaseURL), "/"))
	if base == "" || strings.Contains(base, "api.openai.com") || strings.Contains(base, "openai.azure.com") {
		return "developer"
	}
	return "system"
}

// reasoningDelta 取出本段思考；兼容 reasoning_content 与 reasoning 两种字段。
func reasoningDelta(content, alt string) string {
	if content != "" {
		return content
	}
	return alt
}

// consumeOpenAI 解析 SSE 增量，拼出最终文本、工具调用和用量后关闭流。
// 读完后若请求已取消，把空输出当成流失败，避免打断被记成一次成功的空回复。
func consumeOpenAI(ctx context.Context, chat Chat, body io.ReadCloser, stream *staticStream) {
	defer close(stream.done)
	defer close(stream.events)
	defer body.Close()

	now := time.Now().UTC()
	stream.events <- ModelStreamEvent{Type: ModelStreamStarted, OccurredAt: now}

	var text strings.Builder
	var reasoning strings.Builder
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
			if piece := reasoningDelta(choice.Delta.ReasoningContent, choice.Delta.Reasoning); piece != "" {
				reasoning.WriteString(piece)
				stream.events <- ModelStreamEvent{
					Type:       ModelStreamReasoningDelta,
					Delta:      MarshalPayload(ReasoningDelta{Reasoning: piece}),
					OccurredAt: time.Now().UTC(),
				}
			}
			if choice.Delta.Content != "" {
				text.WriteString(choice.Delta.Content)
				stream.events <- ModelStreamEvent{
					Type:       ModelStreamTextDelta,
					Delta:      MarshalPayload(TextContent{Text: choice.Delta.Content}),
					OccurredAt: time.Now().UTC(),
				}
			}
			for _, item := range choice.Delta.ToolCalls {
				before := len(calls)
				var prevName string
				if item.Index >= 0 && item.Index < len(calls) {
					prevName = calls[item.Index].Name
				}
				calls = mergeToolDelta(calls, item, chat.TurnID)
				if item.Index < 0 || item.Index >= len(calls) {
					continue
				}
				call := calls[item.Index]
				if prevName != "" && item.ID == "" && len(calls) == before {
					continue
				}
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
	if err := ctx.Err(); err != nil {
		stream.err = err
		return
	}
	if usage.TotalTokens == 0 {
		usage.OutputTokens = CountTokens(text.String())
		usage.TotalTokens = usage.OutputTokens + CountTokens(chat.SystemPrompt)
		usage.Estimated = true
	}
	for i := range calls {
		if len(calls[i].Arguments) == 0 {
			calls[i].Arguments = json.RawMessage("{}")
		}
		if calls[i].ID == "" {
			calls[i].ID = fmt.Sprintf("call_%s_%d", chat.TurnID, i+1)
		}
	}
	stream.result = ModelStreamResult{
		Message: Message{
			Role:      RoleAssistant,
			Content:   EncodeText(text.String()),
			ToolCalls: calls,
		},
		ToolCalls: calls,
		Reasoning: reasoning.String(),
		Usage:     usage,
	}
	stream.events <- ModelStreamEvent{Type: ModelStreamCompleted, OccurredAt: time.Now().UTC()}
}

// toOpenAIMessages 把系统提示、历史消息和工具结果映射成 OpenAI chat 消息。
// 模式规则紧跟底座 system：DeepSeek 等网关会丢掉插在对话历史后面的 system。
// 已落库的空助手消息（无正文且无 tool_calls）在这里丢掉，避免下一轮被网关拒绝。
func toOpenAIMessages(chat Chat) []openaiChatMessage {
	var mode *openaiChatMessage
	rest := make([]openaiChatMessage, 0, len(chat.Messages))
	for _, msg := range chat.Messages {
		switch msg.Role {
		case RoleAssistant:
			if assistantBlank(msg.Content, msg.ToolCalls) {
				continue
			}
			item := openaiChatMessage{
				Role:             "assistant",
				Content:          DecodeText(msg.Content),
				ReasoningContent: DecodeReasoning(msg.Content),
			}
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
			rest = append(rest, item)
		case RoleTool:
			var result ToolResultContent
			_ = json.Unmarshal(msg.Content, &result)
			content := string(result.Output)
			if content == "" {
				content = DecodeText(msg.Content)
			}
			rest = append(rest, openaiChatMessage{Role: "tool", Content: content, ToolCallID: result.CallID})
		case RoleSystem:
			rest = append(rest, openaiChatMessage{Role: "system", Content: DecodeText(msg.Content)})
		case RoleDeveloper:
			item := openaiChatMessage{Role: developerWireRole(chat.Model), Content: DecodeText(msg.Content)}
			mode = &item
		default:
			rest = append(rest, openaiChatMessage{Role: "user", Content: DecodeText(msg.Content)})
		}
	}
	messages := make([]openaiChatMessage, 0, len(rest)+2)
	if chat.SystemPrompt != "" {
		messages = append(messages, openaiChatMessage{Role: "system", Content: chat.SystemPrompt})
	}
	if mode != nil {
		messages = append(messages, *mode)
	}
	return append(messages, rest...)
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

// mergeToolDelta 按 index 合并流式 tool_calls 增量，避免每段都当成新调用。
func mergeToolDelta(calls []tool.Call, item openaiToolCall, turnID string) []tool.Call {
	idx := item.Index
	if idx < 0 {
		idx = 0
	}
	for len(calls) <= idx {
		calls = append(calls, tool.Call{Attempt: 1})
	}
	call := calls[idx]
	if item.ID != "" {
		call.ID = item.ID
	}
	if item.Function.Name != "" {
		call.Name = item.Function.Name
	}
	if item.Function.Arguments != "" {
		call.Arguments = append(append(json.RawMessage(nil), call.Arguments...), item.Function.Arguments...)
	}
	if call.ID == "" {
		call.ID = fmt.Sprintf("call_%s_%d", turnID, idx+1)
	}
	calls[idx] = call
	return calls
}

// PublicModelError 抽出可给用户看的模型失败原因。
func PublicModelError(err error) string {
	if err == nil {
		return ""
	}
	text := strings.TrimSpace(err.Error())
	if msg := openaiErrorMessage(text); msg != "" {
		return clipPublicError(msg)
	}
	if _, rest, ok := strings.Cut(text, "openai status "); ok {
		if _, body, found := strings.Cut(rest, ":"); found {
			body = strings.TrimSpace(body)
			if body != "" {
				return clipPublicError(body)
			}
		}
	}
	return clipPublicError(text)
}

// openaiErrorMessage 从网关 JSON 里取出 error.message。
func openaiErrorMessage(text string) string {
	start := strings.Index(text, "{")
	if start < 0 {
		return ""
	}
	var payload struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
		Message string `json:"message"`
	}
	if json.Unmarshal([]byte(text[start:]), &payload) != nil {
		return ""
	}
	if strings.TrimSpace(payload.Error.Message) != "" {
		return payload.Error.Message
	}
	return strings.TrimSpace(payload.Message)
}

// clipPublicError 把失败原因压到状态行能放下的长度。
func clipPublicError(text string) string {
	text = strings.Join(strings.Fields(text), " ")
	runes := []rune(text)
	if len(runes) <= 180 {
		return text
	}
	return string(runes[:177]) + "..."
}
