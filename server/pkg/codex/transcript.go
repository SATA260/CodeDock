package codex

// ProgressKind 是实录里一条进展的种类。
type ProgressKind string

const (
	ProgressUser       ProgressKind = "user"
	ProgressText       ProgressKind = "text"
	ProgressReasoning  ProgressKind = "reasoning"
	ProgressCommand    ProgressKind = "command"
	ProgressFileChange ProgressKind = "file_change"
	ProgressPlan       ProgressKind = "plan"
	ProgressNotice     ProgressKind = "notice" // 给人看的说明，如这里接不住某种提问。
)

// Progress 是给人看、可回放的一条 Codex 进展。
type Progress struct {
	Kind    ProgressKind
	Text    string
	Command string
	Paths   []string
	Diff    string
}

// Transcript 管把 Codex 的进展落成给人看、可回放的记录。这份记录只给人看，
// 不回灌给 Codex 当上下文。不管驱动 Codex，不管改磁盘。
type Transcript struct{}

// AppendUser 记下用户发的这一条，连同它带的文件提及和图片。
func (t *Transcript) AppendUser(sessionID, turnID, text string, input Input) error {
	return nil
}

// AppendProgress 记下 Codex 这一步的进展：正文、推理、跑了什么命令、改了哪些文件、
// 出的方案，或者「这里接不住」这类提示。
func (t *Transcript) AppendProgress(sessionID, turnID string, item Progress) error {
	return nil
}

// Hydrate 按已落下的记录回放整条对话，供人重连后接着看。
func (t *Transcript) Hydrate(sessionID string) ([]Progress, error) {
	return nil, nil
}
