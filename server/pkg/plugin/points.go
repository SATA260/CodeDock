package plugin

import (
	"encoding/json"

	pkgagent "codedock/pkg/agent"
)

// AgentInput 是 agent/input 的入参。此时 Run 还没建。
type AgentInput struct {
	SessionID string            // 当前会话；Host 回调要用
	RunID     string            // 这个口上为空
	TurnID    string            // 这个口上为空
	Content   string            // 用户正文，可改
	Mode      pkgagent.WorkMode // 本轮内置 Agent：ask / plan / agent，可改
	Context   PluginContext     // 插件共享袋子；Reply 会带回
}

// AgentInputResult 是 agent/input 的回包。
type AgentInputResult struct {
	Content string            // 写进用户消息的正文
	Mode    pkgagent.WorkMode // 本轮模式；空表示不改
	Handled bool              // true：不建 Run。用 Handle() 设置
	Context PluginContext     // 盖写后的共享袋子
}

// Reply 继续本口，带回改过的正文、模式和袋子。
func (in AgentInput) Reply() AgentInputResult {
	return AgentInputResult{Content: in.Content, Mode: in.Mode, Context: in.Context.Clone()}
}

// Handle 换到 input/handled，不建 Run。
func (in AgentInput) Handle() AgentInputResult {
	out := in.Reply()
	out.Handled = true
	return out
}

// AgentPreStep 是 agent/pre-step 的入参。
type AgentPreStep struct {
	SessionID    string        // 当前会话
	RunID        string        // 本轮 Run
	TurnID       string        // 当前 Turn，可能为空
	SystemPrompt string        // 本轮系统提示，可改
	Hidden       []Message     // 不入库、只进模型上下文的 system 消息，可追加
	Context      PluginContext // 插件共享袋子；Reply 会带回
}

// AgentPreStepResult 是 agent/pre-step 的回包。
type AgentPreStepResult struct {
	SystemPrompt string        // 盖写后的系统提示
	Hidden       []Message     // 盖写后的隐藏消息
	Blocked      bool          // true：取消本轮。用 Block() 设置
	Context      PluginContext // 盖写后的共享袋子
}

// Reply 继续本口，带回改过的系统提示、隐藏消息和袋子。
func (in AgentPreStep) Reply() AgentPreStepResult {
	return AgentPreStepResult{SystemPrompt: in.SystemPrompt, Hidden: in.Hidden, Context: in.Context.Clone()}
}

// Block 换到 run/blocked，取消本轮。
func (in AgentPreStep) Block() AgentPreStepResult {
	out := in.Reply()
	out.Blocked = true
	return out
}

// AgentRequest 是 agent/request 的入参。只能改数据。
type AgentRequest struct {
	SessionID    string        // 当前会话
	RunID        string        // 本轮 Run
	TurnID       string        // 当前 Turn
	SystemPrompt string        // 即将发给模型的系统提示，可改
	Messages     []Message     // 即将发给模型的消息，可改
	Context      PluginContext // 插件共享袋子；Reply 会带回
}

// AgentRequestResult 是 agent/request 的回包。
type AgentRequestResult struct {
	SystemPrompt string        // 盖写后的系统提示
	Messages     []Message     // 盖写后的消息
	Context      PluginContext // 盖写后的共享袋子
}

// Reply 继续本口，带回改过的提示、消息和袋子。
func (in AgentRequest) Reply() AgentRequestResult {
	return AgentRequestResult{SystemPrompt: in.SystemPrompt, Messages: in.Messages, Context: in.Context.Clone()}
}

// LLMStream 是 llm/stream 的入参。只能改数据；fake 模型不经过这里。
type LLMStream struct {
	SessionID string            // 当前会话
	RunID     string            // 本轮 Run
	TurnID    string            // 当前 Turn
	Headers   map[string]string // 即将发出的 HTTP 头，可改
	Body      json.RawMessage   // 即将发出的 HTTP 体，可改
	Context   PluginContext     // 插件共享袋子；Reply 会带回
}

// LLMStreamResult 是 llm/stream 的回包。
type LLMStreamResult struct {
	Headers map[string]string // 盖写后的请求头；nil 表示不改头
	Body    json.RawMessage   // 盖写后的请求体；空表示不改体
	Context PluginContext     // 盖写后的共享袋子
}

// Reply 继续本口，带回改过的请求头、请求体和袋子。
func (in LLMStream) Reply() LLMStreamResult {
	return LLMStreamResult{Headers: in.Headers, Body: in.Body, Context: in.Context.Clone()}
}

// ToolPreExecute 是 tools/pre-execute 的入参。已批准的调用不会再进。
type ToolPreExecute struct {
	SessionID string        // 当前会话
	RunID     string        // 本轮 Run
	TurnID    string        // 当前 Turn
	Call      Call          // 马上要跑的工具调用，可改 Arguments
	Context   PluginContext // 插件共享袋子；Reply 会带回
}

// ToolPreExecuteResult 是 tools/pre-execute 的回包。Denied 与 Ask 同时为 true 时按 Denied。
type ToolPreExecuteResult struct {
	Call    Call          // 盖写后的调用（改参后仍走这个 Call）
	Denied  bool          // true：当失败。用 Deny() 设置
	Ask     bool          // true：进审批。用 AskApproval() 设置
	Context PluginContext // 盖写后的共享袋子
}

// Reply 继续本口，带回改过的工具调用和袋子。
func (in ToolPreExecute) Reply() ToolPreExecuteResult {
	return ToolPreExecuteResult{Call: in.Call, Context: in.Context.Clone()}
}

// Deny 换到 tools/denied，这次调用当失败。
func (in ToolPreExecute) Deny() ToolPreExecuteResult {
	out := in.Reply()
	out.Denied = true
	return out
}

// AskApproval 换到 tools/ask，进审批。
func (in ToolPreExecute) AskApproval() ToolPreExecuteResult {
	out := in.Reply()
	out.Ask = true
	return out
}

// ToolPostExecute 是 tools/post-execute 的入参。只能改数据。
type ToolPostExecute struct {
	SessionID string        // 当前会话
	RunID     string        // 本轮 Run
	TurnID    string        // 当前 Turn
	Call      Call          // 刚跑完的调用
	Result    Result        // 工具结果，可改 Output / Success / Error
	Context   PluginContext // 插件共享袋子；Reply 会带回
}

// ToolPostExecuteResult 是 tools/post-execute 的回包。
type ToolPostExecuteResult struct {
	Call    Call          // 原样带回即可
	Result  Result        // 盖写后的结果
	Context PluginContext // 盖写后的共享袋子
}

// Reply 继续本口，带回改过的工具结果和袋子。
func (in ToolPostExecute) Reply() ToolPostExecuteResult {
	return ToolPostExecuteResult{Call: in.Call, Result: in.Result, Context: in.Context.Clone()}
}

// LedgerNotify 是账本通知，不是六个口。改回包没有换向效果。
type LedgerNotify struct {
	Type      string          // 事件名，例如 run.completed
	SessionID string          // 当前会话
	RunID     string          // 本轮 Run
	TurnID    string          // 当前 Turn，可能为空
	Payload   json.RawMessage // 事件载荷，只读
	Context   PluginContext   // 当时的共享袋子，只读
}
