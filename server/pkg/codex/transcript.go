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
	Kind    ProgressKind `json:"kind"`
	ItemID  string       `json:"item_id,omitempty"`
	Text    string       `json:"text,omitempty"`
	Command string       `json:"command,omitempty"`
	Paths   []string     `json:"paths,omitempty"`
	Diff    string       `json:"diff,omitempty"`
	Status  string       `json:"status,omitempty"`
}
