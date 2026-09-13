package agent

import (
	"encoding/json"
	"time"

	"codedock/pkg/agent/profile"
	"codedock/pkg/agent/tool"
)

const (
	DefaultSystemPrompt = `你是 CodeDock，一个冷静、靠谱的工作助手。像思路清楚的同事那样说话：直接、具体、留有余地，不卖萌，不端着。

写作要求：
- 用用户的语言回复。默认简洁，需要时再展开。
- 不要输出 emoji、颜文字、表情符号，也不要用它们当语气词。
- 不要用空泛开场或自我介绍，除非用户问你是谁。
- 不要堆砌感叹号，不要加油打气，不要用网络流行语。

需要做事时再调用当前提供的工具，不要为了调用而调用。一次回复里的工具会按批次审批，只提出当前必要的调用。`
	DefaultToolSet = "ping-memory-1"
)

// FakeOptions 控制 fake 模型的确定性输出，供测试与离线闭环使用。
type FakeOptions struct {
	Turns               []FakeTurn `json:"turns"`
	FailTimes           int        `json:"fail_times"`
	Hang                bool       `json:"hang"`
	CompactSummary      string     `json:"compact_summary"`
	IndexCompactSummary string     `json:"index_compact_summary"`
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

// DefaultRunConfig 冻结一份可运行的默认 Run 配置。
func DefaultRunConfig(mode AgentMode, model ModelConfig) RunConfigSnapshot {
	if mode == "" {
		mode = ModeAskForApproval
	}
	if model.Provider == "" {
		model.Provider = "fake"
	}
	if model.Model == "" {
		model.Model = "fake"
	}
	retry := DefaultRetryConfig()
	return RunConfigSnapshot{
		Mode:             mode,
		SystemPromptHash: "default-v3",
		Model:            model,
		ToolSetVersion:   DefaultToolSet,
		PermissionPolicy: tool.PermissionPolicy{
			Version: "1",
		},
		ApprovalPolicy: tool.ApprovalPolicy{
			Version:       "1",
			DefaultExpiry: time.Hour,
		},
		RetryPolicy: RetryPolicy{
			Context: retry,
			Model:   retry,
			Tool:    retry,
		},
		Limits: RunLimits{
			MaxWallTime:      5 * time.Minute,
			MaxTurns:         8,
			MaxToolCalls:     16,
			MaxInputTokens:   128000,
			MaxOutputTokens:  8192,
			MaxParallelTools: 4,
		},
		ToolExecutionMode: tool.ExecutionSerial,
		ToolFailurePolicy: tool.FailureBestEffort,
		Profile: profile.Config{
			ID:      "default",
			Version: "1",
			Mode:    string(mode),
			Prompt: profile.PromptConfig{
				Source:    "inline",
				Inline:    DefaultSystemPrompt,
				Version:   "3",
				Reference: "default",
			},
			Tools: profile.ToolConfig{
				Names:            []string{"ping", "memory_read", "memory_write", "memory_search"},
				Version:          DefaultToolSet,
				PermissionPolicy: tool.PermissionPolicy{Version: "1"},
				ApprovalPolicy:   tool.ApprovalPolicy{Version: "1", DefaultExpiry: time.Hour},
			},
		},
	}
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
