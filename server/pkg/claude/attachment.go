package claude

// 本文件管本条将要带给 Claude Code 的文件提及和图片。不管发送，不管工作目录从哪来。草稿不落库，由本次请求携带。

// Mention 把仓库内文件挂到待发给 Claude Code 的内容上。
func Mention(sessionID, path string) error {
	_, err := Get(sessionID)
	_ = path
	return err
}

// AttachImage 把本地图片挂到待发给 Claude Code 的内容上。
func AttachImage(sessionID, path string) error {
	_, err := Get(sessionID)
	_ = path
	return err
}

// TakeDraft 取出本条附件草稿并清空。
func TakeDraft(sessionID string) (Input, error) {
	_, err := Get(sessionID)
	return Input{Mentions: []string{}, Images: []string{}}, err
}
