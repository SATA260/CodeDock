package codex

import (
	"encoding/json"
	"fmt"
	"strconv"
)

const (
	// MaxFrameBytes 是一条 JSONL 帧的上限。官方 diff 可能超过 64KiB。
	MaxFrameBytes = 16 << 20

	// ClientName 是握手时告诉 Codex 的客户端名。
	ClientName = "codedock"
	// ClientVersion 是握手时告诉 Codex 的客户端版本。
	ClientVersion = "0.1.0"

	MethodInitialize              = "initialize"
	MethodInitialized             = "initialized"
	MethodThreadStart             = "thread/start"
	MethodThreadResume            = "thread/resume"
	MethodThreadFork              = "thread/fork"
	MethodThreadArchive           = "thread/archive"
	MethodThreadUnarchive         = "thread/unarchive"
	MethodThreadNameSet           = "thread/name/set"
	MethodThreadList              = "thread/list"
	MethodThreadRead              = "thread/read"
	MethodThreadCompact           = "thread/compact/start"
	MethodTurnStart               = "turn/start"
	MethodTurnInterrupt           = "turn/interrupt"
	MethodReviewStart             = "review/start"
	MethodModelList               = "model/list"
	MethodPermissionProfileList   = "permissionProfile/list"
	MethodAccountRead             = "account/read"
	MethodConfigRead              = "config/read"
	MethodItemCommandApproval     = "item/commandExecution/requestApproval"
	MethodItemFileApproval        = "item/fileChange/requestApproval"
	MethodItemPermissionsApproval = "item/permissions/requestApproval"
	MethodItemToolUserInput       = "item/tool/requestUserInput"
	MethodMCPElicitation          = "mcpServer/elicitation/request"
	MethodExecCommandApproval     = "execCommandApproval"
	MethodApplyPatchApproval      = "applyPatchApproval"
	MethodServerRequestResolved   = "serverRequest/resolved"
	MethodTurnStarted             = "turn/started"
	MethodTurnCompleted           = "turn/completed"
	MethodItemStarted             = "item/started"
	MethodItemCompleted           = "item/completed"
	MethodAgentMessageDelta       = "item/agentMessage/delta"
	MethodReasoningTextDelta      = "item/reasoning/textDelta"
	MethodReasoningSummaryDelta   = "item/reasoning/summaryTextDelta"
	MethodCommandOutputDelta      = "item/commandExecution/outputDelta"
	MethodFileChangeDelta         = "item/fileChange/outputDelta"
	MethodPlanDelta               = "item/plan/delta"
	MethodError                   = "error"
	MethodThreadStarted           = "thread/started"
	MethodThreadTokenUsageUpdated = "thread/tokenUsage/updated"
	MethodTokenCount              = "codex/event/token_count"

	CodeParseError     = -32700
	CodeInvalidRequest = -32600
	CodeMethodNotFound = -32601
	CodeInvalidParams  = -32602
	CodeInternalError  = -32603
)

// RequestID 是 JSON-RPC 的请求编号，可能是字符串或整数。
type RequestID struct {
	str   string
	num   int64
	isNum bool
	set   bool
}

// IntID 构造整数请求编号。
func IntID(n int64) RequestID {
	return RequestID{num: n, isNum: true, set: true}
}

// StringID 构造字符串请求编号。
func StringID(s string) RequestID {
	return RequestID{str: s, set: true}
}

// ParseRequestID 把 URL 或文本里的编号还原；纯数字优先当整数。
func ParseRequestID(raw string) RequestID {
	if raw == "" {
		return RequestID{}
	}
	if n, err := strconv.ParseInt(raw, 10, 64); err == nil {
		return IntID(n)
	}
	return StringID(raw)
}

// IsZero 表示还没有编号。
func (id RequestID) IsZero() bool { return !id.set }

// String 返回给人看、给 URL 用的编号。
func (id RequestID) String() string {
	if !id.set {
		return ""
	}
	if id.isNum {
		return strconv.FormatInt(id.num, 10)
	}
	return id.str
}

// Key 区分「字符串 1」和整数 1，给 pending map 用。
func (id RequestID) Key() string {
	if !id.set {
		return ""
	}
	if id.isNum {
		return "n:" + strconv.FormatInt(id.num, 10)
	}
	return "s:" + id.str
}

// MarshalJSON 按官方协议写出字符串或整数。
func (id RequestID) MarshalJSON() ([]byte, error) {
	if !id.set {
		return []byte("null"), nil
	}
	if id.isNum {
		return json.Marshal(id.num)
	}
	return json.Marshal(id.str)
}

// UnmarshalJSON 读官方协议里的字符串或整数编号。
func (id *RequestID) UnmarshalJSON(data []byte) error {
	if string(data) == "null" || len(data) == 0 {
		*id = RequestID{}
		return nil
	}
	if data[0] == '"' {
		var s string
		if err := json.Unmarshal(data, &s); err != nil {
			return err
		}
		*id = StringID(s)
		return nil
	}
	var n int64
	if err := json.Unmarshal(data, &n); err != nil {
		return fmt.Errorf("request id: %w", err)
	}
	*id = IntID(n)
	return nil
}

// RPCError 是 JSON-RPC 错误体。
type RPCError struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

// Error 实现 error。
func (e RPCError) Error() string {
	if e.Message == "" {
		return fmt.Sprintf("rpc error %d", e.Code)
	}
	return fmt.Sprintf("rpc error %d: %s", e.Code, e.Message)
}

// Envelope 是一条 JSONL 上的 JSON-RPC 消息。官方省略 jsonrpc 字段。
type Envelope struct {
	ID     *RequestID      `json:"id,omitempty"`
	Method string          `json:"method,omitempty"`
	Params json.RawMessage `json:"params,omitempty"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  *RPCError       `json:"error,omitempty"`
}

// Kind 是这条消息在协议里的角色。
type Kind int

const (
	KindUnknown Kind = iota
	KindRequest
	KindNotification
	KindResponse
)

// Classify 判断信封是请求、通知还是响应。
func Classify(env Envelope) Kind {
	switch {
	case env.Method != "" && env.ID != nil && !env.ID.IsZero():
		return KindRequest
	case env.Method != "" && (env.ID == nil || env.ID.IsZero()):
		return KindNotification
	case env.ID != nil && !env.ID.IsZero():
		return KindResponse
	default:
		return KindUnknown
	}
}

// Message 是读循环交给上层的一条入站消息。
type Message struct {
	Kind   Kind
	ID     RequestID
	Method string
	Params json.RawMessage
	Result json.RawMessage
	Error  *RPCError
}

// ClientInfo 是 initialize 时带的客户端信息。
type ClientInfo struct {
	Name    string `json:"name"`
	Title   string `json:"title,omitempty"`
	Version string `json:"version"`
}

// InitializeParams 是握手请求。
type InitializeParams struct {
	ClientInfo   ClientInfo             `json:"clientInfo"`
	Capabilities map[string]interface{} `json:"capabilities,omitempty"`
}

// InitializeResult 是握手响应。
type InitializeResult struct {
	CodexHome      string `json:"codexHome"`
	PlatformFamily string `json:"platformFamily"`
	PlatformOS     string `json:"platformOs"`
	UserAgent      string `json:"userAgent"`
}
