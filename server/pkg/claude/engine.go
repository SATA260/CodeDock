package claude

// 本文件对本机 Claude Code 说话：开 session、开一轮、打断、分叉、压缩、评审，以及把反问递进递出。

// StartSession 让 Claude Code 新建一条 session。
func StartSession(cwd string, settings Settings) (string, error) {
	_ = cwd
	_ = settings
	return "", nil
}

// ResumeSession 接上已有的 Claude session。
func ResumeSession(claudeSessionID string) error {
	_ = claudeSessionID
	return nil
}

// ForkSession 按 Claude 已落盘历史分叉出新 session。
func ForkSession(claudeSessionID string) (string, error) {
	_ = claudeSessionID
	return "", nil
}

// StartTurn 让 Claude Code 开始一轮。
func StartTurn(claudeSessionID string, input Input, settings Settings) (string, error) {
	_ = claudeSessionID
	_ = input
	_ = settings
	return "", nil
}

// Interrupt 按用户请求打断 Claude Code 当前一轮。
func Interrupt(claudeSessionID, turnID string) error {
	_ = claudeSessionID
	_ = turnID
	return nil
}

// Compact 让 Claude Code 自己压缩上下文。
func Compact(claudeSessionID string) error {
	_ = claudeSessionID
	return nil
}

// Review 让 Claude Code 评审当前工作区改动。
func Review(claudeSessionID string) error {
	_ = claudeSessionID
	return nil
}

// ReplyAsk 把人对已知反问的回答回给 Claude Code。
func ReplyAsk(requestID string, answer AskAnswer) error {
	_ = requestID
	_ = answer
	return nil
}

// RejectUnknown 官方新加、认不出的提问：提示不兼容并回包，避免卡死。
func RejectUnknown(requestID string) error {
	_ = requestID
	return nil
}
