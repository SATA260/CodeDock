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

// AgentMode 控制 Run 提供的能力（从而决定可调用哪些工具）以及审批行为。
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
	Provider string          `json:"provider"`
	Model    string          `json:"model"`
	Options  json.RawMessage `json:"options,omitempty"` // 供应商特有参数，核心运行时不解析
}

// RetryConfig 配置一类可独立重试的操作。
type RetryConfig struct {
	MaxAttempts    int           `json:"max_attempts"`
	InitialBackoff time.Duration `json:"initial_backoff"`
	MaxBackoff     time.Duration `json:"max_backoff"`
	Multiplier     float64       `json:"multiplier"`
	Jitter         float64       `json:"jitter"`
}

// RetryPolicy 分别冻结上下文、模型和工具的重试设置。
type RetryPolicy struct {
	Context RetryConfig `json:"context"`
	Model   RetryConfig `json:"model"`
	Tool    RetryConfig `json:"tool"`
}

// RunLimits 是一次 Run 的不可变执行预算。
type RunLimits struct {
	MaxWallTime      time.Duration `json:"max_wall_time"`
	MaxTurns         int           `json:"max_turns"`
	MaxToolCalls     int           `json:"max_tool_calls"`
	MaxInputTokens   int64         `json:"max_input_tokens"`
	MaxOutputTokens  int64         `json:"max_output_tokens"`
	MaxParallelTools int           `json:"max_parallel_tools"`
}

// RunConfigSnapshot 是 Run 启动时保存的不可变配置。
type RunConfigSnapshot struct {
	Mode              AgentMode             `json:"mode"`
	SystemPromptHash  string                `json:"system_prompt_hash"`
	Model             ModelConfig           `json:"model"`
	ToolSetVersion    string                `json:"tool_set_version"`
	PermissionPolicy  tool.PermissionPolicy `json:"permission_policy"`
	ApprovalPolicy    tool.ApprovalPolicy   `json:"approval_policy"`
	RetryPolicy       RetryPolicy           `json:"retry_policy"`
	Limits            RunLimits             `json:"limits"`
	ToolExecutionMode tool.ExecutionMode    `json:"tool_execution_mode"`
	ToolFailurePolicy tool.FailurePolicy    `json:"tool_failure_policy"`
	Profile           profile.Config        `json:"profile"`
}

// Session 是长期存在的对话容器。
type Session struct {
	ID            string        `json:"id"`
	TenantID      string        `json:"tenant_id"`
	UserID        string        `json:"user_id"`
	AgentID       string        `json:"agent_id"`
	WorkspaceID   string        `json:"workspace_id"`
	Status        SessionStatus `json:"status"`
	ActiveRunID   *string       `json:"active_run_id,omitempty"`
	LastEventSeq  int64         `json:"last_event_seq"`
	CompactionSeq int64         `json:"compaction_seq"`
	Summary       string        `json:"summary"`
	CreatedAt     time.Time     `json:"created_at"`
	UpdatedAt     time.Time     `json:"updated_at"`
}

// Run 是 Session 内由用户触发的一次 Agent 执行。
type Run struct {
	ID               string            `json:"id"`
	SessionID        string            `json:"session_id"`
	TriggerMessageID string            `json:"trigger_message_id"`
	Mode             AgentMode         `json:"mode"`
	Config           RunConfigSnapshot `json:"config"`
	Status           RunStatus         `json:"status"`
	CurrentTurnID    *string           `json:"current_turn_id,omitempty"`
	StopReason       *StopReason       `json:"stop_reason,omitempty"`
	CancelRequested  bool              `json:"cancel_requested"`
	StartedAt        *time.Time        `json:"started_at,omitempty"`
	FinishedAt       *time.Time        `json:"finished_at,omitempty"`
}

// Turn 是 Run 内的一次模型调用。
type Turn struct {
	ID             string     `json:"id"`
	RunID          string     `json:"run_id"`
	Number         int        `json:"number"`
	Status         TurnStatus `json:"status"`
	FirstEventSeq  int64      `json:"first_event_seq"`
	LastEventSeq   int64      `json:"last_event_seq"`
	AssistantMsgID *string    `json:"assistant_msg_id,omitempty"`
	UsageID        *string    `json:"usage_id,omitempty"`
	StartedAt      *time.Time `json:"started_at,omitempty"`
	FinishedAt     *time.Time `json:"finished_at,omitempty"`
}

// Attachment 描述与消息关联的用户输入附件。
type Attachment struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	MediaType string `json:"media_type"`
	URI       string `json:"uri"`
	Size      int64  `json:"size"`
}

// Message 是持久化的用户、助手、工具或系统消息。
type Message struct {
	ID          string          `json:"id"`
	SessionID   string          `json:"session_id"`
	RunID       *string         `json:"run_id,omitempty"`
	TurnID      *string         `json:"turn_id,omitempty"`
	Role        MessageRole     `json:"role"`
	Content     json.RawMessage `json:"content"`
	Attachments []Attachment    `json:"attachments,omitempty"`
	ToolCalls   []tool.Call     `json:"tool_calls,omitempty"`
	EventSeq    int64           `json:"event_seq"`
	CreatedAt   time.Time       `json:"created_at"`
}

// CompactionSummary 是上下文快照引用的结构化摘要。
type CompactionSummary struct {
	CheckpointID string `json:"checkpoint_id"`
	Content      string `json:"content"`
	BaseEventSeq int64  `json:"base_event_seq"`
}

// ContextSnapshot 是为一次 Turn 装配的上下文。
type ContextSnapshot struct {
	SessionID       string             `json:"session_id"`
	BaseEventSeq    int64              `json:"base_event_seq"`
	Summary         *CompactionSummary `json:"summary,omitempty"`
	Messages        []Message          `json:"messages"`
	Tools           []tool.Definition  `json:"tools"`
	SystemPrompt    string             `json:"system_prompt"`
	MemoryIndexes   []string           `json:"memory_indexes,omitempty"`
	EstimatedTokens int64              `json:"estimated_tokens"`
	Version         int64              `json:"version"`
}

// CompactionCheckpoint 记录持久化的上下文摘要边界。
type CompactionCheckpoint struct {
	ID           string    `json:"id"`
	SessionID    string    `json:"session_id"`
	BaseEventSeq int64     `json:"base_event_seq"`
	Summary      string    `json:"summary"`
	CreatedByRun string    `json:"created_by_run"`
	CreatedAt    time.Time `json:"created_at"`
}

// ApprovalToolCall 是一条审批里的单个工具调用及其裁决。
type ApprovalToolCall struct {
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments,omitempty"`
	Status    ApprovalStatus  `json:"status,omitempty"`
	Reason    string          `json:"reason,omitempty"`
}

// Approval 记录等待用户裁决的一批工具调用。
type Approval struct {
	ID         string             `json:"id"`
	SessionID  string             `json:"session_id"`
	RunID      string             `json:"run_id"`
	ToolCallID string             `json:"tool_call_id"`
	ToolCalls  []ApprovalToolCall `json:"tool_calls"`
	Scope      ApprovalScope      `json:"scope"`
	Status     ApprovalStatus     `json:"status"`
	ExpiresAt  time.Time          `json:"expires_at"`
}

// AgentEvent 是 Agent 运行时产生的持久化有序事实。
type AgentEvent struct {
	EventID    string          `json:"event_id"`
	SessionID  string          `json:"session_id"`
	RunID      string          `json:"run_id"`
	TurnID     *string         `json:"turn_id,omitempty"`
	Seq        int64           `json:"seq"`
	Type       EventType       `json:"type"`
	Version    int             `json:"version"`
	OccurredAt time.Time       `json:"occurred_at"`
	Payload    json.RawMessage `json:"payload"`
}

// UsageRecord 保存单次请求的归一化用量与供应商原始用量。
type UsageRecord struct {
	ID                       string          `json:"id"`
	SessionID                string          `json:"session_id"`
	RunID                    string          `json:"run_id"`
	TurnID                   string          `json:"turn_id"`
	RequestID                string          `json:"request_id"`
	Provider                 string          `json:"provider"`
	Model                    string          `json:"model"`
	UsageType                string          `json:"usage_type"`
	CacheCreationInputTokens int64           `json:"cache_creation_input_tokens"`
	CacheReadInputTokens     int64           `json:"cache_read_input_tokens"`
	OutputTokens             int64           `json:"output_tokens"`
	ReasoningTokens          int64           `json:"reasoning_tokens"`
	TotalTokens              int64           `json:"total_tokens"`
	Estimated                bool            `json:"estimated"`
	RawProviderUsage         json.RawMessage `json:"raw_provider_usage,omitempty"`
	CreatedAt                time.Time       `json:"created_at"`
}
