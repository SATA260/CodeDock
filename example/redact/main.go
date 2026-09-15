// redact 在三个口把秘密换成占位符，不拦工具、不建方法。
// 只 import codedock/pkg/plugin。hello 仍是作者拷贝模板。
//
//	mkdir -p "$PLUGIN_DIR/redact"
//	go build -o "$PLUGIN_DIR/redact/redact" .
//	PLUGIN_DIR=... 启动 API
package main

import (
	"context"

	sdk "codedock/pkg/plugin"
)

// plugin 订阅 input / request / post-execute，只改数据。
type plugin struct{}

// Bootstrap 声明听用户正文、发给模型前、工具回包后。
func (p *plugin) Bootstrap(_ context.Context, _ sdk.Host) (sdk.Manifest, error) {
	return sdk.Manifest{
		Name: "redact",
		Subscriptions: []string{
			sdk.TypeInput,
			sdk.TypeRequest,
			sdk.TypePostExecute,
		},
	}, nil
}

// OnAgentInput 刮用户正文后再落库。
func (p *plugin) OnAgentInput(_ context.Context, in sdk.AgentInput) (sdk.AgentInputResult, error) {
	in.Content, _ = maskText(in.Content)
	return in.Reply(), nil
}

// OnAgentRequest 刮即将发给模型的提示、消息和工具参数。
func (p *plugin) OnAgentRequest(_ context.Context, in sdk.AgentRequest) (sdk.AgentRequestResult, error) {
	in.SystemPrompt, _ = maskText(in.SystemPrompt)
	for i := range in.Messages {
		in.Messages[i].Content, _ = maskJSON(in.Messages[i].Content)
		for j := range in.Messages[i].ToolCalls {
			in.Messages[i].ToolCalls[j].Arguments, _ = maskJSON(in.Messages[i].ToolCalls[j].Arguments)
		}
	}
	return in.Reply(), nil
}

// OnToolPostExecute 刮回包和失败信息，不改 Success。
func (p *plugin) OnToolPostExecute(_ context.Context, in sdk.ToolPostExecute) (sdk.ToolPostExecuteResult, error) {
	in.Result.Output = rewriteOutput(in.Result.Output)
	in.Result.Error, _ = maskText(in.Result.Error)
	return in.Reply(), nil
}

// ExecuteMethod 本插件不登记方法。
func (p *plugin) ExecuteMethod(_ context.Context, _ sdk.MethodInput) (sdk.MethodResult, error) {
	return sdk.MethodResult{Success: false, Error: "redact has no methods"}, nil
}

// main 启动 redact 插件进程。
func main() {
	sdk.Serve(&plugin{})
}
