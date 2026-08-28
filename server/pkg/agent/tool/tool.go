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

// RiskLevel 描述工具能力声明的风险级别。
type RiskLevel string

const (
	RiskRead  RiskLevel = "read"
	RiskWrite RiskLevel = "write"
	RiskAdmin RiskLevel = "admin"
)

// Permission 描述工具所需的能力与资源范围。
type Permission struct {
	Capabilities []string
	Risk         RiskLevel
	Resource     string
}

// PermissionPolicy 是 Run 创建时冻结的工具授权策略。
type PermissionPolicy struct {
	Version             string
	AllowedCapabilities []string
	DeniedTools         []string
	ResourceScopes      []string
}

// ApprovalPolicy 是 Run 创建时冻结的用户审批策略。
type ApprovalPolicy struct {
	Version             string
	AutoApprovedTools   []string
	ApprovalRequiredFor []string
	DefaultExpiry       time.Duration
}

// Definition 是模型可见的统一工具描述。
type Definition struct {
	Name             string
	Prompt           string
	ParametersSchema json.RawMessage
	OutputSchema     json.RawMessage
	Permission       Permission
	SupportsCancel   bool
	SupportsRetry    bool
	Version          string
}

// Prompt 是注册中心返回给模型层的统一工具提示词。
type Prompt struct {
	Name             string
	Content          string
	ParametersSchema json.RawMessage
	OutputSchema     json.RawMessage
}

// Reference 是可序列化的版本化工具标识。
type Reference struct {
	Name    string
	Version string
}

// Call 是模型输出并完成参数组装后的工具调用。
type Call struct {
	ID             string
	Name           string
	Arguments      json.RawMessage
	Attempt        int
	IdempotencyKey string
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
}

// DispatchResult 按模型调用顺序保存结果，并标识是否因审批暂停。
type DispatchResult struct {
	Results         []Result
	WaitingApproval bool
	ApprovalIDs     []string
}

// Dispatch 按已解析的工具调用执行调度。
// TODO: 按权限与审批策略调度工具调用。
func Dispatch(_ context.Context, _ Invocation) (DispatchResult, error) {
	return DispatchResult{}, nil
}
