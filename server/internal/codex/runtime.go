package codex

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"strconv"
	"strings"
	"sync"

	cderr "codedock/internal/errors"
	pkg "codedock/pkg/codex"
)

// Options 装配本机 Codex 运行时。
type Options struct {
	Bin        string
	LookPath   LookPath
	Version    Versioner
	Start      Starter
	ClientInfo pkg.ClientInfo
}

// Runtime 管本机 app-server 生命周期和进程内排队、草稿、问票、SSE。
type Runtime struct {
	bin        string
	lookPath   LookPath
	version    Versioner
	start      Starter
	clientInfo pkg.ClientInfo

	connMu sync.Mutex
	client *pkg.Client
	proc   Proc

	mu        sync.Mutex
	mem       map[string]*sessionMem
	listeners map[string][]chan pkg.Event
}

type sessionMem struct {
	mu       sync.Mutex
	settings pkg.Settings
	draft    pkg.Input
	active   *pkg.Turn
	queue    []queuedTurn
	asks     map[string]*askMem
	ring     *eventRing
	archived bool
	loaded   bool
	usage    pkg.TokenUsage
}

type queuedTurn struct {
	turn  pkg.Turn
	input pkg.Input
}

type askMem struct {
	ask      pkg.ApprovalAsk
	rpcID    pkg.RequestID
	resolved bool
}

// New 构造运行时。未安装 Codex 时仍可 Probe，不会拖垮主服务。
func New(opts Options) *Runtime {
	if opts.LookPath == nil {
		opts.LookPath = DefaultLookPath
	}
	if opts.Version == nil {
		opts.Version = DefaultVersioner
	}
	if opts.Start == nil {
		opts.Start = DefaultStarter
	}
	if opts.Bin == "" {
		opts.Bin = "codex"
	}
	if opts.ClientInfo.Name == "" {
		opts.ClientInfo = pkg.DefaultClientInfo()
	}
	return &Runtime{
		bin:        opts.Bin,
		lookPath:   opts.LookPath,
		version:    opts.Version,
		start:      opts.Start,
		clientInfo: opts.ClientInfo,
		mem:        map[string]*sessionMem{},
		listeners:  map[string][]chan pkg.Event{},
	}
}

// Close 杀掉当前 app-server。进行中的回合按失败处理，不重发。
func (rt *Runtime) Close() error {
	rt.connMu.Lock()
	defer rt.connMu.Unlock()
	rt.dropConnLocked(io.EOF)
	return nil
}

func (rt *Runtime) state(id string) *sessionMem {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	return rt.stateLocked(id)
}

func (rt *Runtime) stateLocked(id string) *sessionMem {
	st, ok := rt.mem[id]
	if !ok {
		st = &sessionMem{asks: map[string]*askMem{}, ring: newRing()}
		rt.mem[id] = st
	}
	return st
}

func (rt *Runtime) emit(sessionID string, ev pkg.Event) pkg.Event {
	st := rt.state(sessionID)
	ev.SessionID = sessionID
	ev = st.ring.append(ev)
	rt.mu.Lock()
	subs := append([]chan pkg.Event(nil), rt.listeners[sessionID]...)
	rt.mu.Unlock()
	for _, ch := range subs {
		select {
		case ch <- ev:
		default:
		}
	}
	return ev
}

// Subscribe 订阅一条对话的进程内事件。
func (rt *Runtime) Subscribe(sessionID string) (<-chan pkg.Event, func()) {
	ch := make(chan pkg.Event, 64)
	rt.mu.Lock()
	rt.listeners[sessionID] = append(rt.listeners[sessionID], ch)
	rt.mu.Unlock()
	return ch, func() {
		rt.mu.Lock()
		defer rt.mu.Unlock()
		list := rt.listeners[sessionID]
		out := list[:0]
		for _, item := range list {
			if item != ch {
				out = append(out, item)
			}
		}
		if len(out) == 0 {
			delete(rt.listeners, sessionID)
		} else {
			rt.listeners[sessionID] = out
		}
		close(ch)
	}
}

// Events 按 seq 回放内存环。缺口时 reset=true，调用方应再 thread/read。
func (rt *Runtime) Events(sessionID string, after int64) (events []pkg.Event, reset bool) {
	return rt.state(sessionID).ring.after(after)
}

func (rt *Runtime) ensureClient(ctx context.Context) (*pkg.Client, error) {
	rt.connMu.Lock()
	defer rt.connMu.Unlock()
	if rt.client != nil {
		select {
		case <-rt.client.Done():
			rt.dropConnLocked(rt.client.Err())
		default:
			return rt.client, nil
		}
	}
	path, err := rt.lookPath(rt.bin)
	if err != nil {
		return nil, cderr.Unavailable("codex is not installed")
	}
	proc, err := rt.start(ctx, path)
	if err != nil {
		return nil, cderr.Unavailable("start codex app-server: %s", err.Error())
	}
	Drain(proc.Stderr())
	transport := pkg.NewJSONL(proc.Stdout(), proc.Stdin(), proc.Stdin(), 0)
	client := pkg.NewClient(transport)
	if _, err := client.Handshake(ctx, rt.clientInfo); err != nil {
		_ = proc.Kill()
		_ = client.Close()
		_ = proc.Wait()
		return nil, cderr.Unavailable("codex handshake: %s", err.Error())
	}
	rt.proc = proc
	rt.client = client
	go rt.consume(client)
	go func() {
		_ = proc.Wait()
		rt.connMu.Lock()
		if rt.client == client {
			rt.dropConnLocked(io.EOF)
		}
		rt.connMu.Unlock()
	}()
	return client, nil
}

func (rt *Runtime) dropConnLocked(cause error) {
	proc, client := rt.proc, rt.client
	rt.proc = nil
	rt.client = nil
	if proc != nil {
		_ = proc.Kill()
	}
	if client != nil {
		_ = client.Close()
	}
	rt.failLiveTurns(cause)
}

func (rt *Runtime) failLiveTurns(cause error) {
	msg := "codex app-server exited"
	if cause != nil && !errors.Is(cause, io.EOF) {
		msg = cause.Error()
	}
	rt.mu.Lock()
	ids := make([]string, 0, len(rt.mem))
	for id := range rt.mem {
		ids = append(ids, id)
	}
	rt.mu.Unlock()
	for _, id := range ids {
		st := rt.state(id)
		st.mu.Lock()
		if st.active != nil && (st.active.Status == pkg.TurnRunning || st.active.Status == pkg.TurnWaitingApproval) {
			st.active.Status = pkg.TurnFailed
			st.active.Error = msg
			turn := *st.active
			st.mu.Unlock()
			rt.emit(id, pkg.Event{Type: pkg.EventTurnFailed, TurnID: turn.ID, Turn: &turn, Notice: msg})
			continue
		}
		st.mu.Unlock()
	}
}

func (rt *Runtime) consume(client *pkg.Client) {
	for msg := range client.Incoming() {
		switch msg.Kind {
		case pkg.KindRequest:
			rt.onServerRequest(client, msg)
		case pkg.KindNotification:
			rt.onNotification(msg)
		}
	}
}

func (rt *Runtime) onServerRequest(client *pkg.Client, msg pkg.Message) {
	ctx := context.Background()
	if ask, ok := pkg.ParseAsk(msg); ok {
		st := rt.state(ask.ThreadID)
		st.mu.Lock()
		if st.asks == nil {
			st.asks = map[string]*askMem{}
		}
		st.asks[ask.ID] = &askMem{ask: ask, rpcID: msg.ID}
		if st.active != nil {
			st.active.Status = pkg.TurnWaitingApproval
		}
		st.mu.Unlock()
		copied := ask
		rt.emit(ask.ThreadID, pkg.Event{Type: pkg.EventAskRequired, TurnID: ask.TurnID, Ask: &copied})
		return
	}
	notice := "Codex 问了本模块接不住的事，已按不支持回包。"
	rt.emit(threadIDOf(msg), pkg.Event{Type: pkg.EventNotice, Notice: notice, Progress: &pkg.Progress{Kind: pkg.ProgressNotice, Text: notice}})
	_ = client.ReplyError(ctx, msg.ID, pkg.CodeMethodNotFound, "method not supported by codedock")
}

func (rt *Runtime) Usage(sessionID string) pkg.TokenUsage {
	st := rt.state(sessionID)
	st.mu.Lock()
	defer st.mu.Unlock()
	return st.usage
}

func (rt *Runtime) onNotification(msg pkg.Message) {
	if usage, ok := pkg.ParseTokenUsage(msg); ok {
		st := rt.state(usage.ThreadID)
		st.mu.Lock()
		st.usage = usage.Usage
		st.mu.Unlock()
		got := usage.Usage
		rt.emit(usage.ThreadID, pkg.Event{Type: pkg.EventTokenUsage, TurnID: usage.TurnID, Usage: &got})
		return
	}
	threadID, turnID := idsOf(msg)
	switch msg.Method {
	case pkg.MethodServerRequestResolved:
		rt.resolveAsk(threadID, stringIDOf(msg, "requestId"))
	case pkg.MethodTurnCompleted:
		rt.finishTurn(threadID, turnID, msg)
	case pkg.MethodTurnStarted:
		st := rt.state(threadID)
		st.mu.Lock()
		if st.active != nil && st.active.Status != pkg.TurnWaitingApproval {
			st.active.Status = pkg.TurnRunning
		}
		st.mu.Unlock()
	}
	if ev, ok := pkg.MapNotification(msg); ok {
		p := ev.Progress
		sid := ev.ThreadID
		if sid == "" {
			sid = threadID
		}
		rt.emit(sid, pkg.Event{Type: pkg.EventProgress, TurnID: first(ev.TurnID, turnID), Progress: &p})
	}
}

func (rt *Runtime) finishTurn(threadID, turnID string, msg pkg.Message) {
	st := rt.state(threadID)
	st.mu.Lock()
	var finished *pkg.Turn
	if st.active != nil && (st.active.CodexID == turnID || st.active.ID == turnID || turnID == "") {
		status := pkg.MapTurnStatus(turnStatusOf(msg), false)
		st.active.Status = status
		if status == pkg.TurnFailed {
			st.active.Error = turnErrorOf(msg)
		}
		copyTurn := *st.active
		finished = &copyTurn
		st.active = nil
		for id, ask := range st.asks {
			if !ask.resolved {
				ask.resolved = true
			}
			delete(st.asks, id)
		}
	}
	next := queuedTurn{}
	hasNext := false
	if len(st.queue) > 0 {
		next = st.queue[0]
		st.queue = st.queue[1:]
		hasNext = true
	}
	st.mu.Unlock()
	if finished != nil {
		typ := pkg.EventTurnCompleted
		switch finished.Status {
		case pkg.TurnFailed:
			typ = pkg.EventTurnFailed
		case pkg.TurnCancelled:
			typ = pkg.EventTurnCancelled
		}
		rt.emit(threadID, pkg.Event{Type: typ, TurnID: finished.ID, Turn: finished})
	}
	if hasNext {
		_, _ = rt.dispatchTurn(context.Background(), threadID, next.turn, next.input)
	}
}

func (rt *Runtime) resolveAsk(threadID, requestID string) {
	if threadID == "" || requestID == "" {
		return
	}
	st := rt.state(threadID)
	st.mu.Lock()
	ask, ok := st.asks[requestID]
	if ok {
		ask.resolved = true
		copied := ask.ask
		st.mu.Unlock()
		rt.emit(threadID, pkg.Event{Type: pkg.EventAskResolved, TurnID: copied.TurnID, Ask: &copied})
		return
	}
	st.mu.Unlock()
}

func mapRPC(err error) error {
	if err == nil {
		return nil
	}
	var rpc pkg.RPCError
	if errors.As(err, &rpc) {
		lower := strings.ToLower(rpc.Message)
		if strings.Contains(lower, "not found") || strings.Contains(lower, "unknown thread") {
			return cderr.NotFound("%s", rpc.Message)
		}
		if strings.Contains(lower, "not initialized") {
			return cderr.Unavailable("%s", rpc.Message)
		}
		return cderr.Invalid("%s", rpc.Message)
	}
	if errors.Is(err, context.Canceled) {
		return err
	}
	return err
}

func threadIDOf(msg pkg.Message) string {
	id, _ := idsOf(msg)
	return id
}

func idsOf(msg pkg.Message) (threadID, turnID string) {
	ev, ok := pkg.MapNotification(msg)
	if ok {
		return ev.ThreadID, ev.TurnID
	}
	var raw struct {
		ThreadID  string `json:"threadId"`
		TurnID    string `json:"turnId"`
		RequestID any    `json:"requestId"`
	}
	_ = json.Unmarshal(msg.Params, &raw)
	return raw.ThreadID, raw.TurnID
}

func stringIDOf(msg pkg.Message, field string) string {
	var raw map[string]any
	if json.Unmarshal(msg.Params, &raw) != nil {
		return ""
	}
	switch v := raw[field].(type) {
	case string:
		return v
	case float64:
		return strconv.FormatInt(int64(v), 10)
	default:
		return ""
	}
}

func turnStatusOf(msg pkg.Message) string {
	var raw struct {
		Turn struct {
			ID     string `json:"id"`
			Status string `json:"status"`
		} `json:"turn"`
	}
	_ = json.Unmarshal(msg.Params, &raw)
	return raw.Turn.Status
}

func turnErrorOf(msg pkg.Message) string {
	var raw struct {
		Turn struct {
			Error struct {
				Message string `json:"message"`
			} `json:"error"`
		} `json:"turn"`
	}
	_ = json.Unmarshal(msg.Params, &raw)
	return raw.Turn.Error.Message
}

func first(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
