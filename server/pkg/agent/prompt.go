package agent

import (
	"context"
	"strings"

	"codedock/pkg/agent/tool"
)

// Prompt 包含组装模型调用所需的已准备数据。
type Prompt struct {
	Run     Run
	Turn    Turn
	Context ContextSnapshot
}

var baseGuidelines = []string{
	"用用户的语言回复",
	"不要输出 emoji、颜文字、表情符号",
	"不要用空泛开场或自我介绍，除非用户问你是谁",
	"不要堆砌感叹号，不要加油打气，不要用网络流行语",
	"回复尽量简洁",
	"涉及文件时写清路径",
}

// ComposeSystemPrompt 把身份段与当前可见工具拼成一次调用用的底座系统提示，结构仿 pi：
// 身份 → 可用工具 → 使用要求 → 工作目录。模式规则不在这里。
func ComposeSystemPrompt(base string, tools []tool.Definition) string {
	return composeSystemPrompt(base, tools, nil, "")
}

func composeSystemPrompt(base string, tools []tool.Definition, extra []string, cwd string) string {
	base = strings.TrimSpace(base)
	cwd = strings.TrimSpace(strings.ReplaceAll(cwd, "\\", "/"))

	var b strings.Builder
	if base != "" {
		b.WriteString(base)
		b.WriteString("\n\n")
	}
	b.WriteString("可用工具：\n")
	b.WriteString(formatToolSnippets(tools))
	b.WriteString("\n\n除以上工具外，项目还可能提供其他自定义工具。\n\n使用要求：\n")
	b.WriteString(formatGuidelines(tools, extra))
	if cwd != "" {
		b.WriteString("\n\n当前工作目录：")
		b.WriteString(cwd)
	}
	return b.String()
}

func formatToolSnippets(tools []tool.Definition) string {
	var b strings.Builder
	for _, def := range tools {
		name := strings.TrimSpace(def.Name)
		desc := strings.TrimSpace(def.Prompt)
		if name == "" || desc == "" {
			continue
		}
		if b.Len() > 0 {
			b.WriteByte('\n')
		}
		b.WriteString("- ")
		b.WriteString(name)
		b.WriteString(": ")
		b.WriteString(desc)
	}
	if b.Len() == 0 {
		return "（无）"
	}
	return b.String()
}

func formatGuidelines(tools []tool.Definition, extra []string) string {
	names := make(map[string]bool, len(tools))
	for _, def := range tools {
		if name := strings.TrimSpace(def.Name); name != "" {
			names[name] = true
		}
	}

	var list []string
	seen := make(map[string]struct{})
	add := func(guideline string) {
		guideline = strings.TrimSpace(guideline)
		if guideline == "" {
			return
		}
		if _, ok := seen[guideline]; ok {
			return
		}
		seen[guideline] = struct{}{}
		list = append(list, guideline)
	}

	hasBash := names["bash"]
	hasPowerShell := names["powershell"]
	hasGrep := names["grep"]
	hasFind := names["find"]
	hasLs := names["ls"]
	if (hasBash || hasPowerShell) && !hasGrep && !hasFind && !hasLs {
		switch {
		case hasBash && hasPowerShell:
			add("用 bash 或 PowerShell 做列举、搜索、查找等文件操作")
		case hasPowerShell:
			add("用 PowerShell 做列举、搜索、查找等文件操作")
		default:
			add("用 bash 做 ls、rg、find 等文件操作")
		}
	}
	for _, guideline := range extra {
		add(guideline)
	}
	for _, guideline := range baseGuidelines {
		add(guideline)
	}

	var b strings.Builder
	for i, guideline := range list {
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString("- ")
		b.WriteString(guideline)
	}
	return b.String()
}

// Build 将上下文组装为模型调用。发给网关的工具表按本轮 Names 裁过；模式规则追加为 developer。
func Build(_ context.Context, req Prompt) (Chat, error) {
	system := req.Context.SystemPrompt
	if system == "" {
		system = DefaultSystemPrompt
	}
	tools := boundToolDefs(req.Context.Tools, req.Run.Config.Profile.Tools.Names)
	system = composeSystemPrompt(system, tools, nil, req.Context.WorkspaceRoot)
	var prefix []Message
	for _, index := range req.Context.MemoryIndexes {
		if index == "" {
			continue
		}
		prefix = append(prefix, Message{Role: RoleSystem, Content: EncodeText(index)})
	}
	for _, msg := range req.Context.Hidden {
		if len(msg.Content) == 0 {
			continue
		}
		prefix = append(prefix, Message{Role: RoleSystem, Content: msg.Content})
	}
	if req.Context.Summary != nil && req.Context.Summary.Content != "" {
		prefix = append(prefix, Message{
			Role:    RoleSystem,
			Content: EncodeText("对话摘要：\n" + req.Context.Summary.Content),
		})
	}
	messages := CompleteToolResults(append(prefix, req.Context.Messages...))
	dev := strings.TrimSpace(req.Run.Config.Profile.DeveloperPrompt())
	if note := PlanScopeNote(req.Context.ActivePlan); note != "" {
		if dev != "" {
			dev = dev + "\n\n" + note
		} else {
			dev = note
		}
	}
	if dev != "" {
		messages = append(messages, Message{Role: RoleDeveloper, Content: EncodeText(dev)})
	}
	return Chat{
		SessionID:       req.Context.SessionID,
		RunID:           req.Run.ID,
		TurnID:          req.Turn.ID,
		Model:           req.Run.Config.Model,
		SystemPrompt:    system,
		Messages:        messages,
		Tools:           tools,
		MaxInputTokens:  req.Run.Config.Limits.MaxInputTokens,
		MaxOutputTokens: req.Run.Config.Limits.MaxOutputTokens,
		Attempt:         1,
	}, nil
}

// boundToolDefs 按本轮可执行名裁工具；名单为空时保持原表，方便测试只塞几件工具。
func boundToolDefs(all []tool.Definition, names []string) []tool.Definition {
	if len(names) == 0 {
		return all
	}
	return tool.VisibleDefinitions(all, names)
}
