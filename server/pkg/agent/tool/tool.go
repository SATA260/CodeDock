package tool

import (
	"context"
	"encoding/json"
	"errors"
)

// ErrOutsideWorkspace 表示路径落在会话工作区外。第 1 层校验不算失败，但必须走审批。
var ErrOutsideWorkspace = errors.New("outside workspace")

// ExecutionMode 定义一组工具调用采用串行还是并行执行。
type ExecutionMode string

const (
	ExecutionSerial   ExecutionMode = "serial"
	ExecutionParallel ExecutionMode = "parallel"
)

// FailurePolicy 定义某个工具调用失败后整组调用的处理方式。
// 失败本身是 Result，不是 error；fail_fast 只表示不再执行后续调用。
type FailurePolicy string

const (
	FailureFast       FailurePolicy = "fail_fast"
	FailureCollectAll FailurePolicy = "collect_all"
	FailureBestEffort FailurePolicy = "best_effort"
)

// Effect 是审批流水线每一层的结果。
type Effect string

const (
	EffectAllow Effect = "allow" // 放行，后续层不跑
	EffectAsk   Effect = "ask"   // 交给下一层
	EffectDeny  Effect = "deny"  // 不可调用，后续层不跑
)

// ApprovalMode 是流水线第三层：谁来裁定仍为 ask 的调用。
type ApprovalMode string

const (
	ApprovalManual ApprovalMode = "manual" // 开单等人
	ApprovalAuto   ApprovalMode = "auto"   // 开单后独立复审
	ApprovalYolo   ApprovalMode = "yolo"   // 本层直接 allow
)

// Permission 是工具自带的默认权限；只能是 allow 或 ask。
type Permission struct {
	Effect   Effect `json:"effect"`             // 默认 Effect；不能写成 deny
	Resource string `json:"resource,omitempty"` // 可选资源标签，流水线不读
}

// Definition 是模型可见的统一工具描述。
type Definition struct {
	Name             string          `json:"name"`
	Prompt           string          `json:"prompt,omitempty"`
	ParametersSchema json.RawMessage `json:"parameters_schema,omitempty"`
	OutputSchema     json.RawMessage `json:"output_schema,omitempty"`
	Permission       Permission      `json:"permission"`
	SupportsCancel   bool            `json:"supports_cancel"`
	SupportsRetry    bool            `json:"supports_retry"`
	Version          string          `json:"version,omitempty"`
}

// Prompt 是注册中心返回给模型层的统一工具提示词。
type Prompt struct {
	Name             string          `json:"name"`
	Content          string          `json:"content"`
	ParametersSchema json.RawMessage `json:"parameters_schema,omitempty"`
	OutputSchema     json.RawMessage `json:"output_schema,omitempty"`
}

// Reference 是可序列化的版本化工具标识。
type Reference struct {
	Name    string `json:"name"`
	Version string `json:"version,omitempty"`
}

// Call 是模型输出并完成参数组装后的工具调用。
type Call struct {
	ID             string          `json:"id"`
	Name           string          `json:"name"`
	Arguments      json.RawMessage `json:"arguments,omitempty"`
	Attempt        int             `json:"attempt,omitempty"`
	IdempotencyKey string          `json:"idempotency_key,omitempty"`
}

// Input 是传递给各种工具兼容层的统一执行输入。
type Input struct {
	SessionID     string // 所属会话
	RunID         string // 所属 Run
	TurnID        string // 所属 Turn
	WorkspaceRoot string // 会话创建时冻结的工作目录；空则回落 Ports
	Call          Call
}

// Result 是各种工具兼容层返回的统一结构化输出。
type Result struct {
	CallID  string
	Name    string
	Output  json.RawMessage
	Success bool
	Error   string
}

// Tool 是所有业务工具和兼容层必须实现的统一抽象。
// Execute 的业务失败必须返回 Result{Success:false} 且 error 为 nil；error 只表示取消或超时。
type Tool interface {
	Definition() Definition
	Execute(ctx context.Context, input Input) (Result, error)
}

// Inspector 是工具层额外的参数校验；失败即第 1 层 deny，不能把 ask 抬成 allow。
type Inspector interface {
	Inspect(ctx context.Context, input Input) error
}

// EffectResolver 按本次入参覆盖工具默认 Effect。只能返回 allow 或 ask；目录外调用仍由流水线强制 ask。
type EffectResolver interface {
	ResolveEffect(ctx context.Context, input Input) Effect
}

// Registry 定义工具注册、获取与提示词汇总的能力。
type Registry interface {
	Register(tool Tool) error
	Get(ref Reference) (Tool, error)
	GetAll(refs []Reference) ([]Tool, error)
	Prompts() []Prompt
}

// DispatchHook 由运行时注入，用于发出工具过程事件。
type DispatchHook func(kind string, call Call, attempt int, result *Result)

// Gate 是进程级占槽：Acquire 领取，Release 归还。nil 表示不限制。
type Gate interface {
	Acquire(ctx context.Context) error
	Release()
}

// Invocation 包含处理一组工具调用所需的全部信息。
type Invocation struct {
	SessionID       string
	RunID           string
	TurnID          string
	WorkspaceRoot   string // 会话冻结的工作目录，传给 Inspect / Execute
	Calls           []Call
	Mode            ExecutionMode
	FailurePolicy   FailurePolicy
	MaxParallel     int
	BoundNames      []string
	Effects         map[string]Effect
	Approval        ApprovalMode
	Registry        Registry
	ApprovedCallIDs []string
	DeniedCallIDs   []string
	OnEvent         DispatchHook
	Gate            Gate
}

// DispatchResult 按模型调用顺序保存结果，并标识是否因审批暂停。
type DispatchResult struct {
	Results         []Result
	WaitingApproval bool
	ApprovalIDs     []string
	PendingCalls    []Call
	ApprovalCalls   []Call
}
