package agent

import (
	"encoding/json"

	"codedock/pkg/agent/tool"
)

// RunCreatedPayload 是 run.created 的载荷。
type RunCreatedPayload struct {
	TriggerMessageID string            `json:"trigger_message_id"`
	Mode             WorkMode          `json:"mode"`
	Approval         ApprovalMode      `json:"approval,omitempty"`
	Status           RunStatus         `json:"status"`
	Text             string            `json:"text,omitempty"` // 用户触发正文，供前端展示排队消息
	Config           RunConfigSnapshot `json:"config"`
}

// RunStateChangedPayload 是 run.state_changed 的载荷。
type RunStateChangedPayload struct {
	From   RunStatus `json:"from"`
	To     RunStatus `json:"to"`
	Reason string    `json:"reason"`
}

// TurnStartedPayload 是 turn.started 的载荷。
type TurnStartedPayload struct {
	Number int `json:"number"`
}

// AssistantStartedPayload 是 assistant.started 的载荷。
type AssistantStartedPayload struct {
	MessageID string `json:"message_id"`
}

// AssistantDeltaPayload 是 assistant.delta 的载荷。
type AssistantDeltaPayload struct {
	MessageID string          `json:"message_id"`
	Delta     json.RawMessage `json:"delta"`
}

// AssistantCompletedPayload 是 assistant.completed 的载荷。
type AssistantCompletedPayload struct {
	MessageID string      `json:"message_id"`
	Text      string      `json:"text"`
	Reasoning string      `json:"reasoning,omitempty"`
	ToolCalls []tool.Call `json:"tool_calls,omitempty"`
}

// ToolCallPayload 是工具过程事件的公共载荷。
type ToolCallPayload struct {
	CallID     string          `json:"call_id"`
	Name       string          `json:"name"`
	Arguments  json.RawMessage `json:"arguments,omitempty"`
	Attempt    int             `json:"attempt,omitempty"`
	Success    *bool           `json:"success,omitempty"`
	Error      string          `json:"error,omitempty"`
	Output     json.RawMessage `json:"output,omitempty"`
	ApprovalID string          `json:"approval_id,omitempty"`
}

// ApprovalRequiredPayload 是 tool.approval_required 的载荷。
type ApprovalRequiredPayload struct {
	ApprovalID string             `json:"approval_id"`
	ToolCalls  []ApprovalToolCall `json:"tool_calls"`
	Kind       ApprovalKind       `json:"kind,omitempty"`
}

// ApprovalDecision 是一次提交里对单条 Tool 的裁决。
type ApprovalDecision struct {
	ToolCallID string         `json:"tool_call_id"`
	Status     ApprovalStatus `json:"status"`
	Reason     string         `json:"reason,omitempty"`
}

// ApprovalDecidedPayload 是 tool.approval_decided 的载荷。
type ApprovalDecidedPayload struct {
	ApprovalID string             `json:"approval_id"`
	ToolCallID string             `json:"tool_call_id,omitempty"`
	Status     ApprovalStatus     `json:"status"`
	Scope      ApprovalScope      `json:"scope"`
	Reason     string             `json:"reason,omitempty"`
	Decisions  []ApprovalDecision `json:"decisions,omitempty"`
	ToolCalls  []ApprovalToolCall `json:"tool_calls,omitempty"`
	Kind       ApprovalKind       `json:"kind,omitempty"`
	Override   OverrideAction     `json:"override,omitempty"`
}

// VerifyStartedPayload 是 verify.started 的载荷。
type VerifyStartedPayload struct {
	Round int `json:"round"`
}

// VerifyResultPayload 是 verify.result / verify.skipped 的载荷。
type VerifyResultPayload struct {
	VerifyResult
}

// EvaluateStartedPayload 是 evaluate.started 的载荷。
type EvaluateStartedPayload struct {
	Round int `json:"round"`
}

// EvaluationResult 旧 evaluate.result 载荷；新路径不再发出。
type EvaluationResult struct {
	Verdict string `json:"verdict"`
	Summary string `json:"summary,omitempty"`
	Skipped bool   `json:"skipped,omitempty"`
}

// EvaluateResultPayload 是 evaluate.result 的载荷。
type EvaluateResultPayload struct {
	EvaluationResult
}

// HumanOverridePayload 是 human_override 步骤的载荷。
type HumanOverridePayload struct {
	Action OverrideAction `json:"action"`
	Kind   ApprovalKind   `json:"kind,omitempty"`
}

// ApprovalRequestPayload 是 request_human_approve 指令的载荷。
type ApprovalRequestPayload struct {
	Kind   ApprovalKind `json:"kind"`
	Reason string       `json:"reason,omitempty"`
}

// UsageRecordedPayload 是 turn.usage_recorded 的载荷。
type UsageRecordedPayload struct {
	UsageID     string `json:"usage_id"`
	UsageType   string `json:"usage_type"`
	TotalTokens int64  `json:"total_tokens"`
	Estimated   bool   `json:"estimated"`
}

// ContextCompactedPayload 是 context.compacted 的载荷。
type ContextCompactedPayload struct {
	CheckpointID string `json:"checkpoint_id"`
	BaseEventSeq int64  `json:"base_event_seq"`
}

// TurnCompletedPayload 是 turn.completed 的载荷。
type TurnCompletedPayload struct {
	Number int        `json:"number"`
	Status TurnStatus `json:"status"`
}

// RunTerminalPayload 是 run.completed / failed / cancelled 的载荷。
type RunTerminalPayload struct {
	Status     RunStatus   `json:"status"`
	StopReason *StopReason `json:"stop_reason,omitempty"`
	Error      string      `json:"error,omitempty"` // 失败时给前端看的原因，不含堆栈
}

// MarshalPayload 将载荷编码为 JSON。
func MarshalPayload(v any) json.RawMessage {
	if v == nil {
		return json.RawMessage("{}")
	}
	body, err := json.Marshal(v)
	if err != nil {
		return json.RawMessage("{}")
	}
	return body
}
