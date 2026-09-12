package agent

import (
	"context"
	"strings"
	"testing"

	"codedock/pkg/agent/profile"
	"codedock/pkg/agent/tool"
)

// TestComposeSystemPrompt 校验静态提示与工具自带描述按 pi 结构拼接，且默认提示不含具体工具名。
func TestComposeSystemPrompt(t *testing.T) {
	t.Parallel()
	if strings.Contains(DefaultSystemPrompt, "ping") || strings.Contains(DefaultSystemPrompt, "memory_read") {
		t.Fatal("default system prompt must not name tools")
	}
	empty := ComposeSystemPrompt("  base  ", nil)
	if !strings.HasPrefix(empty, "base\n\n可用工具：\n（无）\n") {
		t.Fatalf("empty tools: %q", empty)
	}
	if !strings.Contains(empty, "使用要求：\n- 用用户的语言回复") {
		t.Fatalf("missing shared guidelines: %q", empty)
	}
	if !strings.Contains(empty, "- 回复尽量简洁\n- 涉及文件时写清路径") {
		t.Fatalf("missing default guidelines: %q", empty)
	}
	got := ComposeSystemPrompt("base", []tool.Definition{
		{Name: "ping", Prompt: "连通性检查。"},
		{Name: "skip", Prompt: ""},
		{Name: "memory_read", Prompt: "先读再写。"},
	})
	if !strings.Contains(got, "可用工具：\n- ping: 连通性检查。\n- memory_read: 先读再写。") {
		t.Fatalf("tools: %q", got)
	}
	if strings.Contains(got, "- skip:") {
		t.Fatalf("empty snippet should be omitted: %q", got)
	}
}

func TestComposeSystemPromptShellGuidelines(t *testing.T) {
	t.Parallel()
	both := composeSystemPrompt("base", []tool.Definition{
		{Name: "bash", Prompt: "执行 bash。"},
		{Name: "powershell", Prompt: "执行 PowerShell。"},
	}, nil, "")
	if !strings.Contains(both, "用 bash 或 PowerShell 做列举、搜索、查找等文件操作") {
		t.Fatalf("both shells: %q", both)
	}
	ps := composeSystemPrompt("base", []tool.Definition{{Name: "powershell", Prompt: "执行 PowerShell。"}}, nil, "")
	if !strings.Contains(ps, "用 PowerShell 做列举、搜索、查找等文件操作") {
		t.Fatalf("powershell: %q", ps)
	}
	bash := composeSystemPrompt("base", []tool.Definition{{Name: "bash", Prompt: "执行 bash。"}}, nil, "/tmp/ws")
	if !strings.Contains(bash, "用 bash 做 ls、rg、find 等文件操作") {
		t.Fatalf("bash: %q", bash)
	}
	if !strings.Contains(bash, "当前工作目录：/tmp/ws") {
		t.Fatalf("cwd: %q", bash)
	}
	withExplore := composeSystemPrompt("base", []tool.Definition{
		{Name: "bash", Prompt: "执行 bash。"},
		{Name: "grep", Prompt: "搜索。"},
	}, nil, "")
	if strings.Contains(withExplore, "用 bash 做 ls、rg、find") {
		t.Fatalf("grep should skip shell explore guideline: %q", withExplore)
	}
}

// TestBuildJoinsToolPrompts 校验 Build 把可见工具描述写入底座，模式规则走 developer。
func TestBuildJoinsToolPrompts(t *testing.T) {
	t.Parallel()
	chat, err := Build(context.Background(), Prompt{
		Run: Run{Config: DefaultRunConfig(WorkAgent, ModelConfig{})},
		Context: ContextSnapshot{
			SystemPrompt:  DefaultSystemPrompt,
			WorkspaceRoot: `/tmp\ws`,
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
	if !strings.Contains(chat.SystemPrompt, "- ping: 连通性检查，无参数，返回 ok。") {
		t.Fatalf("missing tool prompt: %q", chat.SystemPrompt)
	}
	if strings.Contains(chat.SystemPrompt, "需要做事时再调用工具") {
		t.Fatalf("mode rules must not sit in system: %q", chat.SystemPrompt)
	}
	if !strings.Contains(chat.SystemPrompt, "当前工作目录：/tmp/ws") {
		t.Fatalf("missing cwd: %q", chat.SystemPrompt)
	}
	if last := lastDeveloper(chat.Messages); last == "" || !strings.Contains(last, "需要做事时再调用工具") {
		t.Fatalf("missing developer: %#v", chat.Messages)
	}
}

func TestBuildModeDeveloperStableBase(t *testing.T) {
	t.Parallel()
	tools := []tool.Definition{
		{Name: "read", Prompt: "读文件。"},
		{Name: "write", Prompt: "写文件。"},
		{Name: "plan_write", Prompt: "写计划。"},
	}
	ask, err := Build(context.Background(), Prompt{
		Run:     Run{Config: DefaultRunConfig(WorkAsk, ModelConfig{})},
		Context: ContextSnapshot{Tools: tools, Messages: []Message{{Role: RoleUser, Content: EncodeText("hi")}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	agent, err := Build(context.Background(), Prompt{
		Run:     Run{Config: DefaultRunConfig(WorkAgent, ModelConfig{})},
		Context: ContextSnapshot{Tools: tools, Messages: []Message{{Role: RoleUser, Content: EncodeText("hi")}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if ask.SystemPrompt != agent.SystemPrompt {
		t.Fatalf("system must be stable\nask=%q\nagent=%q", ask.SystemPrompt, agent.SystemPrompt)
	}
	if len(ask.Tools) != len(agent.Tools) || ask.Tools[0].Name != agent.Tools[0].Name {
		t.Fatalf("tools must be stable ask=%v agent=%v", ask.Tools, agent.Tools)
	}
	askDev := lastDeveloper(ask.Messages)
	agentDev := lastDeveloper(agent.Messages)
	if askDev == "" || agentDev == "" || askDev == agentDev {
		t.Fatalf("developer must differ ask=%q agent=%q", askDev, agentDev)
	}
	if !strings.Contains(askDev, "只读问答") || !strings.Contains(agentDev, "可以动手") {
		t.Fatalf("developer text ask=%q agent=%q", askDev, agentDev)
	}
	if DecodeText(ask.Messages[0].Content) != "hi" || ask.Messages[len(ask.Messages)-1].Role != RoleDeveloper {
		t.Fatalf("developer should follow history: %#v", ask.Messages)
	}
}

func TestProfileDeveloperPrompt(t *testing.T) {
	t.Parallel()
	empty := profile.Config{}
	if got := empty.DeveloperPrompt(); got != "本轮不能调用任何工具。" {
		t.Fatalf("empty=%q", got)
	}
	inlineOnly := profile.Config{Prompt: profile.PromptConfig{Inline: "  duty  "}, Tools: profile.ToolConfig{Names: []string{"read"}}}
	if got := inlineOnly.DeveloperPrompt(); got != "duty\n\n底座里的工具表是全集，本轮只以这些为准：read。调用未列出的工具会失败。" {
		t.Fatalf("inline=%q", got)
	}
	guidelinesOnly := profile.Config{Prompt: profile.PromptConfig{Guidelines: []string{" a ", ""}}, Tools: profile.ToolConfig{Names: []string{"read"}}}
	if got := guidelinesOnly.DeveloperPrompt(); !strings.Contains(got, "本轮只以这些为准：read") || !strings.Contains(got, "- a") {
		t.Fatalf("guidelines=%q", got)
	}
}

func lastDeveloper(messages []Message) string {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == RoleDeveloper {
			return DecodeText(messages[i].Content)
		}
	}
	return ""
}
