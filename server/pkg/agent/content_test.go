package agent

import (
	"encoding/json"
	"testing"

	"codedock/pkg/agent/tool"
)

func TestDecodeTextEmptyObject(t *testing.T) {
	if got := DecodeText(EncodeText("")); got != "" {
		t.Fatalf("empty text = %q", got)
	}
	if got := DecodeText([]byte(`{"text":""}`)); got != "" {
		t.Fatalf("empty json object = %q", got)
	}
	if got := DecodeText(EncodeText("hello")); got != "hello" {
		t.Fatalf("hello = %q", got)
	}
}

func TestCompleteToolResultsFillsMissingCalls(t *testing.T) {
	messages := []Message{
		{Role: RoleUser, Content: EncodeText("hi")},
		{
			Role:    RoleAssistant,
			Content: EncodeText("calling"),
			ToolCalls: []tool.Call{
				{ID: "c1", Name: "boom"},
				{ID: "c2", Name: "ping"},
			},
		},
		{Role: RoleTool, Content: EncodeText("boom exploded")},
		{Role: RoleUser, Content: EncodeText("again")},
	}
	got := CompleteToolResults(messages)
	if len(got) != 5 {
		t.Fatalf("len=%d", len(got))
	}
	var first ToolResultContent
	if err := json.Unmarshal(got[2].Content, &first); err != nil || first.CallID != "c1" {
		t.Fatalf("first tool = %+v err=%v", first, err)
	}
	var second ToolResultContent
	if err := json.Unmarshal(got[3].Content, &second); err != nil || second.CallID != "c2" {
		t.Fatalf("second tool = %+v err=%v", second, err)
	}
	if got[4].Role != RoleUser {
		t.Fatalf("last role = %s", got[4].Role)
	}
}
