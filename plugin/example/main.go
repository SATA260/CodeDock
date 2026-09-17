// example 是给作者拷贝的模板。实现 sdk.Plugin，再按需实现各口的 Handler，main 里 sdk.Serve 即可。
// 只 import codedock/pkg/plugin。复制本目录，改 go.mod 模块名、Manifest.Name 和下面的策略。
//
// 本模板不订阅、不改正文、不换向、不登记方法。编进 PLUGIN_DIR 也不会改对话。
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
// 跨口、跨插件传参数用 in.Context.Set("example.xxx", v)，不要塞 Hidden。
package main

import (
	"context"

	sdk "codedock/pkg/plugin"
)

// plugin 是空模板：各口原样 Reply。
type plugin struct{}

// Bootstrap 不订阅、不登记方法。
func (plugin) Bootstrap(context.Context, sdk.Host) (sdk.Manifest, error) {
	return sdk.Manifest{Name: "example"}, nil
}

// OnAgentInput 原样通过。复制后可改 Content，或 Handle() 不建 Run。
func (plugin) OnAgentInput(_ context.Context, in sdk.AgentInput) (sdk.AgentInputResult, error) {
	return in.Reply(), nil
}

// OnAgentPreStep 原样通过。复制后可改 Hidden，或 Block() 取消本轮。
func (plugin) OnAgentPreStep(_ context.Context, in sdk.AgentPreStep) (sdk.AgentPreStepResult, error) {
	return in.Reply(), nil
}

// OnAgentRequest 原样通过。只能 Reply。
func (plugin) OnAgentRequest(_ context.Context, in sdk.AgentRequest) (sdk.AgentRequestResult, error) {
	return in.Reply(), nil
}

// OnLLMStream 原样通过。只能 Reply；fake 模型不插这个口。
func (plugin) OnLLMStream(_ context.Context, in sdk.LLMStream) (sdk.LLMStreamResult, error) {
	return in.Reply(), nil
}

// OnToolPreExecute 原样通过。复制后可 Deny() 或 AskApproval()。
func (plugin) OnToolPreExecute(_ context.Context, in sdk.ToolPreExecute) (sdk.ToolPreExecuteResult, error) {
	return in.Reply(), nil
}

// OnToolPostExecute 原样通过。只能 Reply。
func (plugin) OnToolPostExecute(_ context.Context, in sdk.ToolPostExecute) (sdk.ToolPostExecuteResult, error) {
	return in.Reply(), nil
}

// OnLedgerNotify 忽略账本通知。
func (plugin) OnLedgerNotify(context.Context, sdk.LedgerNotify) error {
	return nil
}

// ExecuteMethod 本模板不登记方法。
func (plugin) ExecuteMethod(context.Context, sdk.MethodInput) (sdk.MethodResult, error) {
	return sdk.MethodResult{Success: false, Error: "no methods"}, nil
}

// main 启动 example 插件进程。
func main() {
	sdk.Serve(plugin{})
}
