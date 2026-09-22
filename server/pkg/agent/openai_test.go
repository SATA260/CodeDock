package agent

import (
	"codedock/pkg/agent/tool"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestDeveloperWireRole(t *testing.T) {
	if developerWireRole(ModelConfig{}) != "developer" {
		t.Fatal("empty base should use official openai developer")
	}
	openaiOpts, _ := json.Marshal(map[string]string{"base_url": "https://api.openai.com/v1"})
	if developerWireRole(ModelConfig{Options: openaiOpts}) != "developer" {
		t.Fatal("openai.com")
	}
	deepseekOpts, _ := json.Marshal(map[string]string{"base_url": "https://api.deepseek.com"})
	if developerWireRole(ModelConfig{Options: deepseekOpts}) != "system" {
		t.Fatal("deepseek should fall back to system")
	}
	msgs := toOpenAIMessages(Chat{
		SystemPrompt: "base",
		Model:        ModelConfig{Options: deepseekOpts},
		Messages: []Message{
			{Role: RoleUser, Content: EncodeText("hi")},
			{Role: RoleDeveloper, Content: EncodeText("mode")},
		},
	})
	if len(msgs) != 3 || msgs[0].Role != "system" || msgs[0].Content != "base" || msgs[1].Role != "system" || msgs[1].Content != "mode" || msgs[2].Role != "user" {
		t.Fatalf("%+v", msgs)
	}
}

func TestApplyOutputLimitAndThinkingSupport(t *testing.T) {
	var chat openaiChatRequest
	applyOutputLimit(&chat, "deepseek-v4-flash", 320)
	if chat.MaxTokens != 320 || chat.MaxCompletionTokens != 0 {
		t.Fatalf("compat model: %+v", chat)
	}
	chat = openaiChatRequest{}
	applyOutputLimit(&chat, "o3-mini", 320)
	if chat.MaxCompletionTokens != 320 || chat.MaxTokens != 0 {
		t.Fatalf("reasoning model: %+v", chat)
	}
	if supportsThinking("https://api.openai.com/v1") || !supportsThinking("https://api.deepseek.com") {
		t.Fatal("thinking support")
	}
}

func TestOpenAIRequestDisablesThinking(t *testing.T) {
	body, err := json.Marshal(openaiChatRequest{
		Model:     "deepseek-v4-flash",
		Stream:    true,
		MaxTokens: 96,
		Thinking:  &openaiThinking{Type: "disabled"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), `"thinking":{"type":"disabled"}`) {
		t.Fatalf("body %s", body)
	}
	if !strings.Contains(string(body), `"max_tokens":96`) {
		t.Fatalf("body %s", body)
	}
}

// TestMergeToolDelta 校验流式 tool_calls 按 index 拼成一条完整调用。
func TestMergeToolDelta(t *testing.T) {
	t.Parallel()
	var calls []tool.Call
	calls = mergeToolDelta(calls, openaiToolCall{
		Index: 0,
		ID:    "call_1",
		Function: struct {
			Name      string `json:"name"`
			Arguments string `json:"arguments"`
		}{Name: "ping", Arguments: ""},
	}, "turn")
	calls = mergeToolDelta(calls, openaiToolCall{
		Index: 0,
		Function: struct {
			Name      string `json:"name"`
			Arguments string `json:"arguments"`
		}{Arguments: `{"ok"`},
	}, "turn")
	calls = mergeToolDelta(calls, openaiToolCall{
		Index: 0,
		Function: struct {
			Name      string `json:"name"`
			Arguments string `json:"arguments"`
		}{Arguments: `:true}`},
	}, "turn")
	if len(calls) != 1 {
		t.Fatalf("len(calls)=%d want 1", len(calls))
	}
	if calls[0].ID != "call_1" || calls[0].Name != "ping" {
		t.Fatalf("call = %+v", calls[0])
	}
	if string(calls[0].Arguments) != `{"ok":true}` {
		t.Fatalf("arguments = %s", calls[0].Arguments)
	}
	if !json.Valid(calls[0].Arguments) {
		t.Fatal("arguments are not valid json")
	}
}

func TestStreamOpenAI(t *testing.T) {
	t.Run("missing key", func(t *testing.T) {
		if _, err := Stream(context.Background(), Chat{Model: ModelConfig{Provider: "openai", Model: "gpt"}}); err == nil {
			t.Fatal("expected key error")
		}
	})
	t.Run("status error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadGateway)
			_, _ = w.Write([]byte("nope"))
		}))
		defer server.Close()
		opts, _ := json.Marshal(map[string]string{"api_key": "k", "base_url": server.URL})
		if _, err := Stream(context.Background(), Chat{Model: ModelConfig{Provider: "openai", Model: "gpt", Options: opts}}); err == nil {
			t.Fatal("expected status error")
		}
	})
	t.Run("sse text and tools", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = w.Write([]byte("\n"))
			_, _ = w.Write([]byte("data: not-json\n\n"))
			_, _ = w.Write([]byte("data: {\"id\":\"req1\",\"choices\":[{\"delta\":{\"content\":\"hi\"}}]}\n\n"))
			_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"id\":\"c1\",\"function\":{\"name\":\"ping\",\"arguments\":\"{}\"}}]}}],\"usage\":{\"prompt_tokens\":1,\"completion_tokens\":2,\"total_tokens\":3}}\n\n"))
			_, _ = w.Write([]byte("data: [DONE]\n\n"))
		}))
		defer server.Close()
		opts, _ := json.Marshal(map[string]string{"api_key": "k", "base_url": server.URL, "thinking": "disabled"})
		stream, err := Stream(context.Background(), Chat{
			TurnID:          "t1",
			Model:           ModelConfig{Provider: "openai", Model: "o3-mini", Options: opts},
			SystemPrompt:    "sys",
			Messages:        []Message{{Role: RoleUser, Content: EncodeText("q")}},
			Tools:           []tool.Definition{{Name: "ping", Prompt: "p"}},
			MaxOutputTokens: 32,
		})
		if err != nil {
			t.Fatal(err)
		}
		defer stream.Close()
		for range stream.Events() {
		}
		got, err := stream.Result(context.Background())
		if err != nil || DecodeText(got.Message.Content) != "hi" || len(got.ToolCalls) != 1 || got.Usage.TotalTokens != 3 {
			t.Fatalf("%+v %v", got, err)
		}
	})
	t.Run("sse reasoning and text", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"reasoning_content\":\"think \"}}]}\n\n"))
			_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"reasoning_content\":\"first\"}}]}\n\n"))
			_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"pong\"}}]}\n\n"))
			_, _ = w.Write([]byte("data: [DONE]\n\n"))
		}))
		defer server.Close()
		opts, _ := json.Marshal(map[string]string{"api_key": "k", "base_url": server.URL})
		stream, err := Stream(context.Background(), Chat{
			TurnID: "t-reason",
			Model:  ModelConfig{Provider: "openai", Model: "gpt", Options: opts},
		})
		if err != nil {
			t.Fatal(err)
		}
		defer stream.Close()
		var kinds []string
		for event := range stream.Events() {
			kinds = append(kinds, string(event.Type))
		}
		got, err := stream.Result(context.Background())
		if err != nil || DecodeText(got.Message.Content) != "pong" || got.Reasoning != "think first" {
			t.Fatalf("%+v %v kinds=%v", got, err, kinds)
		}
		if len(kinds) < 3 || kinds[1] != string(ModelStreamReasoningDelta) {
			t.Fatalf("kinds=%v", kinds)
		}
	})
	t.Run("http error", func(t *testing.T) {
		opts, _ := json.Marshal(map[string]string{"api_key": "k", "base_url": "http://127.0.0.1:1"})
		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()
		if _, err := Stream(ctx, Chat{Model: ModelConfig{Provider: "openai", Model: "gpt", Options: opts}}); err == nil {
			t.Fatal("expected dial error")
		}
	})
}

func TestCompactWithOpenAI(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"short\"}}]}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer server.Close()
	opts, _ := json.Marshal(map[string]string{"api_key": "k", "base_url": server.URL})
	model := ModelConfig{Provider: "openai", Model: "gpt", Options: opts}
	got, err := CompactIfNeeded(context.Background(), Compaction{
		Run:      Run{Config: RunConfigSnapshot{Model: model, Limits: RunLimits{MaxInputTokens: 1}}},
		Snapshot: ContextSnapshot{Messages: []Message{{Role: RoleUser, Content: EncodeText(stringsRepeat("z", 20)), EventSeq: 1}}},
	})
	if err != nil || got.Summary == nil || got.Summary.Content != "short" {
		t.Fatalf("%+v %v", got, err)
	}
	idx, err := CompactIndex(context.Background(), model, "# index")
	if err != nil || idx != "short" {
		t.Fatal(idx, err)
	}
}

func TestPublicModelError(t *testing.T) {
	got := PublicModelError(fmt.Errorf(`openai status 503: {"error":{"message":"Service is too busy.","type":"service_unavailable_error"}}`))
	if got != "Service is too busy." {
		t.Fatalf("got %q", got)
	}
	if PublicModelError(nil) != "" {
		t.Fatal("nil")
	}
}

func TestStreamWithRetryThenOK(t *testing.T) {
	hits := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		if hits < 2 {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"error":{"message":"Service is too busy."}}`))
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"ok\"}}]}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer server.Close()
	opts, _ := json.Marshal(map[string]string{"api_key": "k", "base_url": server.URL})
	stream, err := streamWithRetry(context.Background(), Chat{
		Model: ModelConfig{Provider: "openai", Model: "gpt", Options: opts},
	}, RetryConfig{MaxAttempts: 3, InitialBackoff: time.Millisecond, MaxBackoff: 5 * time.Millisecond, Multiplier: 2})
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()
	result, err := stream.Result(context.Background())
	if err != nil || DecodeText(result.Message.Content) != "ok" || hits != 2 {
		t.Fatalf("result=%+v err=%v hits=%d", result, err, hits)
	}
}

// TestToOpenAIMessagesSkipsBlankAssistant 确认空助手消息不进网关，带 tool_calls 的空正文仍保留。
func TestToOpenAIMessagesSkipsBlankAssistant(t *testing.T) {
	msgs := toOpenAIMessages(Chat{
		Messages: []Message{
			{Role: RoleAssistant, Content: EncodeText("")},
			{Role: RoleAssistant, Content: EncodeText("   ")},
			{Role: RoleAssistant, Content: EncodeText(""), ToolCalls: []tool.Call{{ID: "c1", Name: "ping", Arguments: json.RawMessage(`{}`)}}},
			{Role: RoleUser, Content: EncodeText("go")},
		},
	})
	if len(msgs) != 2 || msgs[0].Role != "assistant" || msgs[0].Content != "" || len(msgs[0].ToolCalls) != 1 || msgs[1].Role != "user" {
		t.Fatalf("%+v", msgs)
	}
}

// TestToOpenAIMessagesSendsReasoningContent 确认思考回传在 reasoning_content，不拼进 content。
func TestToOpenAIMessagesSendsReasoningContent(t *testing.T) {
	msgs := toOpenAIMessages(Chat{
		Messages: []Message{
			{Role: RoleAssistant, Content: EncodeTextContent("pong", "think first"), ToolCalls: []tool.Call{{ID: "c1", Name: "ping", Arguments: json.RawMessage(`{}`)}}},
			{Role: RoleUser, Content: EncodeText("again")},
		},
	})
	if len(msgs) != 2 || msgs[0].Content != "pong" || msgs[0].ReasoningContent != "think first" || msgs[1].Role != "user" {
		t.Fatalf("%+v", msgs)
	}
}

// TestConsumeOpenAICancelAfterCleanEOF 确认请求已取消时，干净结束的空流不算成功回复。
func TestConsumeOpenAICancelAfterCleanEOF(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	stream := &staticStream{events: make(chan ModelStreamEvent, 4), done: make(chan struct{})}
	go consumeOpenAI(ctx, Chat{Model: ModelConfig{Model: "m"}}, io.NopCloser(strings.NewReader("")), stream)
	for range stream.Events() {
	}
	if _, err := stream.Result(context.Background()); err == nil {
		t.Fatal("cancelled empty stream should fail")
	}
}

func stringsRepeat(s string, n int) string {
	out := make([]byte, 0, len(s)*n)
	for i := 0; i < n; i++ {
		out = append(out, s...)
	}
	return string(out)
}
