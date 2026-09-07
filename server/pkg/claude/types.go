package claude

// 本包对接本机 Claude Code。类型与内部拆分对齐；会话、实录、模型、权限档和配置从本机 Claude 读，不落库。

// EngineStatus 是本机 Claude Code 是否可用来开回合。
type EngineStatus struct {
	Available  bool   `json:"available"`
	Authorized bool   `json:"authorized"`
	Version    string `json:"version"`
	Hint       string `json:"hint"` // 不可用时给人看的原因，如未安装或未授权。
}

// ModelInfo 是一条 Claude Code 模型及其推理强度。
type ModelInfo struct {
	ID            string   `json:"id"`
	Efforts       []string `json:"efforts"` // 该 Claude 模型支持的推理强度。
	DefaultEffort string   `json:"default_effort"`
	Hidden        bool     `json:"hidden"`
	IsDefault     bool     `json:"is_default"`
}

// ModeInfo 是一条 Claude Code 官方权限档。
type ModeInfo struct {
	ID   string `json:"id"`   // default、acceptEdits、plan、auto、dontAsk、bypassPermissions。
	Kind string `json:"kind"` // permission。
}

// Session 是一条只走 Claude Code 的对话；字段从本机 Claude 读，不落本模块的库。
type Session struct {
	ID              string `json:"id"`
	ClaudeSessionID string `json:"claude_session_id"` // Claude session 编号，首次开回合后才有。
	Title           string `json:"title"`
	ActiveTurnID    string `json:"active_turn_id"` // 同时只能有一个进行中的回合。
	Archived        bool   `json:"archived"`
}

// Settings 是这个对话里生效的 Claude 配置；只有改过的项交给 Claude Code。
type Settings struct {
	Model          string   `json:"model"`
	Effort         string   `json:"effort"`
	PermissionMode string   `json:"permission_mode"` // Claude 的官方权限档。
	Cwd            string   `json:"cwd"`             // 仓库根。
	Overridden     []string `json:"overridden"`      // 用户改过、需要交给 Claude Code 的字段名。
}

// CommandAction 是斜杠名与官方扩展按钮共用的动作种类。
type CommandAction string // apply_settings | turn | session | attach | hint

const (
	CommandApplySettings CommandAction = "apply_settings"
	CommandTurn          CommandAction = "turn"
	CommandSession       CommandAction = "session"
	CommandAttach        CommandAction = "attach"
	CommandHint          CommandAction = "hint"
)

// CommandSpec 是一条与 Claude 斜杠、官方扩展按钮同名的命令。
type CommandSpec struct {
	Name   string        `json:"name"` // 与 Claude 斜杠同名，如 model、plan、branch、mcp。
	Action CommandAction `json:"action"`
	Hint   string        `json:"hint"` // hint 时给人看的话。
}

// CommandResult 是不能在本模块落地时给人看的说明。
type CommandResult struct {
	Hint string `json:"hint"`
}

// Input 是本条将要带给 Claude Code 的正文、文件提及和图片。
type Input struct {
	Text     string   `json:"text"`
	Mentions []string `json:"mentions"` // 仓库内路径，对 Claude Code 的文件提及。
	Images   []string `json:"images"`   // 本地图片路径。
}

// InputMode 决定空闲则开新一轮，还是进行中只排队。
type InputMode string // start | queue

const (
	InputModeStart InputMode = "start"
	InputModeQueue InputMode = "queue"
)

// TurnStatus 是一轮 Claude Code 工作的状态。
type TurnStatus string // queued | running | waiting_approval | completed | failed | cancelled

const (
	TurnQueued          TurnStatus = "queued"
	TurnRunning         TurnStatus = "running"
	TurnWaitingApproval TurnStatus = "waiting_approval"
	TurnCompleted       TurnStatus = "completed"
	TurnFailed          TurnStatus = "failed"
	TurnCancelled       TurnStatus = "cancelled"
)

// Turn 是一次用户请求对应的那一轮 Claude Code 工作。
type Turn struct {
	ID        string     `json:"id"`
	SessionID string     `json:"session_id"`
	Status    TurnStatus `json:"status"`
}

// ProgressKind 是给人看的 Claude Code 进展种类。
type ProgressKind string // user | text | reasoning | command | file_change | plan | notice

const (
	ProgressKindUser       ProgressKind = "user"
	ProgressKindText       ProgressKind = "text"
	ProgressKindReasoning  ProgressKind = "reasoning"
	ProgressKindCommand    ProgressKind = "command"
	ProgressKindFileChange ProgressKind = "file_change"
	ProgressKindPlan       ProgressKind = "plan"
	ProgressKindNotice     ProgressKind = "notice"
)

// Progress 是一条给人看、从本机 Claude 回放的进展。
type Progress struct {
	Kind    ProgressKind `json:"kind"`
	Text    string       `json:"text"`
	Command string       `json:"command"`
	Paths   []string     `json:"paths"`
	Diff    string       `json:"diff"`
}

// DecisionScope 是人对已知反问的生效范围。
type DecisionScope string // once | session

const (
	DecisionOnce    DecisionScope = "once"
	DecisionSession DecisionScope = "session"
)

// AskKind 是 Claude Code 已知的反问种类。
type AskKind string // command | file_change | question | form

const (
	AskKindCommand    AskKind = "command"
	AskKindFileChange AskKind = "file_change"
	AskKindQuestion   AskKind = "question"
	AskKindForm       AskKind = "form"
)

// ApprovalAsk 是一条 Claude Code 已知的反问。
type ApprovalAsk struct {
	Kind              AskKind  `json:"kind"`
	Command           string   `json:"command"`
	Paths             []string `json:"paths"`
	Diff              string   `json:"diff"`
	Prompt            string   `json:"prompt"`              // 选择题或表单给人看的题面。
	Options           []string `json:"options"`             // 选择题的选项。
	Fields            []string `json:"fields"`              // MCP 表单字段名。
	ExternalRequestID string   `json:"external_request_id"` // 用来回给 Claude Code 的那张问票。
}

// AskAnswer 是人对已知反问的作答。
type AskAnswer struct {
	Approved bool          `json:"approved"`
	Scope    DecisionScope `json:"scope"`
	Choice   string        `json:"choice"` // 选择题选中的项。
	Values   []string      `json:"values"` // 表单填写结果。
}
