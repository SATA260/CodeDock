package profile

import "codedock/pkg/agent/tool"

// PromptConfig 描述 Agent 使用的提示词。
type PromptConfig struct {
	Source    string `json:"source,omitempty"`
	Reference string `json:"reference,omitempty"`
	Inline    string `json:"inline,omitempty"`
	Version   string `json:"version,omitempty"`
}

// ToolConfig 描述 Agent 暴露的工具集及策略。
type ToolConfig struct {
	Names            []string              `json:"names,omitempty"`
	Version          string                `json:"version,omitempty"`
	PermissionPolicy tool.PermissionPolicy `json:"permission_policy"`
	ApprovalPolicy   tool.ApprovalPolicy   `json:"approval_policy"`
}

// Config 是一份 Agent 配置的抽象。
type Config struct {
	ID      string       `json:"id"`
	Version string       `json:"version,omitempty"`
	Mode    string       `json:"mode,omitempty"`
	Prompt  PromptConfig `json:"prompt"`
	Tools   ToolConfig   `json:"tools"`
}
