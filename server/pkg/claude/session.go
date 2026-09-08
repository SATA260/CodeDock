package claude

import "github.com/google/uuid"

// Create 创建一条只走 Claude Code 的对话。Claude session 编号在首次开回合后才有。
func Create(userID string) (Session, error) {
	_ = userID
	rt.mu.Lock()
	defer rt.mu.Unlock()
	sess := internLocked(uuid.NewString())
	return sess.snapshot(), nil
}

// Get 读取对话。
func Get(sessionID string) (Session, error) {
	if sessionID == "" {
		return Session{}, wrapErr(errInvalid, "session_id is required")
	}
	return ReadSession(sessionID)
}

// ListSessions 从本机 Claude 列对话。
func ListSessions() ([]Session, error) {
	return ReadSessions()
}

// BindClaudeSession 记下 Claude session 编号，只能写一次。本模块不落库，编号以本机 Claude 为准。
func BindClaudeSession(sessionID, claudeSessionID string) error {
	if sessionID == "" || claudeSessionID == "" {
		return wrapErr(errInvalid, "session id is required")
	}
	rt.mu.Lock()
	defer rt.mu.Unlock()
	sess := internLocked(sessionID)
	if sess.ClaudeSessionID != "" && sess.ClaudeSessionID != claudeSessionID {
		return wrapErr(errConflict, "claude session already bound")
	}
	sess.ClaudeSessionID = claudeSessionID
	return nil
}

// Archive 归档，之后不能再向 Claude Code 开回合。不删本机 Claude 记录。
func Archive(sessionID string) error {
	sess, err := Get(sessionID)
	if err != nil {
		return err
	}
	rt.mu.Lock()
	defer rt.mu.Unlock()
	internLocked(sess.ID).Archived = true
	return nil
}

// Rename 改对话标题。
func Rename(sessionID, title string) error {
	sess, err := Get(sessionID)
	if err != nil {
		return err
	}
	rt.mu.Lock()
	internLocked(sess.ID).Title = title
	claudeID := internLocked(sess.ID).ClaudeSessionID
	rt.mu.Unlock()
	return renameClaude(claudeID, title)
}

// Fork 按已落盘历史分叉出新对话和新的 Claude session，原对话不动。
func Fork(sessionID string) (Session, error) {
	sess, err := Get(sessionID)
	if err != nil {
		return Session{}, err
	}
	newID, err := ForkSession(sess.ClaudeSessionID)
	if err != nil {
		return Session{}, err
	}
	rt.mu.Lock()
	child := internLocked(newID)
	child.ClaudeSessionID = newID
	if sess.Title != "" {
		child.Title = sess.Title
	}
	out := child.snapshot()
	rt.mu.Unlock()
	return out, nil
}

// ClaimActiveTurn 标成当前执行；同时只能有一个。
func ClaimActiveTurn(sessionID, turnID string) error {
	sess, err := Get(sessionID)
	if err != nil {
		return err
	}
	rt.mu.Lock()
	defer rt.mu.Unlock()
	mem := internLocked(sess.ID)
	if mem.Archived {
		return wrapErr(errArchived, "session is archived")
	}
	if mem.ActiveTurnID != "" && mem.ActiveTurnID != turnID {
		return wrapErr(errConflict, "turn already running")
	}
	mem.ActiveTurnID = turnID
	return nil
}

// ClearActiveTurn 清掉当前执行标记。
func ClearActiveTurn(sessionID, turnID string) error {
	if sessionID == "" {
		rt.mu.Lock()
		if turn, ok := rt.turns[turnID]; ok {
			sessionID = turn.SessionID
		}
		rt.mu.Unlock()
	}
	if sessionID == "" {
		return nil
	}
	rt.mu.Lock()
	defer rt.mu.Unlock()
	mem := internLocked(sessionID)
	if mem.ActiveTurnID == turnID || turnID == "" {
		mem.ActiveTurnID = ""
	}
	return nil
}
