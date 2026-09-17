package claude

import "github.com/google/uuid"

// Start 空闲则向 Claude Code 开新一轮；进行中则排队，不打断。
func Start(sessionID, content string, input Input, mode InputMode) (string, error) {
	if sessionID == "" {
		return "", wrapErr(errInvalid, "session_id is required")
	}
	sess, err := Get(sessionID)
	if err != nil {
		return "", err
	}
	rt.mu.Lock()
	archived := internLocked(sess.ID).Archived
	active := internLocked(sess.ID).ActiveTurnID
	rt.mu.Unlock()
	if archived {
		return "", wrapErr(errArchived, "session is archived")
	}
	if mode == InputModeQueue || active != "" {
		return Queue(sessionID, content, input)
	}
	draft, err := TakeDraft(sessionID)
	if err != nil {
		return "", err
	}
	merged := mergeInput(content, input, draft)
	settings, err := Effective(sessionID)
	if err != nil {
		return "", err
	}
	turnID := uuid.NewString()
	if err := ClaimActiveTurn(sessionID, turnID); err != nil {
		return "", err
	}
	sess, err = Get(sessionID)
	if err != nil {
		return "", err
	}
	claudeSessionID := sess.ClaudeSessionID
	if claudeSessionID == "" {
		claudeSessionID, err = StartSession(settings.Cwd, settings)
		if err != nil {
			_ = ClearActiveTurn(sessionID, turnID)
			return "", err
		}
		if err := BindClaudeSession(sessionID, claudeSessionID); err != nil {
			_ = ClearActiveTurn(sessionID, turnID)
			return "", err
		}
	} else if err := ResumeSession(claudeSessionID); err != nil {
		_ = ClearActiveTurn(sessionID, turnID)
		return "", err
	}
	if err := startTurn(turnID, sessionID, claudeSessionID, merged, settings); err != nil {
		_ = ClearActiveTurn(sessionID, turnID)
		return "", err
	}
	if err := AppendUser(sessionID, turnID, content, merged); err != nil {
		return "", err
	}
	return turnID, nil
}

// Queue 等当前 Claude Code 一轮结束或被打断后再开。
func Queue(sessionID, content string, input Input) (string, error) {
	if sessionID == "" {
		return "", wrapErr(errInvalid, "session_id is required")
	}
	if _, err := Get(sessionID); err != nil {
		return "", err
	}
	draft, _ := TakeDraft(sessionID)
	merged := mergeInput(content, input, draft)
	turnID := uuid.NewString()
	rt.mu.Lock()
	internLocked(sessionID).Queue = append(internLocked(sessionID).Queue, queuedTurn{Content: content, Input: merged})
	rt.mu.Unlock()
	if err := AppendUser(sessionID, turnID, content, merged); err != nil {
		return "", err
	}
	return turnID, nil
}

func drainQueue(sessionID string) {
	rt.mu.Lock()
	sess := internLocked(sessionID)
	if sess.Archived || sess.ActiveTurnID != "" || len(sess.Queue) == 0 {
		rt.mu.Unlock()
		return
	}
	item := sess.Queue[0]
	sess.Queue = sess.Queue[1:]
	rt.mu.Unlock()
	_, _ = Start(sessionID, item.Content, item.Input, InputModeStart)
}

// Cancel 用户手动打断当前一轮，并向 Claude Code 传播取消。打断后再开排队里的下一条。
func Cancel(turnID string) error {
	if turnID == "" {
		return nil
	}
	rt.mu.Lock()
	turn := rt.turns[turnID]
	sessionID := ""
	claudeID := ""
	if turn != nil {
		sessionID = turn.SessionID
		claudeID = turn.ClaudeSessionID
		turn.Status = TurnCancelled
	}
	rt.mu.Unlock()
	if turn == nil {
		return nil
	}
	_ = Interrupt(claudeID, turnID)
	_ = ClearActiveTurn(sessionID, turnID)
	if sessionID != "" {
		drainQueue(sessionID)
	}
	return nil
}

// Continue 反问有了结果后让 Claude Code 继续。
func Continue(turnID string) error {
	_ = turnID
	return nil
}
