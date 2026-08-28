package agent

import (
	"encoding/json"
	"testing"

	"codedock/pkg/agent/tool"
)

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
