package codex

// CommandAction 是一条斜杠或扩展按钮落地后交给谁办。
type CommandAction string

const (
	ActionApplySettings CommandAction = "apply_settings" // 交给官方配置改值。
	ActionTurn          CommandAction = "turn"           // 交给回合，如发送、打断。
	ActionSession       CommandAction = "session"        // 交给会话，如分叉、归档、改标题。
	ActionAttach        CommandAction = "attach"         // 交给附件，如挂文件、贴图。
	ActionHint          CommandAction = "hint"           // 本模块不落地，只提示去终端改 Codex 配置。
)

// CommandSpec 是一条与 Codex 斜杠、官方扩展按钮共用的命令。
type CommandSpec struct {
	Name   string // 与 Codex 斜杠同名，如 model、plan、fork、mcp。
	Action CommandAction
	Hint   string // hint 时给人看的话。
}

// CommandResult 是这条命令没法在本模块落地时，给人看的说明。
type CommandResult struct {
	Hint string
}

// Command 管把 Codex 斜杠名和官方扩展按钮收成同一套动作：能做的往下交给会话、配置、
// 回合或附件，不能做的只给一句提示。不管怎么对 Codex 说话。
type Command struct{}

// List 列出对话框 `/` 里能用的命令。这些和官方扩展按钮共用同一套动作，不搞两套语义。
// `/mcp`、`/skills` 这类也在列表里，但点了只会给提示。
func (c *Command) List() []CommandSpec {
	return nil
}

// Invoke 执行与 Codex 同名的那个动作。本模块不做的配置类命令不改 Codex 配置文件，
// 只回一句「去终端改」的提示。
func (c *Command) Invoke(sessionID, name, args string) (CommandResult, error) {
	return CommandResult{}, nil
}
