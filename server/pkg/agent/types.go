package agent

import (
	"encoding/json"
	"time"

	"codedock/pkg/agent/profile"
	"codedock/pkg/agent/tool"
)

// SessionStatus 表示长期对话会话的生命周期状态。
type SessionStatus string

const (
	SessionActive   SessionStatus = "active"
	SessionArchived SessionStatus = "archived"
)

// AgentMode 控制 Run 可使用的工具与审批行为。
type AgentMode string

const (
	ModeAskForApproval AgentMode = "ask_for_approval"
	ModeAutoApprove    AgentMode = "auto_approve"
	ModeYolo           AgentMode = "yolo"
	ModeAsk            AgentMode = "ask"
	ModePlan           AgentMode = "plan"
)

// RunStatus 表示一次用户触发执行的状态机。
type RunStatus string

const (
	RunQueued          RunStatus = "queued"
	RunLoadingContext  RunStatus = "loading_context"
	RunRunningLLM      RunStatus = "running_llm"
	RunExecutingTools  RunStatus = "executing_tools"
	RunWaitingApproval RunStatus = "waiting_approval"
	RunCancelling      RunStatus = "cancelling"
	RunCompleted       RunStatus = "completed"
	RunFailed          RunStatus = "failed"
	RunCancelled       RunStatus = "cancelled"
)

// StopReason 描述 Run 进入终态的原因。
type StopReason string

const (
	StopCompleted      StopReason = "completed"
	StopCancelled      StopReason = "cancelled"
	StopTimeout        StopReason = "timeout"
	StopBudgetExceeded StopReason = "budget_exceeded"
	StopMaxTurns       StopReason = "max_turns"
	StopToolError      StopReason = "tool_error"
	StopModelError     StopReason = "model_error"
	StopApprovalDenied StopReason = "approval_denied"
)

// TurnStatus 表示单次模型调用的生命周期状态。
type TurnStatus string

const (
	TurnPending         TurnStatus = "pending"
	TurnRunning         TurnStatus = "running"
	TurnWaitingApproval TurnStatus = "waiting_approval"
	TurnCompleted       TurnStatus = "completed"
	TurnFailed          TurnStatus = "failed"
	TurnCancelled       TurnStatus = "cancelled"
)

// MessageRole 标识持久化消息的来源角色。
type MessageRole string

const (
	RoleUser      MessageRole = "user"
	RoleAssistant MessageRole = "assistant"
	RoleTool      MessageRole = "tool"
	RoleSystem    MessageRole = "system"
)

// ApprovalScope 控制审批决定的生效范围。
type ApprovalScope string

const (
	ApprovalOnce    ApprovalScope = "once"
	ApprovalForRun  ApprovalScope = "run"
	ApprovalSession ApprovalScope = "session"
)

// ApprovalStatus 表示审批请求的生命周期状态。
type ApprovalStatus string

const (
	ApprovalPending  ApprovalStatus = "pending"
	ApprovalApproved ApprovalStatus = "approved"
	ApprovalDenied   ApprovalStatus = "denied"
	ApprovalExpired  ApprovalStatus = "expired"
)

// EventType 标识持久化 Agent 事件的载荷结构。
type EventType string

const (
	EventRunCreated           EventType = "run.created"
	EventRunStateChanged      EventType = "run.state_changed"
	EventTurnStarted          EventType = "turn.started"
	EventAssistantStarted     EventType = "assistant.started"
	EventAssistantDelta       EventType = "assistant.delta"
	EventAssistantCompleted   EventType = "assistant.completed"
	EventToolCallStarted      EventType = "tool.call_started"
	EventApprovalRequired     EventType = "tool.approval_required"
	EventApprovalDecided      EventType = "tool.approval_decided"
	EventToolExecutionStarted EventType = "tool.execution_started"
	EventToolExecutionRetry   EventType = "tool.execution_retry"
	EventToolExecutionResult  EventType = "tool.execution_result"
	EventUsageRecorded        EventType = "turn.usage_recorded"
	EventContextCompacted     EventType = "context.compacted"
	EventTurnCompleted        EventType = "turn.completed"
	EventRunCompleted         EventType = "run.completed"
	EventRunFailed            EventType = "run.failed"
	EventRunCancelled         EventType = "run.cancelled"
)

// ModelConfig 冻结 Run 使用的供应商无关模型配置。
type ModelConfig struct {
	Provider string
	Model    string
	Options  json.RawMessage // 供应商特有参数，核心运行时不解析
}

// RetryConfig 配置一类可独立重试的操作。
type RetryConfig struct {
	MaxAttempts    int
	InitialBackoff time.Duration
	MaxBackoff     time.Duration
	Multiplier     float64
	Jitter         float64
}

// RetryPolicy 分别冻结上下文、模型和工具的重试设置。
type RetryPolicy struct {
	Context RetryConfig
	Model   RetryConfig
	Tool    RetryConfig
}

// RunLimits 是一次 Run 的不可变执行预算。
type RunLimits struct {
	MaxWallTime      time.Duration
	MaxTurns         int
	MaxToolCalls     int
	MaxInputTokens   int64
	MaxOutputTokens  int64
	MaxParallelTools int
}

// RunConfigSnapshot 是 Run 启动时保存的不可变配置。
type RunConfigSnapshot struct {
	Mode              AgentMode
	SystemPromptHash  string
	Model             ModelConfig
	ToolSetVersion    string
	PermissionPolicy  tool.PermissionPolicy
	ApprovalPolicy    tool.ApprovalPolicy
	RetryPolicy       RetryPolicy
	Limits            RunLimits
	ToolExecutionMode tool.ExecutionMode
	ToolFailurePolicy tool.FailurePolicy
	Profile           profile.Config
}

// Session 是长期存在的对话容器。
type Session struct {
	ID            string
	TenantID      string
	UserID        string
	AgentID       string
	WorkspaceID   string
	Status        SessionStatus
	ActiveRunID   *string
	LastEventSeq  int64
	CompactionSeq int64
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// Run 是 Session 内由用户触发的一次 Agent 执行。
type Run struct {
	ID               string
	SessionID        string
	TriggerMessageID string
	Mode             AgentMode
	Config           RunConfigSnapshot
	Status           RunStatus
	CurrentTurnID    *string
	StopReason       *StopReason
	CancelRequested  bool
	StartedAt        *time.Time
	FinishedAt       *time.Time
}

// Turn 是 Run 内的一次模型调用。
type Turn struct {
	ID             string
	RunID          string
	Number         int
	Status         TurnStatus
	FirstEventSeq  int64
	LastEventSeq   int64
	AssistantMsgID *string
	UsageID        *string
	StartedAt      *time.Time
	FinishedAt     *time.Time
}

// Attachment 描述与消息关联的用户输入附件。
type Attachment struct {
	ID        string
	Name      string
	MediaType string
	URI       string
	Size      int64
}

// Message 是持久化的用户、助手、工具或系统消息。
type Message struct {
	ID          string
	SessionID   string
	RunID       *string
	TurnID      *string
	Role        MessageRole
	Content     json.RawMessage
	Attachments []Attachment
	ToolCalls   []tool.Call
	EventSeq    int64
	CreatedAt   time.Time
}

// CompactionSummary 是上下文快照引用的结构化摘要。
type CompactionSummary struct {
	CheckpointID string
	Content      string
	BaseEventSeq int64
}

// ContextSnapshot 是为一次 Turn 装配的上下文。
type ContextSnapshot struct {
	SessionID       string
	BaseEventSeq    int64
	Summary         *CompactionSummary
	Messages        []Message
	Tools           []tool.Definition
	SystemPrompt    string
	EstimatedTokens int64
	Version         int64
}

// CompactionCheckpoint 记录持久化的上下文摘要边界。
type CompactionCheckpoint struct {
	ID           string
	SessionID    string
	BaseEventSeq int64
	Summary      string
	CreatedByRun string
	CreatedAt    time.Time
}

// Approval 记录等待用户裁决的工具调用。
type Approval struct {
	ID         string
	SessionID  string
	RunID      string
	ToolCallID string
	Scope      ApprovalScope
	Status     ApprovalStatus
	ExpiresAt  time.Time
}

// AgentEvent 是 Agent 运行时产生的持久化有序事实。
type AgentEvent struct {
	EventID    string
	SessionID  string
	RunID      string
	TurnID     *string
	Seq        int64
	Type       EventType
	Version    int
	OccurredAt time.Time
	Payload    json.RawMessage
}

// UsageRecord 保存单次请求的归一化用量与供应商原始用量。
type UsageRecord struct {
	ID                       string
	SessionID                string
	RunID                    string
	TurnID                   string
	RequestID                string
	Provider                 string
	Model                    string
	UsageType                string
	CacheCreationInputTokens int64
	CacheReadInputTokens     int64
	OutputTokens             int64
	ReasoningTokens          int64
	TotalTokens              int64
	Estimated                bool
	RawProviderUsage         json.RawMessage
	CreatedAt                time.Time
}
