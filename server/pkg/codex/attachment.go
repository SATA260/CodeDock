package codex

// Input 是本条要带给 Codex 的正文、文件提及和图片。
type Input struct {
	Text     string
	Mentions []string // 仓库内路径，对 Codex 的 mention。
	Images   []string // 本地图片路径，对 Codex 的 localImage。
}

// Attachment 管本条消息将要带给 Codex 的文件提及和图片。不管发送，不管工作目录从哪来。
type Attachment struct{}

// Mention 把一个仓库内文件挂到还没发出去的这条内容上，对应 Codex 的 mention。
func (a *Attachment) Mention(sessionID, path string) error {
	return nil
}

// AttachImage 把一张本地图片挂到还没发出去的这条内容上，对应 Codex 的 localImage。
func (a *Attachment) AttachImage(sessionID, path string) error {
	return nil
}

// TakeDraft 取出这条攒好的正文与附件并清空草稿，交给回合发给 Codex。
func (a *Attachment) TakeDraft(sessionID string) (Input, error) {
	return Input{}, nil
}
