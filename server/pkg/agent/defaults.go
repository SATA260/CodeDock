package agent

import (
	"encoding/json"
	"time"

	"codedock/pkg/agent/profile"
	"codedock/pkg/agent/tool"
)

const (
	DefaultSystemPrompt = `你是 CodeDock 里的编程助手。`
	DefaultToolSet      = "agent-8"
)

// FakeOptions 控制 fake 模型的确定性输出，供测试与离线闭环使用。
type FakeOptions struct {
	Turns               []FakeTurn         `json:"turns"`
	FailTimes           int                `json:"fail_times"`
	Hang                bool               `json:"hang"`
	CompactSummary      string             `json:"compact_summary"`
	IndexCompactSummary string             `json:"index_compact_summary"`
	Review              *FakeReview        `json:"review,omitempty"`
	Verify              []FakeVerifyResult `json:"verify,omitempty"`
}

// FakeReview 控制 fake 复审模型的输出。
type FakeReview struct {
	Decisions []ApprovalDecision `json:"decisions,omitempty"`
	Escalate  bool               `json:"escalate,omitempty"`
	Fail      bool               `json:"fail,omitempty"`
}

// FakeVerifyResult 控制 fake 验证脚本的一轮输出。
type FakeVerifyResult struct {
	Status        VerifyStatus `json:"status"`
	Output        string       `json:"output,omitempty"`
	Fingerprint   string       `json:"fingerprint,omitempty"`
	FailedCommand string       `json:"failed_command,omitempty"`
}

// FakeTurn 是 fake 模型一轮输出。
type FakeTurn struct {
	Text      string         `json:"text"`
	ToolCalls []FakeToolCall `json:"tool_calls"`
}

// FakeToolCall 是 fake 模型发出的工具调用。
type FakeToolCall struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

// DefaultRetryConfig 返回可测的默认重试配置。
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxAttempts:    3,
		InitialBackoff: 5 * time.Millisecond,
		MaxBackoff:     50 * time.Millisecond,
		Multiplier:     2,
		Jitter:         0,
	}
}

// DefaultRunConfig 冻结一份可运行的默认 Run 配置：指定工作模式，审批默认 manual。
func DefaultRunConfig(mode WorkMode, model ModelConfig) RunConfigSnapshot {
	if mode == "" {
		mode = WorkAgent
	}
	if model.Provider == "" {
		model.Provider = "fake"
	}
	if model.Model == "" {
		model.Model = "fake"
	}
	prof := profile.For(string(mode))
	retry := DefaultRetryConfig()
	return RunConfigSnapshot{
		Mode:             mode,
		Approval:         ApprovalManual,
		SystemPromptHash: prof.Prompt.Version,
		Model:            model,
		ToolSetVersion:   prof.Tools.Version,
		RetryPolicy: RetryPolicy{
			Context: retry,
			Model:   retry,
			Tool:    retry,
		},
		Limits: RunLimits{
			MaxWallTime:       20 * time.Minute,
			MaxTurns:          32,
			MaxToolCalls:      64,
			MaxInputTokens:    256000,
			MaxOutputTokens:   65536,
			MaxParallelTools:  4,
			MaxVerifyRounds:   3,
			MaxEvaluateRounds: 2,
		},
		ToolExecutionMode: tool.ExecutionSerial,
		ToolFailurePolicy: tool.FailureBestEffort,
		Profile:           prof,
	}
}

// DefaultYoloConfig 给测试用：agent + yolo，写类工具不暂停。
func DefaultYoloConfig(model ModelConfig) RunConfigSnapshot {
	cfg := DefaultRunConfig(WorkAgent, model)
	cfg.Approval = ApprovalYolo
	return cfg
}

// ParseFakeOptions 从 ModelConfig.Options 解析 fake 脚本。
func ParseFakeOptions(raw json.RawMessage) FakeOptions {
	var opts FakeOptions
	if len(raw) == 0 {
		return FakeOptions{Turns: []FakeTurn{{Text: "ok"}}}
	}
	if err := json.Unmarshal(raw, &opts); err != nil {
		return FakeOptions{Turns: []FakeTurn{{Text: "ok"}}}
	}
	if len(opts.Turns) == 0 {
		opts.Turns = []FakeTurn{{Text: "ok"}}
	}
	return opts
}
