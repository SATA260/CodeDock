package tools

import (
	"context"
	"encoding/json"

	"codedock/pkg/agent/tool"
)

type pingTool struct{}

type pingInput struct{}

type pingOutput struct {
	OK bool `json:"ok"`
}

// Ping 返回始终成功的示例工具。
func Ping() tool.Tool {
	return pingTool{}
}

func (pingTool) Definition() tool.Definition {
	return tool.Definition{
		Name:             "ping",
		Prompt:           "Health-check tool that returns ok. No arguments are required.",
		ParametersSchema: schemaOf[pingInput](),
		OutputSchema:     schemaOf[pingOutput](),
		Permission: tool.Permission{
			RequiresApproval: true,
		},
		SupportsCancel: true,
		SupportsRetry:  true,
		Version:        "1",
	}
}

func (p pingTool) Execute(ctx context.Context, input tool.Input) (tool.Result, error) {
	if err := ctx.Err(); err != nil {
		return tool.Result{CallID: input.Call.ID, Name: p.Definition().Name, Success: false, Error: err.Error()}, err
	}
	var args pingInput
	if len(input.Call.Arguments) > 0 {
		if err := json.Unmarshal(input.Call.Arguments, &args); err != nil {
			return tool.Result{CallID: input.Call.ID, Name: "ping", Success: false, Error: err.Error()}, nil
		}
	}
	raw, err := json.Marshal(pingOutput{OK: true})
	if err != nil {
		return tool.Result{CallID: input.Call.ID, Name: "ping", Success: false, Error: err.Error()}, nil
	}
	return tool.Result{
		CallID:  input.Call.ID,
		Name:    "ping",
		Output:  raw,
		Success: true,
	}, nil
}
