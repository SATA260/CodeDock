package agent

import (
	"context"
	"encoding/json"
	"time"

	"codedock/pkg/agent/tool"
)

// Phase 表示唤醒一次 Step 的原因。
type Phase string

const (
	PhaseInit              Phase = "init"               // Run 启动后首次执行
	PhaseUserInput         Phase = "user_input"         // 收到新的用户输入
	PhaseLLMResult         Phase = "llm_result"         // 模型流输出结束
	PhaseToolsBatchResult  Phase = "tools_batch_result" // 一批工具执行完毕
	PhaseHumanApproved     Phase = "human_approved"     // 用户审批通过
	PhaseHumanAbort        Phase = "human_abort"        // 用户拒绝继续
	PhaseCompressionResult Phase = "compression_result" // 上下文压缩完成
	PhaseError             Phase = "error"              // 执行出错后重入
)

// InstructionType 是 Brain 能发出的指令类型。
type InstructionType string

const (
	InstructionLoadContext         InstructionType = "load_context"          // 加载上下文
	InstructionCallLLM             InstructionType = "call_llm"              // 调用模型
	InstructionCallToolsBatch      InstructionType = "call_tools_batch"      // 调用一批工具
	InstructionRequestHumanApprove InstructionType = "request_human_approve" // 请求人工审批
	InstructionCompressContext     InstructionType = "compress_context"      // 压缩上下文
	InstructionFinish              InstructionType = "finish"                // 结束 Run
)

// AgentState 是单次 Agent 执行（即一个 Run）在某一时刻可序列化的完整状态，供单步执行只读使用。
type AgentState struct {
	SessionID       string            // 所属会话
	RunID           string            // 本次 Run
	TurnID          *string           // 当前 Turn（如有）
	Status          RunStatus         // 当前粗状态
	StepIndex       int               // 已提交的步骤序号；下一步必须递增
	Config          RunConfigSnapshot // 启动配置快照，只读
	CancelRequested bool              // 用户是否请求取消
	StopReason      *StopReason       // 结束原因（终态时）
	ForceFinish     bool              // 是否强制收尾
	Checkpoint      ToolCheckpoint    // 工具执行恢复点
	PendingApproval *string           // 未完成的审批 ID
	StartedAt       *time.Time
	FinishedAt      *time.Time
}

// ToolCheckpoint 记录同一批 tool_call 中哪些已完成、已批准、已拒绝、待执行，以及已产生的结果。
type ToolCheckpoint struct {
	TurnID    string
	Completed []string      // 已执行完毕的 tool_call_id
	Approved  []string      // 已批准 tool_call_id
	Denied    []string      // 已拒绝 tool_call_id
	Pending   []tool.Call   // 待执行 tool_call
	Results   []tool.Result // 已产生结果，按原始顺序
}

// StepJob 是 Worker 执行总线上的任务。按 run_id + step_index 去重，确保同一幂等键只执行一次。
type StepJob struct {
	RunID     string          // 所属 Run
	StepIndex int             // 幂等去重键
	Phase     Phase           // 唤醒原因
	Payload   json.RawMessage // 上一步留下的载荷
	Attempt   int             // 本步骤重试次数
}

// CallToolsBatchPayload 是 call_tools_batch 指令的完整载荷。
type CallToolsBatchPayload struct {
	Calls       []tool.Call // 本次全部 tool_call
	Mode        string      // 执行模式：serial 或 parallel
	MaxParallel int         // 并行上限
}

// ToolsBatchResultPayload 是 tools_batch_result 事实的载荷。
type ToolsBatchResultPayload struct {
	Results []tool.Result // 含成功 / 失败 / 拒绝 / 跳过，按原始顺序
}

// Instruction 是 Brain 对 Engine 的一条指令。
type Instruction struct {
	Type    InstructionType
	Payload json.RawMessage
}

// Fact 是观察面需要持久化的一条事实。
type Fact struct {
	Type    EventType
	TurnID  *string
	Payload json.RawMessage
}

// FactWriter 由 Runtime 实现：步骤内写入一条事实（不推进 step_index）。
type FactWriter interface {
	Append(ctx context.Context, runID string, fact Fact) error
}

// StepInput 是 Engine.Step 的入参：快照、作业和 Coordinator 装好的上下文。
type StepInput struct {
	State   AgentState
	Job     StepJob
	History History
}

// FinishPayload 是 finish 指令的载荷。
type FinishPayload struct {
	Status RunStatus  `json:"status"`
	Reason StopReason `json:"reason"`
}

// StepResult 是一步执行后的输出。
type StepResult struct {
	State    AgentState // 更新后的 AgentState（仅内存只读，持久化由 Coordinator 负责）
	Facts    []Fact     // 本步骤产生的事件
	Messages []Message  // 本步骤要落库的助手 / 工具消息
	Next     *StepJob   // 非终态时指向下一步作业
}
