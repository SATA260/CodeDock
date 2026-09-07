package claude

// 本文件管 Claude Code 已知的反问：跑命令、改文件、选择题、MCP 弹出的表单。不管官方新加、本模块认不出的提问，不管本地模型那套工具审批。问票以 Claude 为准，不落库。

// Require 登记一条 Claude Code 已知的反问。
func Require(turnID string, ask ApprovalAsk) (string, error) {
	_ = turnID
	_ = ask
	return "", nil
}

// Decide 按人对已知反问的作答记下结果。
func Decide(approvalID string, answer AskAnswer) error {
	if err := ReplyAsk(approvalID, answer); err != nil {
		return err
	}
	return Continue("")
}

// Expire 过期按拒绝回给 Claude Code，避免死等。
func Expire(approvalID string) error {
	return ReplyAsk(approvalID, AskAnswer{Approved: false})
}
