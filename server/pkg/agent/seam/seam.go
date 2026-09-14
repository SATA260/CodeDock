package seam

import (
	"context"
	"encoding/json"
)

const (
	TypeInput       = "agent/input"
	TypePreStep     = "agent/pre-step"
	TypeRequest     = "agent/request"
	TypeStream      = "llm/stream"
	TypePreExecute  = "tools/pre-execute"
	TypePostExecute = "tools/post-execute"

	TypeInputHandled = "input/handled"
	TypeRunBlocked   = "run/blocked"
	TypeToolsDenied  = "tools/denied"
	TypeToolsAsk     = "tools/ask"
)

// Envelope 是一次喊话：当前口、已问过谁、以及还能改的数据。
type Envelope struct {
	Type      string          `json:"type"`           // 口或事件名
	ChainID   string          `json:"chain_id"`       // 这一次 Dispatch 的链，宿主生成
	Seen      []string        `json:"seen,omitempty"` // 这条链上已经问过的插件（目录名）
	SessionID string          `json:"session_id,omitempty"`
	RunID     string          `json:"run_id,omitempty"`
	TurnID    string          `json:"turn_id,omitempty"`
	Payload   json.RawMessage `json:"payload,omitempty"` // 该口的 JSON 载荷
	Context   json.RawMessage `json:"context,omitempty"` // 插件共享袋子，不进模型、不换向
}

// Dispatcher 把信封交给订阅者。没有实现时由 Dispatch 原样返回。
type Dispatcher interface {
	// Dispatch 按订阅依次处理信封；类型变了应立刻返回。
	Dispatch(ctx context.Context, ev Envelope) (Envelope, error)
}

// Dispatch 是所有口统一调用的入口：d 为 nil 时原信封原样返回。
func Dispatch(ctx context.Context, d Dispatcher, ev Envelope) (Envelope, error) {
	if d == nil {
		return ev, nil
	}
	return d.Dispatch(ctx, ev)
}

// Func 让测试用闭包充当 Dispatcher。
type Func func(ctx context.Context, ev Envelope) (Envelope, error)

// Dispatch 调用闭包；闭包为 nil 时原信封原样返回。
func (f Func) Dispatch(ctx context.Context, ev Envelope) (Envelope, error) {
	if f == nil {
		return ev, nil
	}
	return f(ctx, ev)
}

// IsSeam 判断 typ 是否为六个拦截口之一。
func IsSeam(typ string) bool {
	switch typ {
	case TypeInput, TypePreStep, TypeRequest, TypeStream, TypePreExecute, TypePostExecute:
		return true
	default:
		return false
	}
}
