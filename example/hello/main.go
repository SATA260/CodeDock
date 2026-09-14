// hello 是给作者拷贝的唯一示例。实现 sdk.Plugin，再按需实现各口的 Handler，main 里 sdk.Serve 即可。
// 只 import codedock/pkg/plugin。复制本目录，改 go.mod 模块名、Manifest.Name 和下面的策略。
//
//	mkdir -p "$PLUGIN_DIR/hello"
//	go build -o "$PLUGIN_DIR/hello/hello" .
//	PLUGIN_DIR=... 启动 API
//
// 六个口的参数和回包就是 SDK 里的结构体，跳进类型看字段：
//
//	OnAgentInput       sdk.AgentInput / sdk.AgentInputResult
//	OnAgentPreStep     sdk.AgentPreStep / sdk.AgentPreStepResult
//	OnAgentRequest     sdk.AgentRequest / sdk.AgentRequestResult
//	OnLLMStream        sdk.LLMStream / sdk.LLMStreamResult
//	OnToolPreExecute   sdk.ToolPreExecute / sdk.ToolPreExecuteResult
//	OnToolPostExecute  sdk.ToolPostExecute / sdk.ToolPostExecuteResult
//	OnLedgerNotify     sdk.LedgerNotify
//
// 没实现的口原样通过。Reply 继续，Handle / Block / Deny / AskApproval 换向。
// 跨口、跨插件传参数用 in.Context.Set("hello.xxx", v)，不要塞 Hidden。
//
// 本示例：普通正文加 [hello] 前缀；/skip 不建 Run；开跑前塞一条隐藏提示；参数含 forbidden 则否决；登记 hello 方法。
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

// main 启动 hello 插件进程。
func main() {
	sdk.Serve(&hello{})
}
