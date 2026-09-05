package codex

// Codex 管对本机 Codex 说话：开 thread、开一轮、打断、分叉、压缩、评审，
// 以及把反问递进递出。不管本模块有多少对话，不管本地模型。
type Codex struct {
	Transcript *Transcript
	Turn       *Turns
}

// StartThread 让 Codex 新建一条 thread。工作目录和这条对话的生效配置由调用方给。
func (c *Codex) StartThread(cwd string, settings Settings) (threadID string, err error) {
	return "", nil
}

// ResumeThread 接上一条已有的 Codex thread，继续原来那段上下文。
// thread 已经没了就让这一轮失败，不静默新开一条。
func (c *Codex) ResumeThread(threadID string) error {
	return nil
}

// ForkThread 按 Codex 自己落盘的历史分出一条新 thread，原 thread 不动。
func (c *Codex) ForkThread(threadID string) (newThreadID string, err error) {
	return "", nil
}

// StartTurn 把这条输入交给 Codex，让它开始干这一轮。
func (c *Codex) StartTurn(threadID string, input Input, settings Settings) (turnID string, err error) {
	return "", nil
}

// Interrupt 按用户请求打断 Codex 当前这一轮，不关掉 Codex 本身。
func (c *Codex) Interrupt(threadID, turnID string) error {
	return nil
}

// Compact 让 Codex 用它自己的办法压缩这条 thread 的上下文。
func (c *Codex) Compact(threadID string) error {
	return nil
}

// Review 让 Codex 评审当前工作目录里的改动。
func (c *Codex) Review(threadID string) error {
	return nil
}

// ReplyAsk 把人对已知反问的回答回给 Codex，它拿到回话才会接着往下走。
func (c *Codex) ReplyAsk(requestID string, answer AskAnswer) error {
	return nil
}

// RejectUnknown 处理官方新加、本模块认不出的提问：在实录里写明这里接不住，
// 再按拒绝回包，让 Codex 自己换个办法或收尾，别把这一轮卡死。
// 这不等于用户拒绝了某条命令或某批改文件。
func (c *Codex) RejectUnknown(requestID string) error {
	c.Transcript.AppendProgress("", "", Progress{Kind: ProgressNotice})
	c.Turn.Continue("")
	return nil
}

// Module 持有 Codex 对接的九块，自己没有业务方法，方便调用方拿一份就能调各块。
type Module struct {
	Catalog    *Catalog
	Session    *Sessions
	Settings   *Configs
	Command    *Command
	Attachment *Attachment
	Turn       *Turns
	Transcript *Transcript
	Approval   *Approval
	Codex      *Codex
}

// New 构造九块，并把互相要调的那几根指针接上。
func New() *Module {
	m := &Module{
		Catalog:    &Catalog{},
		Session:    &Sessions{},
		Settings:   &Configs{},
		Command:    &Command{},
		Attachment: &Attachment{},
		Turn:       &Turns{},
		Transcript: &Transcript{},
		Approval:   &Approval{},
		Codex:      &Codex{},
	}
	m.Catalog.Session = m.Session
	m.Catalog.Settings = m.Settings
	m.Session.Codex = m.Codex
	m.Settings.Command = m.Command
	m.Turn.Session = m.Session
	m.Turn.Codex = m.Codex
	m.Turn.Transcript = m.Transcript
	m.Turn.Attachment = m.Attachment
	m.Approval.Codex = m.Codex
	m.Approval.Turn = m.Turn
	m.Codex.Transcript = m.Transcript
	m.Codex.Turn = m.Turn
	return m
}
