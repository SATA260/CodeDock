package codex

// CommandAction 是一条斜杠或扩展按钮落地后交给谁办。
type CommandAction string

const (
	ActionApplySettings CommandAction = "apply_settings" // 交给官方配置改值。
	ActionTurn          CommandAction = "turn"           // 交给回合，如发送、打断。
	ActionSession       CommandAction = "session"        // 交给会话，如 fork、归档、改标题。
	ActionAttach        CommandAction = "attach"         // 交给附件，如挂文件、贴图。
	ActionHint          CommandAction = "hint"           // 本模块不落地，只提示去终端改 Codex 配置。
)

// CommandSpec 是一条与 Codex 斜杠、官方扩展按钮共用的命令。
type CommandSpec struct {
	Name   string        `json:"name"` // 与 Codex 斜杠同名，如 model、plan、fork、mcp。
	Action CommandAction `json:"action"`
	Hint   string        `json:"hint,omitempty"` // hint 时给人看的话。
	Field  string        `json:"field,omitempty"`
}

// CommandResult 是这条命令没法在本模块落地时，给人看的说明。
type CommandResult struct {
	Hint    string        `json:"hint,omitempty"`
	Action  CommandAction `json:"action,omitempty"`
	Handled bool          `json:"handled"`
}

const hintUseTerminal = "这条配置请在终端里改 Codex（~/.codex/config.toml），本模块不改官方配置文件。"

// Commands 列出对话框 `/` 里能用的命令。这些和官方扩展按钮共用同一套动作。
func Commands() []CommandSpec {
	return []CommandSpec{
		{Name: "model", Action: ActionApplySettings, Field: "model"},
		{Name: "effort", Action: ActionApplySettings, Field: "effort"},
		{Name: "plan", Action: ActionApplySettings, Field: "collaboration_mode"},
		{Name: "permissions", Action: ActionApplySettings, Field: "sandbox"},
		{Name: "approval", Action: ActionApplySettings, Field: "approval_policy"},
		{Name: "stop", Action: ActionTurn},
		{Name: "compact", Action: ActionTurn},
		{Name: "review", Action: ActionTurn},
		{Name: "fork", Action: ActionSession},
		{Name: "archive", Action: ActionSession},
		{Name: "rename", Action: ActionSession},
		{Name: "mention", Action: ActionAttach},
		{Name: "image", Action: ActionAttach},
		{Name: "mcp", Action: ActionHint, Hint: hintUseTerminal},
		{Name: "skills", Action: ActionHint, Hint: hintUseTerminal},
		{Name: "plugins", Action: ActionHint, Hint: hintUseTerminal},
		{Name: "hooks", Action: ActionHint, Hint: hintUseTerminal},
	}
}

// LookupCommand 按与 Codex 同名的斜杠名取命令。
func LookupCommand(name string) (CommandSpec, bool) {
	for _, spec := range Commands() {
		if spec.Name == name {
			return spec, true
		}
	}
	return CommandSpec{}, false
}
