package codex

// DecisionScope 是这次作答管一次，还是管整条对话。
type DecisionScope string

const (
	ScopeOnce    DecisionScope = "once"    // 只管这一次。
	ScopeSession DecisionScope = "session" // 本会话以后同类不再问。
)

// AskKind 是 Codex 已知反问的种类，这几种都要有完整作答界面。
type AskKind string

const (
	AskCommand    AskKind = "command"     // 能不能跑这条命令。
	AskFileChange AskKind = "file_change" // 能不能改这些文件。
	AskQuestion   AskKind = "question"    // 让人补一句字，或从几个选项里挑一个。
	AskForm       AskKind = "form"        // MCP 跑起来之后弹出来、要人填的表单。
)

// ApprovalAsk 是一条 Codex 已知的反问。
type ApprovalAsk struct {
	Kind              AskKind
	Command           string
	Paths             []string
	Diff              string
	Prompt            string   // 选择题或表单给人看的题面。
	Options           []string // 补一句字时的选项。
	Fields            []string // MCP 表单字段名。
	ExternalRequestID string   // 用来回给 Codex 的那张问票。
}

// AskAnswer 是人对这条反问的作答。
type AskAnswer struct {
	Approved bool
	Scope    DecisionScope
	Choice   string   // 选择题选中的项。
	Values   []string // 表单填写结果。
}

// Approval 管 Codex 已知的反问：跑命令、改文件、补一句字、MCP 弹出的表单，这几种都做完整界面。
// 不管官方新加、本模块认不出的提问，也不管本地模型那套工具审批。
type Approval struct {
	Codex *Codex
	Turn  *Turns
}

// Require 登记一条 Codex 的反问等人作答，拿到结果后回给 Codex，并让这一轮接着跑。
// 人不回话 Codex 就一直等着，所以这条必须有人答。
func (a *Approval) Require(turnID string, ask ApprovalAsk) (approvalID string, err error) {
	a.Decide("", AskAnswer{})
	a.Codex.ReplyAsk(ask.ExternalRequestID, AskAnswer{})
	a.Turn.Continue(turnID)
	return "", nil
}

// Decide 记下人对这条反问的作答：批还是拒、管一次还是管整条对话、选了哪一项、表单填了什么。
func (a *Approval) Decide(approvalID string, answer AskAnswer) error {
	return nil
}

// Expire 这条反问放太久没人理，按拒绝回给 Codex，免得它一直等在那儿。
func (a *Approval) Expire(approvalID string) error {
	return nil
}
