package tool

import (
	"context"
	"encoding/json"
	"time"
)

// ExecutionMode 定义一组工具调用采用串行还是并行执行。
type ExecutionMode string

const (
	ExecutionSerial   ExecutionMode = "serial"
	ExecutionParallel ExecutionMode = "parallel"
)

// FailurePolicy 定义某个工具调用失败后整组调用的处理方式。
type FailurePolicy string

const (
	FailureFast       FailurePolicy = "fail_fast"
	FailureCollectAll FailurePolicy = "collect_all"
	FailureBestEffort FailurePolicy = "best_effort"
)

// Capability 是工具声明的动作能力；运行模式提供能力，须覆盖工具的全部能力才可调用。
type Capability string

const (
	CapabilityRead   Capability = "read"
	CapabilityWrite  Capability = "write"
	CapabilityMemory Capability = "memory"
)

// Permission 描述工具所需的能力；审批与否由工具自己声明。
type Permission struct {
	Capabilities     []Capability `json:"capabilities,omitempty"`
	RequiresApproval bool         `json:"requires_approval"`
	Resource         string       `json:"resource,omitempty"`
}

// PermissionPolicy 是 Run 创建时冻结的工具授权策略。
type PermissionPolicy struct {
	Version             string       `json:"version,omitempty"`
	AllowedCapabilities []Capability `json:"allowed_capabilities,omitempty"`
	DeniedTools         []string     `json:"denied_tools,omitempty"`
	ResourceScopes      []string     `json:"resource_scopes,omitempty"`
}

// ApprovalPolicy 是 Run 创建时冻结的用户审批策略。
type ApprovalPolicy struct {
	Version           string        `json:"version,omitempty"`
	AutoApprovedTools []string      `json:"auto_approved_tools,omitempty"`
	DefaultExpiry     time.Duration `json:"default_expiry,omitempty"`
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
	SessionID string
	RunID     string
	TurnID    string
	Call      Call
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
type Tool interface {
	Definition() Definition
	Execute(ctx context.Context, input Input) (Result, error)
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

// Invocation 包含处理一组工具调用所需的全部信息。
type Invocation struct {
	SessionID        string
	RunID            string
	TurnID           string
	Calls            []Call
	Mode             ExecutionMode
	FailurePolicy    FailurePolicy
	MaxParallel      int
	PermissionPolicy PermissionPolicy
	ApprovalPolicy   ApprovalPolicy
	AgentMode        string
	Registry         Registry
	ApprovedCallIDs  []string
	DeniedCallIDs    []string
	OnEvent          DispatchHook
}

// DispatchResult 按模型调用顺序保存结果，并标识是否因审批暂停。
type DispatchResult struct {
	Results         []Result
	WaitingApproval bool
	ApprovalIDs     []string
	PendingCalls    []Call
	ApprovalCalls   []Call
}
