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

// ComposeSystemPrompt 把静态系统提示与当前可见工具各自维护的描述拼成一次调用用的提示词。
func ComposeSystemPrompt(base string, tools []tool.Definition) string {
	base = strings.TrimSpace(base)
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
		b.WriteString("：")
		b.WriteString(desc)
	}
	extra := b.String()
	if extra == "" {
		return base
	}
	if base == "" {
		return "工具：\n" + extra
	}
	return base + "\n\n工具：\n" + extra
}

func withWorkspacePrompt(system, root string) string {
	root = strings.TrimSpace(root)
	if root == "" {
		return system
	}
	line := "Current working directory: " + root
	if system == "" {
		return line
	}
	return system + "\n\n" + line
}

// Build 将上下文组装为模型调用。工具描述由各工具自己维护，这里只做统一拼接。
func Build(_ context.Context, req Prompt) (Chat, error) {
	system := req.Context.SystemPrompt
	if system == "" {
		system = req.Run.Config.Profile.Prompt.Inline
	}
	if system == "" {
		system = DefaultSystemPrompt
	}
	system = ComposeSystemPrompt(system, req.Context.Tools)
	system = withWorkspacePrompt(system, req.Context.WorkspaceRoot)
	var prefix []Message
	for _, index := range req.Context.MemoryIndexes {
		if index == "" {
			continue
		}
		prefix = append(prefix, Message{Role: RoleSystem, Content: EncodeText(index)})
	}
	if req.Context.Summary != nil && req.Context.Summary.Content != "" {
		prefix = append(prefix, Message{
			Role:    RoleSystem,
			Content: EncodeText("Conversation summary:\n" + req.Context.Summary.Content),
		})
	}
	messages := CompleteToolResults(append(prefix, req.Context.Messages...))
	return Chat{
		SessionID:       req.Context.SessionID,
		RunID:           req.Run.ID,
		TurnID:          req.Turn.ID,
		Model:           req.Run.Config.Model,
		SystemPrompt:    system,
		Messages:        messages,
		Tools:           req.Context.Tools,
		MaxInputTokens:  req.Run.Config.Limits.MaxInputTokens,
		MaxOutputTokens: req.Run.Config.Limits.MaxOutputTokens,
		Attempt:         1,
	}, nil
}
