package plugin

import (
	"context"
	"encoding/json"

	goplugin "github.com/hashicorp/go-plugin"

	pkgagent "codedock/pkg/agent"
	"codedock/pkg/agent/seam"
	"codedock/pkg/agent/tool"
)

// PluginName 是 go-plugin 握手时登记的插件键。
const PluginName = "codedock"

// Handshake 是宿主与插件进程的约定口令。
var Handshake = goplugin.HandshakeConfig{
	ProtocolVersion:  1,
	MagicCookieKey:   "CODEDOCK_PLUGIN",
	MagicCookieValue: "codedock-plugin-v1",
}

const (
	TypeInput        = seam.TypeInput
	TypePreStep      = seam.TypePreStep
	TypeRequest      = seam.TypeRequest
	TypeStream       = seam.TypeStream
	TypePreExecute   = seam.TypePreExecute
	TypePostExecute  = seam.TypePostExecute
	TypeInputHandled = seam.TypeInputHandled
	TypeRunBlocked   = seam.TypeRunBlocked
	TypeToolsDenied  = seam.TypeToolsDenied
	TypeToolsAsk     = seam.TypeToolsAsk
)

type (
	Envelope = seam.Envelope
	Message  = pkgagent.Message
	// Call 是一次工具调用：ID、Name、Arguments（模型填的 JSON）。
	Call = tool.Call
	// Result 是一次工具结果：CallID、Name、Output、Success、Error。
	Result = tool.Result
	// WorkMode 是本轮内置 Agent：ask / plan / agent。
	WorkMode = pkgagent.WorkMode
)

// Plugin 是作者要实现的接口。六个口用可选的 *Handler 接口，入参和回包都是结构体。
type Plugin interface {
	// Bootstrap 进程起来时调用一次，登记方法和订阅。
	Bootstrap(ctx context.Context, host Host) (Manifest, error)
	// ExecuteMethod 执行本插件登记给模型的方法。
	ExecuteMethod(ctx context.Context, in MethodInput) (MethodResult, error)
}

// Handler 是宿主侧进程：信封进出。作者实现 Plugin，Serve 会转成 Handler。
type Handler interface {
	// Bootstrap 拉起插件进程后立刻调用。
	Bootstrap(ctx context.Context, host Host) (Manifest, error)
	// OnEvent 把信封交给插件；作者侧已拆成各口结构体。
	OnEvent(ctx context.Context, ev seam.Envelope) (seam.Envelope, error)
	// ExecuteMethod 执行本插件登记给模型的方法。
	ExecuteMethod(ctx context.Context, in MethodInput) (MethodResult, error)
}

// AgentInputHandler 拦 agent/input。
type AgentInputHandler interface {
	// OnAgentInput 用户刚提交、Run 还没建。
	OnAgentInput(ctx context.Context, in AgentInput) (AgentInputResult, error)
}

// AgentPreStepHandler 拦 agent/pre-step。
type AgentPreStepHandler interface {
	// OnAgentPreStep 本轮第一拍，可改系统提示和隐藏消息。
	OnAgentPreStep(ctx context.Context, in AgentPreStep) (AgentPreStepResult, error)
}

// AgentRequestHandler 拦 agent/request。只能改数据。
type AgentRequestHandler interface {
	// OnAgentRequest 即将发给模型的提示和消息。
	OnAgentRequest(ctx context.Context, in AgentRequest) (AgentRequestResult, error)
}

// LLMStreamHandler 拦 llm/stream。只能改数据；fake 模型不经过这里。
type LLMStreamHandler interface {
	// OnLLMStream 即将发出的模型 HTTP 请求。
	OnLLMStream(ctx context.Context, in LLMStream) (LLMStreamResult, error)
}

// ToolPreExecuteHandler 拦 tools/pre-execute。已批准的调用不会再进。
type ToolPreExecuteHandler interface {
	// OnToolPreExecute 某个工具马上要跑。
	OnToolPreExecute(ctx context.Context, in ToolPreExecute) (ToolPreExecuteResult, error)
}

// ToolPostExecuteHandler 拦 tools/post-execute。只能改数据。
type ToolPostExecuteHandler interface {
	// OnToolPostExecute 工具已经跑完。
	OnToolPostExecute(ctx context.Context, in ToolPostExecute) (ToolPostExecuteResult, error)
}

// LedgerNotifyHandler 收账本通知（如 run.completed）。改回包没有换向效果。
type LedgerNotifyHandler interface {
	// OnLedgerNotify 处理非六个口的账本事件。
	OnLedgerNotify(ctx context.Context, in LedgerNotify) error
}

// Host 是插件回调宿主的白名单。
type Host interface {
	// Emit 另发一条与当前口无关的事件；不能发六个口的同名类型。
	Emit(ctx context.Context, ev seam.Envelope) error
	// RegisterMethod 给模型加方法；不能覆盖 ping / memory_*。
	RegisterMethod(ctx context.Context, method Method) error
	// MemoryGet 按会话读一篇专题记忆。
	MemoryGet(ctx context.Context, key MemoryKey) (string, error)
	// MemoryUpsert 按会话写一篇专题记忆。
	MemoryUpsert(ctx context.Context, key MemoryKey, text string) error
	// Complete 自己打一次模型，不进当前助手流。
	Complete(ctx context.Context, req CompleteRequest) (CompleteResult, error)
	// AppendNotice 写一条用户看得见的 system 消息（本期只落库）。
	AppendNotice(ctx context.Context, sessionID, runID, text string) error
}

// Manifest 是插件启动时声明的名字与订阅。
type Manifest struct {
	Name          string   // 日志用；空则用 PLUGIN_DIR 子目录名。Seen 永远是子目录名
	Subscriptions []string // 要听的口（TypeInput 等）或账本事件（如 run.completed）
}

// Method 是插件登记给模型的方法。
type Method struct {
	Name             string          // 模型看到的工具名；不能覆盖 ping / memory_*
	Prompt           string          // 什么时候该调这个方法
	ParametersSchema json.RawMessage // 模型填参用的 JSON Schema；ExecuteMethod 按同一份解 Arguments
	Capabilities     []string        // proto 保留字段，不映射到工具 Permission
	RequiresApproval bool            // true 时默认 EffectAsk，需过审批流水线
}

// MethodInput 是一次方法执行的入参。
type MethodInput struct {
	SessionID string          // 当前会话
	RunID     string          // 本轮 Run
	TurnID    string          // 当前 Turn
	CallID    string          // 这次调用号
	Name      string          // 被点到的方法名（一个插件可登记多个）
	Arguments json.RawMessage // 模型按 ParametersSchema 填的 JSON
}

// MethodResult 是一次方法执行的结果。业务失败用 Success:false，error 只表示取消或超时。
type MethodResult struct {
	Success bool            // 业务是否成功
	Output  json.RawMessage // 成功时的 JSON
	Error   string          // 失败原因
}

// MemoryKey 定位一篇专题记忆。
type MemoryKey struct {
	SessionID string // 当前会话
	Scope     string // 记忆范围
	Name      string // 专题名
}

// CompleteRequest 是插件自己打一次模型的入参。
type CompleteRequest struct {
	SessionID string // 当前会话
	RunID     string // 本轮 Run
	Prompt    string // 系统提示
	Text      string // 用户侧文本
}

// CompleteResult 是插件自己打一次模型的出参。
type CompleteResult struct {
	Text string // 模型回的正文
}

// HiddenText 构造一条不入库的隐藏 system 消息，供 agent/pre-step 注入。
func HiddenText(text string) pkgagent.Message {
	return pkgagent.Message{Role: pkgagent.RoleSystem, Content: pkgagent.EncodeText(text)}
}
