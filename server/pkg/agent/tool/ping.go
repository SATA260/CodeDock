package tool

import (
	"context"
	"encoding/json"
)

type pingTool struct{}

// Ping 返回始终成功的示例工具。
func Ping() Tool {
	return pingTool{}
}

// Definition 返回 ping 的只读健康检查定义。
func (pingTool) Definition() Definition {
	return Definition{
		Name:             "ping",
		Prompt:           "Health-check tool that returns ok. No arguments are required.",
		ParametersSchema: json.RawMessage(`{"type":"object","properties":{}}`),
		OutputSchema:     json.RawMessage(`{"type":"object","properties":{"ok":{"type":"boolean"}}}`),
		Permission: Permission{
			Capabilities: []string{"ping"},
			Risk:         RiskRead,
		},
		SupportsCancel: true,
		SupportsRetry:  true,
		Version:        "1",
	}
}

// Execute 立即返回 {"ok":true}，不访问外部系统。
func (p pingTool) Execute(ctx context.Context, input Input) (Result, error) {
	if err := ctx.Err(); err != nil {
		return Result{CallID: input.Call.ID, Name: p.Definition().Name, Success: false, Error: err.Error()}, err
	}
	return Result{
		CallID:  input.Call.ID,
		Name:    "ping",
		Output:  json.RawMessage(`{"ok":true}`),
		Success: true,
	}, nil
}
