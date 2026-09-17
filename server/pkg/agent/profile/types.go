package profile

import (
	"strings"

	"codedock/pkg/agent/tool"
)

// PromptConfig 描述 Agent 使用的提示词。
type PromptConfig struct {
	Source     string   `json:"source,omitempty"`     // 来源：inline / 文件等
	Reference  string   `json:"reference,omitempty"`  // 引用名，内置 Agent 用自身 ID
	Inline     string   `json:"inline,omitempty"`     // 本轮模式规则正文；底座身份不在这里
	Guidelines []string `json:"guidelines,omitempty"` // 本轮模式使用要求，拼进 developer 消息
	Version    string   `json:"version,omitempty"`
}

// ToolConfig 描述 Agent 暴露的工具集及覆盖表。
type ToolConfig struct {
	Names   []string               `json:"names,omitempty"` // 本 Agent 可执行的工具名；不在列表里 Dispatch 拒绝
	Version string                 `json:"version,omitempty"`
	Effects map[string]tool.Effect `json:"effects,omitempty"` // 第 2 层覆盖表；只审第 1 层仍为 ask 的调用
}

// Config 是一份内置 Agent 的配置。
type Config struct {
	ID      string       `json:"id"` // ask / plan / agent
	Version string       `json:"version,omitempty"`
	Mode    string       `json:"mode,omitempty"` // 与 ID 相同，标识这份配置对应的工作模式
	Prompt  PromptConfig `json:"prompt"`
	Tools   ToolConfig   `json:"tools"`
}

func configOf(id, version, duty string, extra []string, names []string) Config {
	return Config{
		ID:      id,
		Version: version,
		Mode:    id,
		Prompt: PromptConfig{
			Source:     "inline",
			Inline:     duty,
			Guidelines: extra,
			Version:    version,
			Reference:  id,
		},
		Tools: ToolConfig{
			Names:   names,
			Version: id + "-" + version,
		},
	}
}

// DeveloperPrompt 拼出本轮模式的 developer 消息正文。空则调用方不注入。
func (c Config) DeveloperPrompt() string {
	body := strings.TrimSpace(c.Prompt.Inline)
	allowed := formatAllowedTools(c.Tools.Names)
	var b strings.Builder
	for _, item := range c.Prompt.Guidelines {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if b.Len() > 0 {
			b.WriteByte('\n')
		}
		b.WriteString("- ")
		b.WriteString(item)
	}
	extra := b.String()
	var parts []string
	if body != "" {
		parts = append(parts, body)
	}
	if allowed != "" {
		parts = append(parts, allowed)
	}
	if extra != "" {
		parts = append(parts, "使用要求：\n"+extra)
	}
	return strings.Join(parts, "\n\n")
}

func formatAllowedTools(names []string) string {
	clean := make([]string, 0, len(names))
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		clean = append(clean, name)
	}
	if len(clean) == 0 {
		return "本轮不能调用任何工具。"
	}
	return "底座里的工具表是全集，本轮只以这些为准：" + strings.Join(clean, "、") + "。调用未列出的工具会失败。"
}

// For 按工作模式返回对应的内置 Agent。未知值回落到 agent。
func For(mode string) Config {
	switch mode {
	case "ask":
		return Ask()
	case "plan":
		return Plan()
	default:
		return Agent()
	}
}
