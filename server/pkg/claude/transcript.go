package claude

// AppendUser 记下用户这一条。
func AppendUser(sessionID, turnID, text string, input Input) error {
	_, err := Get(sessionID)
	_ = turnID
	_ = text
	_ = input
	return err
}

// AppendProgress 记下 Claude Code 的推理、正文、命令、改文件、方案或提示。
func AppendProgress(sessionID, turnID string, item Progress) error {
	if sessionID == "" {
		return nil
	}
	_, err := Get(sessionID)
	_ = turnID
	_ = item
	return err
}

// Hydrate 按本机 Claude 已落下的记录回放。
func Hydrate(sessionID string) ([]Progress, error) {
	if sessionID == "" {
		return []Progress{}, wrapErr(errInvalid, "session_id is required")
	}
	sess, err := Get(sessionID)
	if err != nil {
		return []Progress{}, err
	}
	id := sess.ClaudeSessionID
	if id == "" {
		id = sess.ID
	}
	return ReadTranscript(id)
}
