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
	SessionActive   SessionStatus = "active"   // 会话可接收新消息
	SessionArchived SessionStatus = "archived" // 会话已归档，禁止新建 Run
)

// WorkMode 选出本轮使用的内置 Agent。
type WorkMode string

const (
	WorkAsk   WorkMode = "ask"   // 只读问答
	WorkPlan  WorkMode = "plan"  // 只写 .cursor 计划
	WorkAgent WorkMode = "agent" // 可改仓库 / shell / 记忆
)

// ApprovalMode 是审批流水线第三层。
type ApprovalMode = tool.ApprovalMode

const (
	ApprovalManual = tool.ApprovalManual
	ApprovalAuto   = tool.ApprovalAuto
	ApprovalYolo   = tool.ApprovalYolo
)

// RunStatus 表示一次用户触发执行的状态机。
type RunStatus string

const (
	RunQueued          RunStatus = "queued"           // 等待被会话 active 队列取出
	RunLoadingContext  RunStatus = "loading_context"  // 准备上下文与可见工具
	RunRunningLLM      RunStatus = "running_llm"      // 正在流式调用模型
	RunExecutingTools  RunStatus = "executing_tools"  // 正在执行模型下发的一批工具
	RunWaitingApproval RunStatus = "waiting_approval" // 工具批次等待用户审批
	RunVerifying       RunStatus = "verifying"        // 正在跑收尾验证
	RunEvaluating      RunStatus = "evaluating"       // 正在做独立旁路复审
	RunCancelling      RunStatus = "cancelling"       // 已请求取消，正在收尾
	RunCompleted       RunStatus = "completed"        // 正常结束
	RunFailed          RunStatus = "failed"           // 执行失败
	RunCancelled       RunStatus = "cancelled"        // 被取消
)

// StopReason 描述 Run 进入终态的原因。
type StopReason string

const (
	StopCompleted      StopReason = "completed"        // 正常完成
	StopCancelled      StopReason = "cancelled"        // 用户取消或中断
	StopTimeout        StopReason = "timeout"          // 超过最大 wall time
	StopBudgetExceeded StopReason = "budget_exceeded"  // 超过 token 预算
	StopMaxTurns       StopReason = "max_turns"        // 超过最大轮数
	StopToolError      StopReason = "tool_error"       // 工具执行失败导致结束
	StopModelError     StopReason = "model_error"      // 模型调用失败导致结束
	StopApprovalDenied StopReason = "approval_denied"  // 审批被拒绝
	StopAcceptedByUser StopReason = "accepted_by_user" // 验证/复审熔断后用户强制收工
)

// TurnStatus 表示单次模型调用的生命周期状态。
type TurnStatus string

const (
	TurnPending         TurnStatus = "pending"          // 尚未开始
	TurnRunning         TurnStatus = "running"          // 模型调用中
	TurnWaitingApproval TurnStatus = "waiting_approval" // 本轮工具待审批
	TurnCompleted       TurnStatus = "completed"        // 本轮正常完成
	TurnFailed          TurnStatus = "failed"           // 本轮失败
	TurnCancelled       TurnStatus = "cancelled"        // 本轮取消
)

// MessageRole 标识持久化消息的来源角色。
type MessageRole string

const (
	RoleUser      MessageRole = "user"      // 用户输入
	RoleAssistant MessageRole = "assistant" // 助手回复（文本或工具调用）
	RoleTool      MessageRole = "tool"      // 工具执行结果
	RoleSystem    MessageRole = "system"    // 系统提示、记忆目录、压缩摘要等
	RoleDeveloper MessageRole = "developer" // 本轮模式规则；Build 注入，不落库
)

// ApprovalScope 控制审批决定的生效范围。
type ApprovalScope string

const (
	ApprovalOnce    ApprovalScope = "once"    // 仅本次工具批次有效
	ApprovalForRun  ApprovalScope = "run"     // 同一 Run 内同类工具持续有效
	ApprovalSession ApprovalScope = "session" // 整个会话内同类工具持续有效
)

// ApprovalStatus 表示审批请求的生命周期状态。
type ApprovalStatus string

const (
	ApprovalPending  ApprovalStatus = "pending"  // 待审批
	ApprovalApproved ApprovalStatus = "approved" // 已批准
	ApprovalDenied   ApprovalStatus = "denied"   // 已拒绝
	ApprovalExpired  ApprovalStatus = "expired"  // 已过期
)

// EventType 标识持久化 Agent 事件的载荷结构。
type EventType string

const (
	EventRunCreated           EventType = "run.created"            // Run 已创建（queued）
	EventRunStateChanged      EventType = "run.state_changed"      // Run 粗状态迁移
	EventTurnStarted          EventType = "turn.started"           // 一次模型 Turn 开始
	EventAssistantStarted     EventType = "assistant.started"      // 开始流式助手回复
	EventAssistantDelta       EventType = "assistant.delta"        // 助手流式增量（文本或工具调用）
	EventAssistantCompleted   EventType = "assistant.completed"    // 助手本轮输出结束
	EventToolCallStarted      EventType = "tool.call_started"      // 模型发出来的 tool_call 进入处理
	EventApprovalRequired     EventType = "tool.approval_required" // 工具批次需要人工审批
	EventApprovalDecided      EventType = "tool.approval_decided"  // 审批裁决已提交
	EventToolExecutionStarted EventType = "tool.execution_started" // 单个工具开始执行
	EventToolExecutionRetry   EventType = "tool.execution_retry"   // 单个工具重试
	EventToolExecutionResult  EventType = "tool.execution_result"  // 单个工具结果
	EventUsageRecorded        EventType = "turn.usage_recorded"    // 本 Turn 用量入账
	EventContextCompacted     EventType = "context.compacted"      // 上下文被压缩
	EventTurnCompleted        EventType = "turn.completed"         // 本 Turn 结束
	EventRunCompleted         EventType = "run.completed"          // Run 正常结束
	EventRunFailed            EventType = "run.failed"             // Run 失败
	EventRunCancelled         EventType = "run.cancelled"          // Run 取消
	EventVerifyStarted        EventType = "verify.started"         // 收尾验证开始
	EventVerifyResult         EventType = "verify.result"          // 收尾验证出结论
	EventVerifySkipped        EventType = "verify.skipped"         // 无验证规则，按通过处理
	EventEvaluateStarted      EventType = "evaluate.started"       // 独立复审开始
	EventEvaluateResult       EventType = "evaluate.result"        // 独立复审出结论
	EventSnapshotSkipped      EventType = "snapshot.skipped"       // 工作区不是 Git 仓库，无法拍快照
)

// ModelConfig 冻结 Run 使用的供应商无关模型配置。
type ModelConfig struct {
	Provider string          `json:"provider"`          // 供应商：fake / openai
	Model    string          `json:"model"`             // 模型名
	Options  json.RawMessage `json:"options,omitempty"` // 供应商特有参数，核心运行时不解析
}

// RetryConfig 配置一类可独立重试的操作。
type RetryConfig struct {
	MaxAttempts    int           `json:"max_attempts"`    // 最大尝试次数
	InitialBackoff time.Duration `json:"initial_backoff"` // 首次退避时长
	MaxBackoff     time.Duration `json:"max_backoff"`     // 最大退避时长
	Multiplier     float64       `json:"multiplier"`      // 退避乘数
	Jitter         float64       `json:"jitter"`          // 抖动比例
}

// RetryPolicy 分别冻结上下文、模型和工具的重试设置。
type RetryPolicy struct {
	Context RetryConfig `json:"context"` // 上下文加载/压缩重试
	Model   RetryConfig `json:"model"`   // 模型调用重试
	Tool    RetryConfig `json:"tool"`    // 工具执行重试
}

// RunLimits 是一次 Run 的不可变执行预算。
type RunLimits struct {
	MaxWallTime       time.Duration `json:"max_wall_time"`       // 最大执行时间
	MaxTurns          int           `json:"max_turns"`           // 最大模型调用轮数
	MaxToolCalls      int           `json:"max_tool_calls"`      // 最大工具调用次数
	MaxInputTokens    int64         `json:"max_input_tokens"`    // 最大输入 token 数（含上下文）
	MaxOutputTokens   int64         `json:"max_output_tokens"`   // 最大输出 token 数
	MaxParallelTools  int           `json:"max_parallel_tools"`  // 工具并行上限
	MaxVerifyRounds   int           `json:"max_verify_rounds"`   // 验证失败最多打回几次；0 表示用默认 3
	MaxEvaluateRounds int           `json:"max_evaluate_rounds"` // 复审驳回最多打回几次；0 表示用默认 2
}

// RunConfigSnapshot 是 Run 启动时保存的不可变配置。
type RunConfigSnapshot struct {
	Mode              WorkMode           `json:"mode"`                // 选中的内置 Agent
	Approval          ApprovalMode       `json:"approval"`            // 审批模式
	SystemPromptHash  string             `json:"system_prompt_hash"`  // 系统提示哈希
	Model             ModelConfig        `json:"model"`               // 模型配置
	ToolSetVersion    string             `json:"tool_set_version"`    // 工具集版本
	RetryPolicy       RetryPolicy        `json:"retry_policy"`        // 重试策略
	Limits            RunLimits          `json:"limits"`              // 执行预算
	ToolExecutionMode tool.ExecutionMode `json:"tool_execution_mode"` // 工具串行/并行模式
	ToolFailurePolicy tool.FailurePolicy `json:"tool_failure_policy"` // 工具失败策略
	Profile           profile.Config     `json:"profile"`             // Agent 配置
	EvaluatorModel    ModelConfig        `json:"evaluator_model"`     // 独立复审模型；空则回落主模型
	SubagentModel     ModelConfig        `json:"subagent_model"`      // explore 子代理模型；空则回落 EvaluatorModel
}

// Session 是长期存在的对话容器。
type Session struct {
	ID            string        `json:"id"`                      // 会话 ID
	TenantID      string        `json:"tenant_id"`               // 租户 ID
	UserID        string        `json:"user_id"`                 // 用户 ID
	AgentID       string        `json:"agent_id"`                // Agent 配置 ID
	WorkspaceID   string        `json:"workspace_id"`            // 创建时冻结的工作目录（绝对路径）；本会话权限只覆盖该目录
	Status        SessionStatus `json:"status"`                  // 会话状态
	ActiveRunID   *string       `json:"active_run_id,omitempty"` // 当前正在执行的 Run ID
	NeedsRecover  bool          `json:"needs_recover,omitempty"` // Handler 计算：active Run 已中断、需用户恢复；不入库
	LastEventSeq  int64         `json:"last_event_seq"`          // 已分配的最大事件序号
	CompactionSeq int64         `json:"compaction_seq"`          // 上次压缩对应的事件序号
	Summary       string        `json:"summary"`                 // 会话列表摘要（首条用户输入首行）
	CreatedAt     time.Time     `json:"created_at"`              // 创建时间
	UpdatedAt     time.Time     `json:"updated_at"`              // 更新时间
}

// Run 是 Session 内由用户触发的一次 Agent 执行。
type Run struct {
	ID               string            `json:"id"`                        // Run ID
	SessionID        string            `json:"session_id"`                // 所属会话
	TriggerMessageID string            `json:"trigger_message_id"`        // 触发 Run 的用户消息 ID
	Mode             WorkMode          `json:"mode"`                      // 选中的内置 Agent
	Approval         ApprovalMode      `json:"approval"`                  // 审批模式（来自快照）
	Config           RunConfigSnapshot `json:"config"`                    // 启动配置快照
	Status           RunStatus         `json:"status"`                    // 当前状态
	NeedsRecover     bool              `json:"needs_recover,omitempty"`   // Handler 计算：执行已中断且 Worker 不在跑；不入库
	CurrentTurnID    *string           `json:"current_turn_id,omitempty"` // 当前 Turn ID
	StopReason       *StopReason       `json:"stop_reason,omitempty"`     // 结束原因
	CancelRequested  bool              `json:"cancel_requested"`          // 是否已请求取消
	StartedAt        *time.Time        `json:"started_at,omitempty"`      // 开始时间
	FinishedAt       *time.Time        `json:"finished_at,omitempty"`     // 结束时间
}

// Turn 是 Run 内的一次模型调用。
type Turn struct {
	ID             string     `json:"id"`                         // Turn ID
	RunID          string     `json:"run_id"`                     // 所属 Run
	Number         int        `json:"number"`                     // 第几轮（从 1 开始）
	Status         TurnStatus `json:"status"`                     // 当前状态
	FirstEventSeq  int64      `json:"first_event_seq"`            // 本轮第一个事件 seq
	LastEventSeq   int64      `json:"last_event_seq"`             // 本轮最后一个事件 seq
	AssistantMsgID *string    `json:"assistant_msg_id,omitempty"` // 助手消息 ID
	UsageID        *string    `json:"usage_id,omitempty"`         // 用量记录 ID
	StartedAt      *time.Time `json:"started_at,omitempty"`       // 开始时间
	FinishedAt     *time.Time `json:"finished_at,omitempty"`      // 结束时间
}

// Attachment 描述与消息关联的用户输入附件。
type Attachment struct {
	ID        string `json:"id"`         // 附件 ID
	Name      string `json:"name"`       // 文件名
	MediaType string `json:"media_type"` // MIME 类型
	URI       string `json:"uri"`        // 存储地址
	Size      int64  `json:"size"`       // 字节大小
}

// Message 是持久化的用户、助手、工具或系统消息。
type Message struct {
	ID          string          `json:"id"`                    // 消息 ID
	SessionID   string          `json:"session_id"`            // 所属会话
	RunID       *string         `json:"run_id,omitempty"`      // 所属 Run（可选）
	TurnID      *string         `json:"turn_id,omitempty"`     // 所属 Turn（可选）
	Role        MessageRole     `json:"role"`                  // 角色
	Content     json.RawMessage `json:"content"`               // 内容（统一结构）
	Attachments []Attachment    `json:"attachments,omitempty"` // 附件
	ToolCalls   []tool.Call     `json:"tool_calls,omitempty"`  // 助手消息的工具调用
	EventSeq    int64           `json:"event_seq"`             // 对应事件序号
	CreatedAt   time.Time       `json:"created_at"`            // 创建时间
}

// CompactionSummary 是上下文快照引用的结构化摘要。
type CompactionSummary struct {
	CheckpointID string `json:"checkpoint_id"`  // 压缩检查点 ID
	Content      string `json:"content"`        // 摘要内容
	BaseEventSeq int64  `json:"base_event_seq"` // 摘要涵盖到的事件序号
}

// ContextSnapshot 是为一次 Turn 装配的上下文。
type ContextSnapshot struct {
	SessionID       string             `json:"session_id"`               // 所属会话
	BaseEventSeq    int64              `json:"base_event_seq"`           // 上下文起点事件序号
	Summary         *CompactionSummary `json:"summary,omitempty"`        // 压缩摘要
	Messages        []Message          `json:"messages"`                 // 历史消息
	Tools           []tool.Definition  `json:"tools"`                    // 本轮可见工具定义
	SystemPrompt    string             `json:"system_prompt"`            // 注入的系统提示（身份段；Compose 再拼工具与 Guidelines）
	WorkspaceRoot   string             `json:"workspace_root,omitempty"` // 会话冻结的工作目录，写入 Current working directory
	ActivePlan      string             `json:"active_plan,omitempty"`    // 本会话绑定的计划；注入 developer 范围说明
	Hidden          []Message          `json:"hidden,omitempty"`         // 插件注入的隐藏消息，不入库
	MemoryIndexes   []string           `json:"memory_indexes,omitempty"` // 冻结记忆目录
	Packet          string             `json:"packet,omitempty"`         // Work Packet，独立 system，不入库、不抄进记忆
	EstimatedTokens int64              `json:"estimated_tokens"`         // 估算 token 数
	Version         int64              `json:"version"`                  // 快照版本
}

// CompactionCheckpoint 记录持久化的上下文摘要边界。
type CompactionCheckpoint struct {
	ID           string    `json:"id"`             // 检查点 ID
	SessionID    string    `json:"session_id"`     // 所属会话
	BaseEventSeq int64     `json:"base_event_seq"` // 摘要起点事件序号
	Summary      string    `json:"summary"`        // 摘要内容
	CreatedByRun string    `json:"created_by_run"` // 创建该检查点的 Run ID
	CreatedAt    time.Time `json:"created_at"`     // 创建时间
}

// ApprovalToolCall 是一条审批里的单个工具调用及其裁决。
type ApprovalToolCall struct {
	ID        string          `json:"id"`                  // tool_call_id
	Name      string          `json:"name"`                // 工具名
	Arguments json.RawMessage `json:"arguments,omitempty"` // 参数
	Status    ApprovalStatus  `json:"status,omitempty"`    // 裁决状态
	Reason    string          `json:"reason,omitempty"`    // 裁决理由
}

// Approval 记录等待用户裁决的一批工具调用。
type Approval struct {
	ID         string             `json:"id"`                 // 审批 ID
	SessionID  string             `json:"session_id"`         // 所属会话
	RunID      string             `json:"run_id"`             // 所属 Run
	ToolCallID string             `json:"tool_call_id"`       // 首个 tool_call_id
	ToolCalls  []ApprovalToolCall `json:"tool_calls"`         // 全部工具调用及裁决
	Scope      ApprovalScope      `json:"scope"`              // 生效范围
	Status     ApprovalStatus     `json:"status"`             // 审批状态
	ExpiresAt  time.Time          `json:"expires_at"`         // 过期时间
	Kind       ApprovalKind       `json:"kind,omitempty"`     // tools / verify / evaluate；空视为 tools
	Override   OverrideAction     `json:"override,omitempty"` // 验证/复审单的人工动作
}

// AgentEvent 是 Agent 运行时产生的持久化有序事实。
type AgentEvent struct {
	EventID    string          `json:"event_id"`          // 事件 ID
	SessionID  string          `json:"session_id"`        // 所属会话
	RunID      string          `json:"run_id"`            // 所属 Run
	TurnID     *string         `json:"turn_id,omitempty"` // 所属 Turn（可选）
	Seq        int64           `json:"seq"`               // 会话内严格递增序号
	Type       EventType       `json:"type"`              // 事件类型
	Version    int             `json:"version"`           // 载荷版本
	OccurredAt time.Time       `json:"occurred_at"`       // 发生时间
	Payload    json.RawMessage `json:"payload"`           // 类型化载荷
}

// UsageRecord 保存单次请求的归一化用量与供应商原始用量。
type UsageRecord struct {
	ID                       string          `json:"id"`                           // 用量记录 ID
	SessionID                string          `json:"session_id"`                   // 所属会话
	RunID                    string          `json:"run_id"`                       // 所属 Run
	TurnID                   string          `json:"turn_id"`                      // 所属 Turn
	RequestID                string          `json:"request_id"`                   // 供应商请求 ID
	Provider                 string          `json:"provider"`                     // 供应商
	Model                    string          `json:"model"`                        // 模型名
	UsageType                string          `json:"usage_type"`                   // 用量类型
	CacheCreationInputTokens int64           `json:"cache_creation_input_tokens"`  // 缓存创建输入 token
	CacheReadInputTokens     int64           `json:"cache_read_input_tokens"`      // 缓存读取输入 token
	OutputTokens             int64           `json:"output_tokens"`                // 输出 token
	ReasoningTokens          int64           `json:"reasoning_tokens"`             // 推理 token
	TotalTokens              int64           `json:"total_tokens"`                 // 总 token
	Estimated                bool            `json:"estimated"`                    // 是否估算
	RawProviderUsage         json.RawMessage `json:"raw_provider_usage,omitempty"` // 供应商原始用量
	CreatedAt                time.Time       `json:"created_at"`                   // 创建时间
}
