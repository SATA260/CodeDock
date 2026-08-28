package memory

import (
	"context"
	"encoding/json"

	"codedock/pkg/agent/tool"
)

type readTool struct{}

type writeTool struct{}

type searchTool struct{}

// ReadTool 按 scope 与 name 读取目录或专题。本阶段为空实现，不注册到默认 Registry。
func ReadTool() tool.Tool {
	return readTool{}
}

// WriteTool 覆盖写入一篇目录或专题。本阶段为空实现，不注册到默认 Registry。
func WriteTool() tool.Tool {
	return writeTool{}
}

// SearchTool 按关键词检索同一工作区的 context message。本阶段为空实现，不注册到默认 Registry。
func SearchTool() tool.Tool {
	return searchTool{}
}

func (readTool) Definition() tool.Definition {
	return tool.Definition{
		Name:             "memory_read",
		Prompt:           "Read a memory index or topic by scope and name.",
		ParametersSchema: json.RawMessage(`{"type":"object","properties":{"scope":{"type":"string"},"name":{"type":"string"}},"required":["scope","name"]}`),
		Permission: tool.Permission{
			Capabilities: []string{"memory"},
			Risk:         tool.RiskRead,
		},
		Version: "1",
	}
}

func (writeTool) Definition() tool.Definition {
	return tool.Definition{
		Name:             "memory_write",
		Prompt:           "Overwrite a memory index or topic by scope and name.",
		ParametersSchema: json.RawMessage(`{"type":"object","properties":{"scope":{"type":"string"},"name":{"type":"string"},"content":{"type":"string"}},"required":["scope","name","content"]}`),
		Permission: tool.Permission{
			Capabilities: []string{"memory"},
			Risk:         tool.RiskWrite,
		},
		Version: "1",
	}
}

func (searchTool) Definition() tool.Definition {
	return tool.Definition{
		Name:             "memory_search",
		Prompt:           "Search indexed messages in the current workspace by keyword.",
		ParametersSchema: json.RawMessage(`{"type":"object","properties":{"query":{"type":"string"}},"required":["query"]}`),
		Permission: tool.Permission{
			Capabilities: []string{"memory"},
			Risk:         tool.RiskRead,
		},
		Version: "1",
	}
}

// Execute 本阶段不读库。
// TODO: 解析 scope 与 name，调用 Get。
func (t readTool) Execute(ctx context.Context, input tool.Input) (tool.Result, error) {
	if err := ctx.Err(); err != nil {
		return tool.Result{CallID: input.Call.ID, Name: t.Definition().Name, Success: false, Error: err.Error()}, err
	}
	return tool.Result{CallID: input.Call.ID, Name: t.Definition().Name, Success: true, Output: json.RawMessage(`{}`)}, nil
}

// Execute 本阶段不写库。
// TODO: 解析参数并调用 Upsert。
func (t writeTool) Execute(ctx context.Context, input tool.Input) (tool.Result, error) {
	if err := ctx.Err(); err != nil {
		return tool.Result{CallID: input.Call.ID, Name: t.Definition().Name, Success: false, Error: err.Error()}, err
	}
	return tool.Result{CallID: input.Call.ID, Name: t.Definition().Name, Success: true, Output: json.RawMessage(`{}`)}, nil
}

// Execute 本阶段不检索。
// TODO: 按 Session 的 workspace_id 调用 SearchMessages。
func (t searchTool) Execute(ctx context.Context, input tool.Input) (tool.Result, error) {
	if err := ctx.Err(); err != nil {
		return tool.Result{CallID: input.Call.ID, Name: t.Definition().Name, Success: false, Error: err.Error()}, err
	}
	return tool.Result{CallID: input.Call.ID, Name: t.Definition().Name, Success: true, Output: json.RawMessage(`{}`)}, nil
}
