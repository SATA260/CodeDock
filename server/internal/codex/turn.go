package codex

import (
	"context"

	cderr "codedock/internal/errors"
	"codedock/internal/util"
	pkg "codedock/pkg/codex"
)

// StartTurn 发出这条输入。空闲就开一轮，正忙则排队。
func (rt *Runtime) StartTurn(ctx context.Context, sessionID, content string, extra pkg.Input, mode pkg.InputMode) (pkg.Turn, error) {
	if sessionID == "" {
		return pkg.Turn{}, cderr.Invalid("session_id is required")
	}
	st := rt.state(sessionID)
	st.mu.Lock()
	if st.archived {
		st.mu.Unlock()
		return pkg.Turn{}, cderr.Invalid("archived session cannot start a turn")
	}
	busy := st.active != nil && (st.active.Status == pkg.TurnRunning || st.active.Status == pkg.TurnWaitingApproval || st.active.Status == pkg.TurnQueued)
	st.mu.Unlock()

	input := rt.takeDraft(sessionID, content, extra)
	turn := pkg.Turn{ID: util.NewID(), SessionID: sessionID, Status: pkg.TurnQueued}
	if busy || mode == pkg.InputQueue {
		st.mu.Lock()
		st.queue = append(st.queue, queuedTurn{turn: turn, input: input})
		st.mu.Unlock()
		rt.emit(sessionID, pkg.Event{Type: pkg.EventTurnQueued, TurnID: turn.ID, Turn: &turn})
		return turn, nil
	}
	return rt.dispatchTurn(ctx, sessionID, turn, input)
}

func (rt *Runtime) dispatchTurn(ctx context.Context, sessionID string, turn pkg.Turn, input pkg.Input) (pkg.Turn, error) {
	client, err := rt.requireReady(ctx)
	if err != nil {
		turn.Status = pkg.TurnFailed
		turn.Error = err.Error()
		rt.emit(sessionID, pkg.Event{Type: pkg.EventTurnFailed, TurnID: turn.ID, Turn: &turn, Notice: turn.Error})
		return turn, err
	}
	settings, err := rt.Effective(ctx, sessionID)
	if err != nil {
		st := rt.state(sessionID)
		st.mu.Lock()
		settings = st.settings
		st.mu.Unlock()
	}
	st := rt.state(sessionID)
	st.mu.Lock()
	turn.Status = pkg.TurnRunning
	copyTurn := turn
	st.active = &copyTurn
	st.mu.Unlock()

	params := pkg.ApplyTurnOverrides(pkg.TurnStartParams{
		ThreadID: sessionID,
		Input:    pkg.UserInputs(input),
		Cwd:      settings.Cwd,
	}, settings)
	if err := rt.ensureResumed(ctx, client, sessionID); err != nil && !alreadyOpen(err) {
		st.mu.Lock()
		if st.active != nil && st.active.ID == turn.ID {
			st.active.Status = pkg.TurnFailed
			st.active.Error = err.Error()
			failed := *st.active
			st.active = nil
			st.mu.Unlock()
			rt.emit(sessionID, pkg.Event{Type: pkg.EventTurnFailed, TurnID: failed.ID, Turn: &failed, Notice: failed.Error})
			return failed, mapRPC(err)
		}
		st.mu.Unlock()
		return turn, mapRPC(err)
	}
	res, err := client.TurnStart(ctx, params)
	if err != nil && threadUnavailable(err) {
		if rerr := rt.ensureResumed(ctx, client, sessionID); rerr == nil {
			res, err = client.TurnStart(ctx, params)
		}
	}
	if err != nil {
		st.mu.Lock()
		if st.active != nil && st.active.ID == turn.ID {
			st.active.Status = pkg.TurnFailed
			st.active.Error = err.Error()
			failed := *st.active
			st.active = nil
			st.mu.Unlock()
			rt.emit(sessionID, pkg.Event{Type: pkg.EventTurnFailed, TurnID: failed.ID, Turn: &failed, Notice: failed.Error})
			return failed, mapRPC(err)
		}
		st.mu.Unlock()
		return turn, mapRPC(err)
	}
	st.mu.Lock()
	if st.active != nil && st.active.ID == turn.ID {
		st.active.CodexID = res.Turn.ID
		st.active.Status = pkg.MapTurnStatus(res.Turn.Status, len(st.asks) > 0)
		turn = *st.active
	}
	st.mu.Unlock()
	rt.emit(sessionID, pkg.Event{Type: pkg.EventTurnStarted, TurnID: turn.ID, Turn: &turn, Progress: &pkg.Progress{Kind: pkg.ProgressUser, Text: input.Text}})
	return turn, nil
}

// CancelTurn 手动打断当前回合，或从队列里拿掉还没开的那条。
func (rt *Runtime) CancelTurn(ctx context.Context, sessionID, turnID string) error {
	if turnID == "" {
		return cderr.Invalid("turn_id is required")
	}
	st := rt.state(sessionID)
	st.mu.Lock()
	if st.active != nil && st.active.ID == turnID {
		codexID := st.active.CodexID
		st.mu.Unlock()
		if codexID == "" {
			st.mu.Lock()
			st.active.Status = pkg.TurnCancelled
			done := *st.active
			st.active = nil
			next := queuedTurn{}
			hasNext := false
			if len(st.queue) > 0 {
				next = st.queue[0]
				st.queue = st.queue[1:]
				hasNext = true
			}
			st.mu.Unlock()
			rt.emit(sessionID, pkg.Event{Type: pkg.EventTurnCancelled, TurnID: done.ID, Turn: &done})
			if hasNext {
				_, _ = rt.dispatchTurn(ctx, sessionID, next.turn, next.input)
			}
			return nil
		}
		client, err := rt.ensureClient(ctx)
		if err != nil {
			return err
		}
		return mapRPC(client.TurnInterrupt(ctx, sessionID, codexID))
	}
	for i, item := range st.queue {
		if item.turn.ID == turnID {
			st.queue = append(st.queue[:i], st.queue[i+1:]...)
			item.turn.Status = pkg.TurnCancelled
			st.mu.Unlock()
			rt.emit(sessionID, pkg.Event{Type: pkg.EventTurnCancelled, TurnID: item.turn.ID, Turn: &item.turn})
			return nil
		}
	}
	st.mu.Unlock()
	return cderr.NotFound("turn %s", turnID)
}

// Compact 让 Codex 压缩这条 thread。
func (rt *Runtime) Compact(ctx context.Context, sessionID string) error {
	client, err := rt.requireReady(ctx)
	if err != nil {
		return err
	}
	if err := rt.ensureResumed(ctx, client, sessionID); err != nil {
		return mapRPC(err)
	}
	return mapRPC(client.ThreadCompact(ctx, sessionID))
}

// Review 让 Codex 评审工作目录改动。
func (rt *Runtime) Review(ctx context.Context, sessionID string) error {
	client, err := rt.requireReady(ctx)
	if err != nil {
		return err
	}
	if err := rt.ensureResumed(ctx, client, sessionID); err != nil {
		return mapRPC(err)
	}
	return mapRPC(client.ReviewStart(ctx, sessionID))
}
