package profile

import (
	"strings"
	"testing"
)

func TestForAgents(t *testing.T) {
	ask := For("ask")
	if ask.ID != "ask" || !contains(ask.Tools.Names, "memory_read") || contains(ask.Tools.Names, "write") || contains(ask.Tools.Names, "plan_write") {
		t.Fatalf("ask names=%v", ask.Tools.Names)
	}
	if ask.Prompt.Inline == "" || !contains(ask.Prompt.Guidelines, "需要查资料时再调用只读工具") {
		t.Fatal("ask prompt empty")
	}
	if strings.Contains(ask.Prompt.Inline, "你是 CodeDock") || strings.Contains(ask.Prompt.Inline, "可用工具") {
		t.Fatalf("ask identity leaked into mode prompt=%q", ask.Prompt.Inline)
	}
	dev := ask.DeveloperPrompt()
	if !strings.Contains(dev, "只读问答") || !strings.Contains(dev, "本轮可以调用：read、grep、find、ls、memory_read、memory_search") {
		t.Fatalf("ask developer=%q", dev)
	}
	allowed := allowedToolsSection(dev)
	if strings.Contains(allowed, "write") || strings.Contains(allowed, "bash") {
		t.Fatalf("ask allowed list must not include write tools: %q", allowed)
	}
	plan := For("plan")
	if plan.ID != "plan" || !contains(plan.Tools.Names, "plan_write") || contains(plan.Tools.Names, "bash") {
		t.Fatalf("plan names=%v", plan.Tools.Names)
	}
	if !contains(plan.Prompt.Guidelines, "只用计划工具改 .cursor/ 下的文件，不要动仓库里的其他文件") {
		t.Fatalf("plan guidelines=%v", plan.Prompt.Guidelines)
	}
	if !contains(plan.Prompt.Guidelines, "用户没有点名已有计划时，不要列出或读取其他计划，为本会话新建一篇") {
		t.Fatalf("plan isolation=%v", plan.Prompt.Guidelines)
	}
	devPlan := plan.DeveloperPrompt()
	if !strings.Contains(devPlan, "本轮可以调用：plan_list、plan_read、plan_write、plan_pass") {
		t.Fatalf("plan developer=%q", devPlan)
	}
	if !strings.Contains(devPlan, "id、description、verify_cmd") || !strings.Contains(devPlan, "Markdown") {
		t.Fatalf("plan must spell out markdown items: %q", devPlan)
	}
	agent := For("agent")
	if agent.ID != "agent" || !contains(agent.Tools.Names, "bash") || !contains(agent.Tools.Names, "plan_write") || !contains(agent.Tools.Names, "plan_pass") || !contains(agent.Tools.Names, "explore") {
		t.Fatalf("agent names=%v", agent.Tools.Names)
	}
	if !contains(agent.Prompt.Guidelines, "更新计划用 plan_write，不必等人批准") {
		t.Fatalf("agent guidelines=%v", agent.Prompt.Guidelines)
	}
	if !contains(agent.Prompt.Guidelines, "用户没有点名已有计划时，不要列出或读取其他计划，只动本会话这一篇") {
		t.Fatalf("agent isolation=%v", agent.Prompt.Guidelines)
	}
	devAgent := agent.DeveloperPrompt()
	if !strings.Contains(devAgent, "write、edit、bash、powershell") || !strings.Contains(allowedToolsSection(devAgent), "plan_write") {
		t.Fatalf("agent developer=%q", devAgent)
	}
	if !strings.Contains(devAgent, "id、description、verify_cmd") {
		t.Fatalf("agent must spell out plan item fields: %q", devAgent)
	}
	if For("unknown").ID != "agent" {
		t.Fatal("unknown should fall back to agent")
	}
}

func allowedToolsSection(dev string) string {
	const mark = "本轮可以调用："
	i := strings.Index(dev, mark)
	if i < 0 {
		return ""
	}
	section := dev[i+len(mark):]
	if end := strings.Index(section, "。"); end >= 0 {
		return section[:end]
	}
	return section
}

func contains(items []string, name string) bool {
	for _, item := range items {
		if item == name {
			return true
		}
	}
	return false
}
