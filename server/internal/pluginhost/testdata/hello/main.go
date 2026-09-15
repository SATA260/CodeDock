// hello 只给宿主和回路测试用：改正文、跳过、否决、登记方法。
// 不是作者模板，不要编进仓根 plugin/。
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"strings"

	sdk "codedock/pkg/plugin"
)

// hello 演示四个口和方法登记。
type hello struct {
	host sdk.Host
}

// helloInput 是 hello 方法的入参。
type helloInput struct {
	Text string `json:"text"`
}

// helloOutput 是 hello 方法的出参。
type helloOutput struct {
	Text string `json:"text"`
}

// Bootstrap 登记 hello 方法，并订阅 input / pre-step / pre-execute / run.completed。
func (p *hello) Bootstrap(ctx context.Context, host sdk.Host) (sdk.Manifest, error) {
	p.host = host
	err := host.RegisterMethod(ctx, sdk.Method{
		Name:   "hello",
		Prompt: "连通性检查：把 text 原样返回。",
		ParametersSchema: json.RawMessage(`{
			"type": "object",
			"properties": {"text": {"type": "string", "description": "要回显的文本"}},
			"required": ["text"]
		}`),
	})
	return sdk.Manifest{
		Name: "hello",
		Subscriptions: []string{
			sdk.TypeInput,
			sdk.TypePreStep,
			sdk.TypePreExecute,
			"run.completed",
		},
	}, err
}

// OnAgentInput 给正文加 [hello] 前缀；以 /skip 开头则不建 Run。
func (p *hello) OnAgentInput(ctx context.Context, in sdk.AgentInput) (sdk.AgentInputResult, error) {
	content := strings.TrimSpace(in.Content)
	if strings.HasPrefix(content, "/skip") {
		if p.host != nil && in.SessionID != "" {
			_ = p.host.AppendNotice(ctx, in.SessionID, in.RunID, "hello 已跳过本次对话。")
		}
		return in.Handle(), nil
	}
	if content != "" && !strings.HasPrefix(content, "[hello] ") {
		in.Content = "[hello] " + content
	}
	in.Context.Set("hello.marked", true)
	return in.Reply(), nil
}

// OnAgentPreStep 注入一条告诉模型可以调用 hello 的隐藏提示。
func (p *hello) OnAgentPreStep(_ context.Context, in sdk.AgentPreStep) (sdk.AgentPreStepResult, error) {
	in.Hidden = append(in.Hidden, sdk.HiddenText("hello 插件已加载。需要回显时调用 hello。"))
	if string(in.Context.Get("hello.marked")) == "true" {
		in.Context.Set("hello.seen_at_pre_step", true)
	}
	return in.Reply(), nil
}

// OnToolPreExecute 在工具参数含 forbidden 时否决该次调用。
func (p *hello) OnToolPreExecute(_ context.Context, in sdk.ToolPreExecute) (sdk.ToolPreExecuteResult, error) {
	if bytes.Contains(in.Call.Arguments, []byte("forbidden")) {
		return in.Deny(), nil
	}
	return in.Reply(), nil
}

// OnLedgerNotify 在 run 结束时打一条日志。
func (p *hello) OnLedgerNotify(_ context.Context, in sdk.LedgerNotify) error {
	if in.Type == "run.completed" {
		slog.Info("hello: run completed", "session_id", in.SessionID, "run_id", in.RunID)
	}
	return nil
}

// ExecuteMethod 把 hello 方法的 text 原样返回。
func (p *hello) ExecuteMethod(_ context.Context, in sdk.MethodInput) (sdk.MethodResult, error) {
	var args helloInput
	if len(in.Arguments) > 0 {
		if err := json.Unmarshal(in.Arguments, &args); err != nil {
			return sdk.MethodResult{Success: false, Error: err.Error()}, nil
		}
	}
	out, err := json.Marshal(helloOutput{Text: args.Text})
	if err != nil {
		return sdk.MethodResult{Success: false, Error: err.Error()}, nil
	}
	return sdk.MethodResult{Success: true, Output: out}, nil
}

// main 启动 hello 测试插件进程。
func main() {
	sdk.Serve(&hello{})
}
