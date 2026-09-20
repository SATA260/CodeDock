package tools

import (
	"context"
	"encoding/json"
	"fmt"

	pkgagent "codedock/pkg/agent"
	"codedock/pkg/agent/tool"
)

// ExploreFunc 由 Runtime 注入的只读探索实现。
type ExploreFunc func(ctx context.Context, input pkgagent.ExploreInput, workspaceRoot, sessionID, runID string) (pkgagent.ExploreOutput, error)

type exploreTool struct {
	ports Ports
}

// Definition 描述只读探索工具。
func (t exploreTool) Definition() tool.Definition {
	return tool.Definition{
		Name:             "explore",
		Prompt:           "用便宜模型只读翻代码，返回带文件:行号引用的短结论。已知道精确位置时直接用 read。",
		ParametersSchema: schemaOf[pkgagent.ExploreInput](),
		Permission:       tool.Permission{Effect: tool.EffectAllow},
		SupportsRetry:    true,
		Version:          "1",
	}
}

// Inspect 探索不校验路径；子循环内部拒绝工作区外读取。
func (t exploreTool) Inspect(_ context.Context, _ tool.Input) error {
	return nil
}

// Execute 调度只读探索子代理。
func (t exploreTool) Execute(ctx context.Context, input tool.Input) (tool.Result, error) {
	if err := ctx.Err(); err != nil {
		return tool.Result{CallID: input.Call.ID, Name: "explore", Success: false, Error: err.Error()}, err
	}
	var args pkgagent.ExploreInput
	if err := json.Unmarshal(nonzeroJSON(input.Call.Arguments), &args); err != nil {
		return tool.Result{CallID: input.Call.ID, Name: "explore", Success: false, Error: err.Error()}, nil
	}
	if t.ports.Explore == nil {
		return tool.Result{CallID: input.Call.ID, Name: "explore", Success: false, Error: "explore 失败：未配置子代理，请直接用 read / grep"}, nil
	}
	out, err := t.ports.Explore(ctx, args, workspaceOf(input, t.ports), input.SessionID, input.RunID)
	if err != nil {
		return tool.Result{CallID: input.Call.ID, Name: "explore", Success: false, Error: fmt.Sprintf("explore 失败：%s，请直接用 read / grep", err)}, nil
	}
	if out.Error != "" && out.Summary == "" {
		out.Summary = out.Error
	}
	return okToolResult(input.Call.ID, "explore", out)
}
