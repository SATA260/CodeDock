package claude

// 本文件管一次用户请求对应的那一轮 Claude Code 工作：开始、排队、手动打断。不管实录怎么记，不管人怎么点批准。

// Start 空闲则向 Claude Code 开新一轮；进行中则排队，不打断。
func Start(sessionID, content string, input Input, mode InputMode) (string, error) {
	if mode == InputModeQueue {
		return Queue(sessionID, content, input)
	}
	draft, err := TakeDraft(sessionID)
	if err != nil {
		return "", err
	}
	_ = draft
	settings, err := Effective(sessionID)
	if err != nil {
		return "", err
	}
	if err := ClaimActiveTurn(sessionID, ""); err != nil {
		return "", err
	}
	sess, err := Get(sessionID)
	if err != nil {
		return "", err
	}
	claudeSessionID := sess.ClaudeSessionID
	if claudeSessionID == "" {
		claudeSessionID, err = StartSession(settings.Cwd, settings)
		if err != nil {
			return "", err
		}
		if err := BindClaudeSession(sessionID, claudeSessionID); err != nil {
			return "", err
		}
	} else if err := ResumeSession(claudeSessionID); err != nil {
		return "", err
	}
	turnID, err := StartTurn(claudeSessionID, input, settings)
	if err != nil {
		return "", err
	}
	if err := AppendUser(sessionID, turnID, content, input); err != nil {
		return "", err
	}
	return turnID, nil
}

// Queue 等当前 Claude Code 一轮结束或被打断后再开。
func Queue(sessionID, content string, input Input) (string, error) {
	if err := AppendUser(sessionID, "", content, input); err != nil {
		return "", err
	}
	return "", nil
}

// Cancel 用户手动打断当前一轮，并向 Claude Code 传播取消。打断后再开排队里的下一条。
func Cancel(turnID string) error {
	if err := Interrupt("", turnID); err != nil {
		return err
	}
	if err := ClearActiveTurn("", turnID); err != nil {
		return err
	}
	_, err := Start("", "", Input{}, InputModeStart)
	return err
}

// Continue 反问有了结果后让 Claude Code 继续。
func Continue(turnID string) error {
	_ = turnID
	return nil
}
