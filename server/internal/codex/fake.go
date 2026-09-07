package codex

import (
	"encoding/json"
	"strconv"
	"sync"
	"sync/atomic"

	pkg "codedock/pkg/codex"
)

// FakeHandler 是覆盖本模块用到的官方方法的假 app-server。
type FakeHandler struct {
	mu         sync.Mutex
	Threads    map[string]pkg.ThreadObject
	ThreadsN   atomic.Int64
	Turns      atomic.Int64
	Asks       atomic.Int64
	Authorized bool
	SendAsk    bool
	Unknown    bool
	FailTurn   bool
	OnRequest  func(pkg.Envelope)
}

// NewFakeHandler 默认已授权、可开会话。
func NewFakeHandler() *FakeHandler {
	return &FakeHandler{
		Threads:    map[string]pkg.ThreadObject{},
		Authorized: true,
	}
}

// Handle 实现假 app-server。
func (f *FakeHandler) Handle(env pkg.Envelope) []pkg.Envelope {
	if f.OnRequest != nil {
		f.OnRequest(env)
	}
	if env.Method == "" {
		return nil
	}
	switch env.Method {
	case pkg.MethodInitialize:
		return []pkg.Envelope{{ID: env.ID, Result: json.RawMessage(`{"codexHome":"/tmp/codex","platformFamily":"unix","platformOs":"macos","userAgent":"codex"}`)}}
	case pkg.MethodInitialized:
		return nil
	case pkg.MethodAccountRead:
		if f.Authorized {
			return []pkg.Envelope{{ID: env.ID, Result: json.RawMessage(`{"requiresOpenaiAuth":false,"account":{"type":"apiKey"}}`)}}
		}
		return []pkg.Envelope{{ID: env.ID, Result: json.RawMessage(`{"requiresOpenaiAuth":true,"account":null}`)}}
	case pkg.MethodModelList:
		cursor := jsonField(env.Params, "cursor")
		if cursor == "" {
			return []pkg.Envelope{{ID: env.ID, Result: json.RawMessage(`{"data":[{"id":"gpt-5.6","displayName":"GPT","defaultReasoningEffort":"medium","supportedReasoningEfforts":["low","medium",{"effort":"high"},{"id":"minimal"}],"hidden":false,"isDefault":true},"bad"],"nextCursor":"p2"}`)}}
		}
		return []pkg.Envelope{{ID: env.ID, Result: json.RawMessage(`{"data":[{"model":"gpt-other","displayName":"Other","supportedReasoningEfforts":[],"hidden":true,"isDefault":false}],"nextCursor":null}`)}}
	case pkg.MethodPermissionProfileList:
		cursor := jsonField(env.Params, "cursor")
		if cursor == "" {
			return []pkg.Envelope{{ID: env.ID, Result: json.RawMessage(`{"data":[{"id":"read-only","description":"Read only","allowed":true},"bad"],"nextCursor":"p2"}`)}}
		}
		return []pkg.Envelope{{ID: env.ID, Result: json.RawMessage(`{"data":[{"id":"full-access","description":"Full","allowed":true}],"nextCursor":null}`)}}
	case pkg.MethodConfigRead:
		return []pkg.Envelope{{ID: env.ID, Result: json.RawMessage(`{"config":{"model":"gpt-5.6","model_reasoning_effort":"medium","approval_policy":"on-request","sandbox_mode":"workspace-write"},"origins":{}}`)}}
	case pkg.MethodThreadStart:
		th := f.addThread("thread-"+itoa(f.ThreadsN.Add(1)), "New thread")
		body, _ := json.Marshal(map[string]any{"thread": th, "model": "gpt-5.6", "cwd": "/tmp", "approvalPolicy": "on-request", "sandbox": "workspace-write"})
		return []pkg.Envelope{{ID: env.ID, Result: body}}
	case pkg.MethodThreadList:
		f.mu.Lock()
		data := make([]pkg.ThreadObject, 0, len(f.Threads))
		for _, th := range f.Threads {
			data = append(data, th)
		}
		f.mu.Unlock()
		body, _ := json.Marshal(map[string]any{"data": data})
		return []pkg.Envelope{{ID: env.ID, Result: body}}
	case pkg.MethodThreadRead, pkg.MethodThreadResume:
		id := jsonField(env.Params, "threadId")
		f.mu.Lock()
		th, ok := f.Threads[id]
		f.mu.Unlock()
		if !ok {
			return []pkg.Envelope{{ID: env.ID, Error: &pkg.RPCError{Code: -32602, Message: "unknown thread"}}}
		}
		if env.Method == pkg.MethodThreadRead {
			body, _ := json.Marshal(map[string]any{"thread": th})
			return []pkg.Envelope{{ID: env.ID, Result: body}}
		}
		body, _ := json.Marshal(map[string]any{"thread": th, "model": "gpt-5.6", "cwd": "/tmp", "approvalPolicy": "on-request", "sandbox": "workspace-write"})
		return []pkg.Envelope{{ID: env.ID, Result: body}}
	case pkg.MethodThreadFork:
		th := f.addThread("thread-fork-"+itoa(f.ThreadsN.Add(1)), "Fork")
		body, _ := json.Marshal(map[string]any{"thread": th, "model": "gpt-5.6", "cwd": "/tmp", "approvalPolicy": "on-request", "sandbox": "workspace-write"})
		return []pkg.Envelope{{ID: env.ID, Result: body}}
	case pkg.MethodThreadArchive, pkg.MethodThreadUnarchive, pkg.MethodThreadNameSet, pkg.MethodThreadCompact, pkg.MethodReviewStart, pkg.MethodTurnInterrupt:
		if env.Method == pkg.MethodTurnInterrupt {
			tid := jsonField(env.Params, "threadId")
			turnID := jsonField(env.Params, "turnId")
			completed, _ := json.Marshal(map[string]any{"threadId": tid, "turn": map[string]any{"id": turnID, "status": "interrupted", "items": []any{}}})
			return []pkg.Envelope{
				{ID: env.ID, Result: json.RawMessage(`{}`)},
				{Method: pkg.MethodTurnCompleted, Params: completed},
			}
		}
		return []pkg.Envelope{{ID: env.ID, Result: json.RawMessage(`{}`)}}
	case pkg.MethodTurnStart:
		turnID := "turn-" + itoa(f.Turns.Add(1))
		threadID := jsonField(env.Params, "threadId")
		result, _ := json.Marshal(map[string]any{"turn": map[string]any{"id": turnID, "status": "inProgress", "items": []any{}}})
		delta, _ := json.Marshal(map[string]any{"threadId": threadID, "turnId": turnID, "itemId": "m1", "delta": "hello"})
		reason, _ := json.Marshal(map[string]any{"threadId": threadID, "turnId": turnID, "itemId": "r1", "delta": "think"})
		cmdOut, _ := json.Marshal(map[string]any{"threadId": threadID, "turnId": turnID, "itemId": "c1", "delta": "out"})
		fileDelta, _ := json.Marshal(map[string]any{"threadId": threadID, "turnId": turnID, "itemId": "f1", "delta": "+x"})
		planDelta, _ := json.Marshal(map[string]any{"threadId": threadID, "turnId": turnID, "itemId": "p1", "delta": "step"})
		errNote, _ := json.Marshal(map[string]any{"threadId": threadID, "message": "warn"})
		item, _ := json.Marshal(map[string]any{"threadId": threadID, "turnId": turnID, "item": map[string]any{"id": "m1", "type": "agentMessage", "text": "hello"}, "completedAtMs": 1})
		cmdItem, _ := json.Marshal(map[string]any{"threadId": threadID, "turnId": turnID, "item": map[string]any{"id": "c1", "type": "commandExecution", "command": "ls", "aggregatedOutput": "ok", "status": "completed"}})
		fileItem, _ := json.Marshal(map[string]any{"threadId": threadID, "turnId": turnID, "item": map[string]any{"id": "f1", "type": "fileChange", "status": "completed", "changes": []any{map[string]any{"path": "a.go", "diff": "+x"}}}})
		planItem, _ := json.Marshal(map[string]any{"threadId": threadID, "turnId": turnID, "item": map[string]any{"id": "p1", "type": "plan", "text": "do"}})
		reasonItem, _ := json.Marshal(map[string]any{"threadId": threadID, "turnId": turnID, "item": map[string]any{"id": "r1", "type": "reasoning", "summary": []any{"think"}}})
		userItem, _ := json.Marshal(map[string]any{"threadId": threadID, "turnId": turnID, "item": map[string]any{"id": "u1", "type": "userMessage", "content": []any{map[string]any{"type": "text", "text": "hi"}}}})
		otherItem, _ := json.Marshal(map[string]any{"threadId": threadID, "turnId": turnID, "item": map[string]any{"id": "x1", "type": "todoList"}})
		done, _ := json.Marshal(map[string]any{"threadId": threadID, "turn": map[string]any{"id": turnID, "status": "completed", "items": []any{}}})
		failed, _ := json.Marshal(map[string]any{"threadId": threadID, "turn": map[string]any{"id": turnID, "status": "failed", "error": map[string]any{"message": "boom"}, "items": []any{}}})
		out := []pkg.Envelope{
			{ID: env.ID, Result: result},
			{Method: pkg.MethodTurnStarted, Params: mustRaw(map[string]any{"threadId": threadID, "turn": map[string]any{"id": turnID, "status": "inProgress"}})},
			{Method: pkg.MethodAgentMessageDelta, Params: delta},
			{Method: pkg.MethodReasoningTextDelta, Params: reason},
			{Method: pkg.MethodCommandOutputDelta, Params: cmdOut},
			{Method: pkg.MethodFileChangeDelta, Params: fileDelta},
			{Method: pkg.MethodPlanDelta, Params: planDelta},
			{Method: pkg.MethodError, Params: errNote},
			{Method: pkg.MethodItemStarted, Params: item},
			{Method: pkg.MethodItemCompleted, Params: item},
			{Method: pkg.MethodItemCompleted, Params: cmdItem},
			{Method: pkg.MethodItemCompleted, Params: fileItem},
			{Method: pkg.MethodItemCompleted, Params: planItem},
			{Method: pkg.MethodItemCompleted, Params: reasonItem},
			{Method: pkg.MethodItemCompleted, Params: userItem},
			{Method: pkg.MethodItemCompleted, Params: otherItem},
		}
		if f.Unknown {
			reqID := pkg.IntID(900)
			out = append(out, pkg.Envelope{ID: &reqID, Method: "future/unknown", Params: json.RawMessage(`{"threadId":"` + threadID + `"}`)})
		}
		if f.FailTurn {
			out = append(out, pkg.Envelope{Method: pkg.MethodTurnCompleted, Params: failed})
			return out
		}
		if f.SendAsk {
			askID := pkg.IntID(800 + f.Asks.Add(1))
			params, _ := json.Marshal(map[string]any{"threadId": threadID, "turnId": turnID, "itemId": "c1", "command": "ls", "startedAtMs": 1})
			out = append(out, pkg.Envelope{ID: &askID, Method: pkg.MethodItemCommandApproval, Params: params})
			return out
		}
		out = append(out, pkg.Envelope{Method: pkg.MethodTurnCompleted, Params: done})
		return out
	default:
		return []pkg.Envelope{{ID: env.ID, Error: &pkg.RPCError{Code: pkg.CodeMethodNotFound, Message: env.Method}}}
	}
}

func (f *FakeHandler) addThread(id, name string) pkg.ThreadObject {
	th := pkg.ThreadObject{
		ID: id, Name: name, Preview: name, Cwd: "/tmp", CreatedAt: 1, UpdatedAt: 1,
		Turns: []pkg.TurnObject{{
			ID:     "hist-1",
			Status: "completed",
			Items:  []json.RawMessage{json.RawMessage(`{"id":"u","type":"userMessage","content":[{"type":"text","text":"hi"}]}`)},
		}},
	}
	f.mu.Lock()
	f.Threads[id] = th
	f.mu.Unlock()
	return th
}

func jsonField(raw json.RawMessage, key string) string {
	var obj map[string]any
	_ = json.Unmarshal(raw, &obj)
	s, _ := obj[key].(string)
	return s
}

func mustRaw(v any) json.RawMessage {
	body, _ := json.Marshal(v)
	return body
}

func itoa(n int64) string {
	return strconv.FormatInt(n, 10)
}
