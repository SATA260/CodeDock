package profile

import "codedock/pkg/agent/tool"

// PromptConfig 描述 Agent 使用的提示词。
type PromptConfig struct {
	Source    string
	Reference string
	Inline    string
	Version   string
}

// ToolConfig 描述 Agent 暴露的工具集及策略。
type ToolConfig struct {
	Names            []string
	Version          string
	PermissionPolicy tool.PermissionPolicy
	ApprovalPolicy   tool.ApprovalPolicy
}

// Config 是一份 Agent 配置的抽象。
type Config struct {
	ID      string
	Version string
	Mode    string
	Prompt  PromptConfig
	Tools   ToolConfig
}
