package claude

// ListCommands 列出与 Claude 斜杠、官方扩展按钮共用的命令。
func ListCommands() []CommandSpec {
	hint := "到终端改 Claude 配置"
	return []CommandSpec{
		{Name: "model", Action: CommandApplySettings},
		{Name: "effort", Action: CommandApplySettings},
		{Name: "plan", Action: CommandApplySettings},
		{Name: "compact", Action: CommandTurn},
		{Name: "review", Action: CommandTurn},
		{Name: "branch", Action: CommandSession},
		{Name: "mcp", Action: CommandHint, Hint: hint},
		{Name: "skills", Action: CommandHint, Hint: hint},
		{Name: "plugins", Action: CommandHint, Hint: hint},
		{Name: "hooks", Action: CommandHint, Hint: hint},
	}
}

// Invoke 执行同名动作，或返回去终端改 Claude 配置的提示。
func Invoke(sessionID, name, args string) (CommandResult, error) {
	_ = ListCommands()
	switch name {
	case "model":
		_, err := Apply(sessionID, Settings{Model: args})
		return CommandResult{}, err
	case "effort":
		_, err := Apply(sessionID, Settings{Effort: args})
		return CommandResult{}, err
	case "plan":
		mode := args
		if mode == "" {
			mode = "plan"
		}
		_, err := Apply(sessionID, Settings{PermissionMode: mode})
		return CommandResult{}, err
	case "compact":
		sess, err := Get(sessionID)
		if err != nil {
			return CommandResult{}, err
		}
		return CommandResult{}, Compact(sess.ClaudeSessionID)
	case "review":
		sess, err := Get(sessionID)
		if err != nil {
			return CommandResult{}, err
		}
		return CommandResult{}, Review(sess.ClaudeSessionID)
	case "branch":
		_, err := Fork(sessionID)
		return CommandResult{}, err
	case "mcp", "skills", "plugins", "hooks":
		return CommandResult{Hint: "到终端改 Claude 配置"}, nil
	default:
		_, err := Start(sessionID, args, Input{Text: args, Mentions: emptyStrings(), Images: emptyStrings()}, InputModeStart)
		return CommandResult{}, err
	}
}
