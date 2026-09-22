package agent

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"codedock/internal/config"
	"codedock/pkg/agent/tool"
)

// liveModelFromDotEnv 用仓库 .env 里的真实供应商、模型和地址组装调用配置。
func liveModelFromDotEnv(t *testing.T) (ModelConfig, config.Config) {
	t.Helper()
	if err := config.LoadDotEnv(); err != nil {
		t.Fatalf("load .env: %v", err)
	}
	cfg := config.Load()
	if strings.TrimSpace(cfg.LLMAPIKey) == "" || strings.EqualFold(cfg.LLMProvider, "fake") || strings.TrimSpace(cfg.LLMProvider) == "" {
		t.Skip("live test needs LLM_API_KEY and a non-fake LLM_PROVIDER in .env")
	}
	if !strings.EqualFold(cfg.LLMProvider, "openai") {
		t.Skipf("live wire test uses the openai-compatible client, provider=%s", cfg.LLMProvider)
	}
	opts, err := json.Marshal(map[string]string{
		"api_key":  cfg.LLMAPIKey,
		"base_url": cfg.LLMBaseURL,
	})
	if err != nil {
		t.Fatal(err)
	}
	return ModelConfig{Provider: cfg.LLMProvider, Model: cfg.LLMModel, Options: opts}, cfg
}

// TestLiveBlankAssistantWire 用 .env 的真实网关确认：空助手消息会被拒绝，过滤后再发则能通过。
func TestLiveBlankAssistantWire(t *testing.T) {
	model, cfg := liveModelFromDotEnv(t)
	history := []Message{
		{Role: RoleUser, Content: EncodeText("hi")},
		{Role: RoleAssistant, Content: EncodeText(""), ToolCalls: []tool.Call{{ID: "call_1", Name: "ping", Arguments: json.RawMessage(`{"path":"."}`)}}},
		{Role: RoleTool, Content: EncodeToolResult("call_1", json.RawMessage(`"ok"`))},
		{Role: RoleAssistant, Content: EncodeText("")},
		{Role: RoleUser, Content: EncodeText("Reply with the single word pong")},
	}
	wired := toOpenAIMessages(Chat{Model: model, Messages: history})
	for _, msg := range wired {
		if msg.Role == "assistant" && strings.TrimSpace(msg.Content) == "" && len(msg.ToolCalls) == 0 {
			t.Fatalf("blank assistant still on the wire: %+v", wired)
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	stream, err := Stream(ctx, Chat{Model: model, TurnID: "live-blank", Messages: history})
	if err != nil {
		t.Fatal(PublicModelError(err))
	}
	defer stream.Close()
	got, err := stream.Result(ctx)
	if err != nil {
		t.Fatal(PublicModelError(err))
	}
	text := strings.TrimSpace(DecodeText(got.Message.Content))
	if text == "" && len(got.ToolCalls) == 0 {
		t.Fatal("model returned a blank assistant")
	}
	t.Logf("model=%s reply=%q", cfg.LLMModel, clipPublicError(text))

	status, message := postRawAssistant(t, cfg, openaiChatRequest{
		Model:  cfg.LLMModel,
		Stream: true,
		Messages: []openaiChatMessage{
			{Role: "user", Content: "hi"},
			{Role: "assistant"},
			{Role: "user", Content: "Reply with the single word pong"},
		},
	})
	if status < 400 || !strings.Contains(message, "Invalid assistant message") {
		t.Fatalf("unfiltered blank assistant status=%d message=%s", status, message)
	}
}

// postRawAssistant 把未过滤的消息直接发给 .env 里的网关，用来对照空助手消息会被拒绝。
func postRawAssistant(t *testing.T, cfg config.Config, payload openaiChatRequest) (int, string) {
	t.Helper()
	base := strings.TrimRight(cfg.LLMBaseURL, "/")
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/chat/completions", strings.NewReader(string(body)))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+cfg.LLMAPIKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	return resp.StatusCode, PublicModelError(errString(string(raw)))
}

// errString 包一层 error，供 PublicModelError 抽出网关消息。
type errString string

// Error 返回网关原文。
func (e errString) Error() string { return string(e) }
