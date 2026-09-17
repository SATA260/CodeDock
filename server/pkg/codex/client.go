package codex

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"
	"sync/atomic"
)

// Client 是并发安全的 JSON-RPC 客户端：单读循环、乱序响应分发、写锁由 Transport 负责。
type Client struct {
	t        Transport
	pending  map[string]chan rpcResult
	mu       sync.Mutex
	nextID   atomic.Int64
	incoming chan Message
	done     chan struct{}
	err      error
	errOnce  sync.Once
}

type rpcResult struct {
	raw json.RawMessage
	err error
}

// NewClient 启动读循环。调用方负责 Handshake。
func NewClient(t Transport) *Client {
	c := &Client{
		t:        t,
		pending:  make(map[string]chan rpcResult),
		incoming: make(chan Message, 256),
		done:     make(chan struct{}),
	}
	go c.readLoop()
	return c
}

// Incoming 返回通知和来自服务端的请求。响应不会出现在这里。
func (c *Client) Incoming() <-chan Message {
	return c.incoming
}

// Done 在读循环结束后关闭。
func (c *Client) Done() <-chan struct{} {
	return c.done
}

// Err 返回读循环结束原因。
func (c *Client) Err() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.err
}

// Close 关掉传输并结束读循环。
func (c *Client) Close() error {
	err := c.t.Close()
	<-c.done
	return err
}

// Handshake 发送 initialize 再发 initialized 通知。
func (c *Client) Handshake(ctx context.Context, info ClientInfo) (InitializeResult, error) {
	var out InitializeResult
	if err := c.Call(ctx, MethodInitialize, initializeParams(info), &out); err != nil {
		return InitializeResult{}, err
	}
	if err := c.Notify(ctx, MethodInitialized, map[string]any{}); err != nil {
		return InitializeResult{}, err
	}
	return out, nil
}

// Call 发一条有编号的请求并等待响应。取消后迟到的响应会被丢掉。
func (c *Client) Call(ctx context.Context, method string, params any, result any) error {
	raw, err := c.CallRaw(ctx, method, params)
	if err != nil {
		return err
	}
	if result == nil || len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	if err := json.Unmarshal(raw, result); err != nil {
		return fmt.Errorf("decode %s result: %w", method, err)
	}
	return nil
}

// CallRaw 发请求并返回原始 result。
func (c *Client) CallRaw(ctx context.Context, method string, params any) (json.RawMessage, error) {
	id := IntID(c.nextID.Add(1))
	ch := make(chan rpcResult, 1)
	c.mu.Lock()
	if c.err != nil {
		err := c.err
		c.mu.Unlock()
		return nil, err
	}
	c.pending[id.Key()] = ch
	c.mu.Unlock()

	if err := c.send(ctx, &Envelope{ID: &id, Method: method, Params: mustParams(params)}); err != nil {
		c.drop(id)
		return nil, err
	}

	select {
	case <-ctx.Done():
		c.drop(id)
		return nil, ctx.Err()
	case <-c.done:
		c.drop(id)
		if err := c.Err(); err != nil {
			return nil, err
		}
		return nil, io.EOF
	case out := <-ch:
		return out.raw, out.err
	}
}

// Notify 发一条没有编号的通知。
func (c *Client) Notify(ctx context.Context, method string, params any) error {
	return c.send(ctx, &Envelope{Method: method, Params: mustParams(params)})
}

// Reply 把服务端请求的结果写回去。
func (c *Client) Reply(ctx context.Context, id RequestID, result any) error {
	raw, err := json.Marshal(result)
	if err != nil {
		return err
	}
	return c.send(ctx, &Envelope{ID: &id, Result: raw})
}

// ReplyError 把服务端请求按错误写回去。
func (c *Client) ReplyError(ctx context.Context, id RequestID, code int, message string) error {
	return c.send(ctx, &Envelope{ID: &id, Error: &RPCError{Code: code, Message: message}})
}

func (c *Client) send(ctx context.Context, env *Envelope) error {
	body, err := json.Marshal(env)
	if err != nil {
		return err
	}
	return c.t.Write(ctx, body)
}

func (c *Client) drop(id RequestID) {
	c.mu.Lock()
	delete(c.pending, id.Key())
	c.mu.Unlock()
}

func (c *Client) readLoop() {
	defer close(c.done)
	ctx := context.Background()
	for {
		frame, err := c.t.Read(ctx)
		if err != nil {
			c.fail(err)
			return
		}
		var env Envelope
		if err := json.Unmarshal(frame, &env); err != nil {
			c.fail(fmt.Errorf("bad jsonl: %w", err))
			return
		}
		c.dispatch(env)
	}
}

func (c *Client) dispatch(env Envelope) {
	switch Classify(env) {
	case KindResponse:
		if env.ID == nil {
			return
		}
		c.mu.Lock()
		ch, ok := c.pending[env.ID.Key()]
		if ok {
			delete(c.pending, env.ID.Key())
		}
		c.mu.Unlock()
		if !ok {
			return
		}
		if env.Error != nil {
			ch <- rpcResult{err: *env.Error}
			return
		}
		ch <- rpcResult{raw: env.Result}
	case KindRequest, KindNotification:
		msg := Message{
			Kind:   Classify(env),
			Method: env.Method,
			Params: env.Params,
		}
		if env.ID != nil {
			msg.ID = *env.ID
		}
		select {
		case c.incoming <- msg:
		case <-c.done:
		}
	}
}

func (c *Client) fail(err error) {
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrClosedPipe) {
		err = io.EOF
	}
	c.errOnce.Do(func() {
		c.mu.Lock()
		c.err = err
		pending := c.pending
		c.pending = map[string]chan rpcResult{}
		c.mu.Unlock()
		for _, ch := range pending {
			ch <- rpcResult{err: err}
		}
		close(c.incoming)
	})
}

func mustParams(params any) json.RawMessage {
	if params == nil {
		return nil
	}
	if raw, ok := params.(json.RawMessage); ok {
		return raw
	}
	body, err := json.Marshal(params)
	if err != nil {
		return json.RawMessage(`{}`)
	}
	if string(body) == "null" {
		return nil
	}
	return body
}
