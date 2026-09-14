package agent

import (
	"context"
	"strings"
	"testing"

	"codedock/pkg/agent/tool"
)

// TestComposeSystemPrompt 校验静态提示与工具自带描述拼接，且默认提示不含具体工具名。
func TestComposeSystemPrompt(t *testing.T) {
	t.Parallel()
	if strings.Contains(DefaultSystemPrompt, "ping") || strings.Contains(DefaultSystemPrompt, "memory_read") {
		t.Fatal("default system prompt must not name tools")
	}
	if got := ComposeSystemPrompt("  base  ", nil); got != "base" {
		t.Fatalf("empty tools: %q", got)
	}
	got := ComposeSystemPrompt("base", []tool.Definition{
		{Name: "ping", Prompt: "连通性检查。"},
		{Name: "skip", Prompt: ""},
		{Name: "memory_read", Prompt: "先读再写。"},
	})
	want := "base\n\n工具：\n- ping：连通性检查。\n- memory_read：先读再写。"
	if got != want {
		t.Fatalf("got=%q want=%q", got, want)
	}
}

// TestBuildJoinsToolPrompts 校验 Build 把可见工具描述写入本次调用的系统提示。
func TestBuildJoinsToolPrompts(t *testing.T) {
	t.Parallel()
	chat, err := Build(context.Background(), Prompt{
		Context: ContextSnapshot{
			SystemPrompt: DefaultSystemPrompt,
			Tools: []tool.Definition{
				{Name: "ping", Prompt: "连通性检查，无参数，返回 ok。"},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(chat.SystemPrompt, DefaultSystemPrompt) {
		t.Fatalf("missing base prompt: %q", chat.SystemPrompt)
	}
	if !strings.Contains(chat.SystemPrompt, "- ping：连通性检查，无参数，返回 ok。") {
		t.Fatalf("missing tool prompt: %q", chat.SystemPrompt)
	}
}

func TestBuildInsertsHiddenAfterMemory(t *testing.T) {
	t.Parallel()
	chat, err := Build(context.Background(), Prompt{
		Context: ContextSnapshot{
			SystemPrompt:  "base",
			MemoryIndexes: []string{"index-note"},
			Hidden:        []Message{{Role: RoleSystem, Content: EncodeText("hidden-note")}},
			Messages:      []Message{{Role: RoleUser, Content: EncodeText("hi")}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(chat.Messages) < 3 {
		t.Fatalf("messages=%d", len(chat.Messages))
	}
	if DecodeText(chat.Messages[0].Content) != "index-note" || chat.Messages[0].Role != RoleSystem {
		t.Fatalf("memory first: %+v", chat.Messages[0])
	}
	if DecodeText(chat.Messages[1].Content) != "hidden-note" || chat.Messages[1].Role != RoleSystem {
		t.Fatalf("hidden second: %+v", chat.Messages[1])
	}
}
