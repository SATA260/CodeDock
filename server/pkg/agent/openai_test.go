package agent

import (
	"encoding/json"
	"strings"
	"testing"

	"codedock/pkg/agent/tool"
)

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
