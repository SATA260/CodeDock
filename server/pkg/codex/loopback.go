package codex

import (
	"context"
	"encoding/json"
)

// StartLoopback 在管道对面跑假 app-server，返回已连接的 Client（尚未 Handshake）。
func StartLoopback(handler func(Envelope) []Envelope) *Client {
	left, right := PipePair()
	go Serve(right, handler)
	return NewClient(left)
}

// Serve 在 Transport 上按 handler 回答请求，直到读失败。
func Serve(t Transport, handler func(Envelope) []Envelope) {
	ctx := context.Background()
	for {
		frame, err := t.Read(ctx)
		if err != nil {
			_ = t.Close()
			return
		}
		var env Envelope
		if err := json.Unmarshal(frame, &env); err != nil {
			_ = t.Close()
			return
		}
		replies := handler(env)
		for _, reply := range replies {
			body, err := json.Marshal(reply)
			if err != nil {
				continue
			}
			if err := t.Write(ctx, body); err != nil {
				_ = t.Close()
				return
			}
		}
	}
}
