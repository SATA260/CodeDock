package codex

import (
	"context"

	cderr "codedock/internal/errors"
	pkg "codedock/pkg/codex"
)

// PendingAsks 列出这条对话还没作答的反问。
func (rt *Runtime) PendingAsks(sessionID string) []pkg.ApprovalAsk {
	st := rt.state(sessionID)
	st.mu.Lock()
	defer st.mu.Unlock()
	out := make([]pkg.ApprovalAsk, 0, len(st.asks))
	for _, item := range st.asks {
		if item.resolved {
			continue
		}
		out = append(out, item.ask)
	}
	return out
}

func (rt *Runtime) lookupAsk(requestID string) (sessionID string, st *sessionMem) {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	for id, mem := range rt.mem {
		mem.mu.Lock()
		_, ok := mem.asks[requestID]
		mem.mu.Unlock()
		if ok {
			return id, mem
		}
	}
	return "", nil
}

// Decide 记下作答并回给 Codex。resolved 之后的迟到作答会被拒绝。
func (rt *Runtime) Decide(ctx context.Context, requestID string, answer pkg.AskAnswer) error {
	sessionID, st := rt.lookupAsk(requestID)
	if st == nil {
		return cderr.NotFound("ask %s", requestID)
	}
	st.mu.Lock()
	item := st.asks[requestID]
	if item == nil {
		st.mu.Unlock()
		return cderr.NotFound("ask %s", requestID)
	}
	if item.resolved {
		st.mu.Unlock()
		return cderr.Conflict("ask %s already resolved", requestID)
	}
	rpcID := item.rpcID
	ask := item.ask
	st.mu.Unlock()

	client, err := rt.ensureClient(ctx)
	if err != nil {
		return err
	}
	if err := client.Reply(ctx, rpcID, pkg.ReplyBody(ask, answer)); err != nil {
		return err
	}

	st.mu.Lock()
	if current := st.asks[requestID]; current != nil {
		current.resolved = true
		if st.active != nil {
			st.active.Status = pkg.TurnRunning
		}
	}
	st.mu.Unlock()
	rt.emit(sessionID, pkg.Event{Type: pkg.EventAskResolved, TurnID: ask.TurnID, Ask: &ask})
	return nil
}

// Expire 按拒绝回给 Codex，免得它一直等。
func (rt *Runtime) Expire(ctx context.Context, requestID string) error {
	return rt.Decide(ctx, requestID, pkg.AskAnswer{Approved: false})
}
