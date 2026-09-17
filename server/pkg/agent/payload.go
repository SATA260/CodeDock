package agent

import "encoding/json"

// InputPayload 是 agent/input 口能改的数据。
type InputPayload struct {
	Content string   `json:"content"`
	Mode    WorkMode `json:"mode,omitempty"` // ask / plan / agent；空表示不改
}

// PreStepPayload 是 agent/pre-step 口能改的数据。
type PreStepPayload struct {
	SystemPrompt string    `json:"system_prompt"`
	Hidden       []Message `json:"hidden,omitempty"`
}

// RequestPayload 是 agent/request 口能改的数据。
type RequestPayload struct {
	SystemPrompt string    `json:"system_prompt"`
	Messages     []Message `json:"messages,omitempty"`
}

// StreamPayload 是 llm/stream 口能改的数据。
type StreamPayload struct {
	Headers map[string]string `json:"headers,omitempty"`
	Body    json.RawMessage   `json:"body,omitempty"`
}
