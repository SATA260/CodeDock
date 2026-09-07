package claude

// 本文件把 Claude Code 斜杠名和官方扩展按钮收成同一套动作。不管怎么对 Claude Code 说话。

// ListCommands 列出与 Claude 斜杠、官方扩展按钮共用的命令。
func ListCommands() []CommandSpec {
	return []CommandSpec{}
}

// Invoke 执行同名动作，或返回去终端改 Claude 配置的提示。
func Invoke(sessionID, name, args string) (CommandResult, error) {
	_ = ListCommands()
	switch name {
	case "model", "effort", "plan":
		_, err := Apply(sessionID, Settings{})
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
		_ = args
		_, err := Start(sessionID, args, Input{Text: args}, InputModeStart)
		return CommandResult{}, err
	}
}
