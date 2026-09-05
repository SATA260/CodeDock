package codex

// Settings 是这条对话里最终生效的 Codex 模型、推理强度、Plan 与权限。
type Settings struct {
	Model             string
	Effort            string
	CollaborationMode string   // Codex 的 Plan；空表示非 Plan。
	ApprovalPolicy    string   // Codex 的值，如 on-request。
	Sandbox           string   // Codex 的值，如 workspace-write。
	Cwd               string   // 由调用方（看板）传入的工作路径；问答可空。
	Overridden        []string // 用户改过、需要交给 Codex 的字段名。
}

// Configs 管这条对话里生效的 Codex 配置，并且只把用户改过的项交给 Codex。
// 不管发消息，不管命令怎么拆词。
type Configs struct {
	Command *Command
}

// Effective 返回 Codex 自己的默认值与用户覆盖合并后、这条对话此刻真正生效的配置。
func (c *Configs) Effective(sessionID string) (Settings, error) {
	return Settings{}, nil
}

// Apply 记下用户这次改动，只记改过的项并写进 Overridden；没改的仍旧跟 Codex 默认走。
// 不整份下发，也不自造官方没有的档位。
func (c *Configs) Apply(sessionID string, patch Settings) (Settings, error) {
	c.Command.Invoke(sessionID, "", "")
	return Settings{}, nil
}
