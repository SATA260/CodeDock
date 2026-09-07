package claude

// 本文件管把 Claude Code 的进展落成给人看、可回放的记录。不管驱动 Claude Code，不管改磁盘。实录以本机 Claude 为准，不落本模块的库。

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
	_, err := Get(sessionID)
	_ = turnID
	_ = item
	return err
}

// Hydrate 按本机 Claude 已落下的记录回放。
func Hydrate(sessionID string) ([]Progress, error) {
	return ReadTranscript(sessionID)
}
