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
	AskCommand     AskKind = "command"     // 能不能跑这条命令。
	AskFileChange  AskKind = "file_change" // 能不能改这些文件。
	AskQuestion    AskKind = "question"    // 让人补一句字，或从几个选项里挑一个。
	AskForm        AskKind = "form"        // MCP 跑起来之后弹出来、要人填的表单。
	AskPermissions AskKind = "permissions" // 额外权限。
)

// ApprovalAsk 是一条 Codex 已知的反问。
type ApprovalAsk struct {
	ID                string   `json:"id"`
	Kind              AskKind  `json:"kind"`
	ThreadID          string   `json:"thread_id,omitempty"`
	TurnID            string   `json:"turn_id,omitempty"`
	Method            string   `json:"method,omitempty"`
	Command           string   `json:"command,omitempty"`
	Paths             []string `json:"paths,omitempty"`
	Diff              string   `json:"diff,omitempty"`
	Prompt            string   `json:"prompt,omitempty"` // 选择题或表单给人看的题面。
	Options           []string `json:"options,omitempty"`
	Fields            []string `json:"fields,omitempty"` // MCP 表单字段名。
	ExternalRequestID string   `json:"external_request_id"`
}

// AskAnswer 是人对这条反问的作答。
type AskAnswer struct {
	Approved bool          `json:"approved"`
	Scope    DecisionScope `json:"scope,omitempty"`
	Choice   string        `json:"choice,omitempty"` // 选择题选中的项。
	Values   []string      `json:"values,omitempty"` // 表单填写结果。
}

// KnownAskMethod 判断这是不是本模块能做完整界面的官方反问。
func KnownAskMethod(method string) bool {
	switch method {
	case MethodItemCommandApproval, MethodItemFileApproval, MethodItemPermissionsApproval,
		MethodItemToolUserInput, MethodMCPElicitation,
		MethodExecCommandApproval, MethodApplyPatchApproval:
		return true
	default:
		return false
	}
}
