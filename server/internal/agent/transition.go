package agent

import (
	"context"

	cderr "codedock/internal/errors"
	pkgagent "codedock/pkg/agent"
	"codedock/internal/util"
)

// Transition 消费模型流，把增量先落库再发到事件总线，并返回最终结果。
// 同时监听 ctx.Done()，避免 hang 流在取消时卡住。
func (r *Runtime) Transition(ctx context.Context, run pkgagent.Run, turn pkgagent.Turn, messageID string, stream pkgagent.ModelStream) (pkgagent.ModelStreamResult, error) {
	if stream == nil {
		return pkgagent.ModelStreamResult{}, cderr.Unavailable("model stream is nil")
	}
	defer stream.Close()

	if _, err := r.AppendEvent(ctx, pkgagent.AgentEvent{
		SessionID: run.SessionID,
		RunID:     run.ID,
		TurnID:    &turn.ID,
		Type:      pkgagent.EventAssistantStarted,
		Payload:   pkgagent.MarshalPayload(pkgagent.AssistantStartedPayload{MessageID: messageID}),
	}); err != nil {
		return pkgagent.ModelStreamResult{}, err
	}

	events := stream.Events()
	for {
		select {
		case <-ctx.Done():
			_ = stream.Close()
			return pkgagent.ModelStreamResult{}, ctx.Err()
		case event, ok := <-events:
			if !ok {
				result, err := stream.Result(ctx)
				if err != nil {
					return pkgagent.ModelStreamResult{}, err
				}
				if result.Message.ID == "" {
					result.Message.ID = messageID
				}
				if result.Message.SessionID == "" {
					result.Message.SessionID = run.SessionID
				}
				if result.Message.CreatedAt.IsZero() {
					result.Message.CreatedAt = util.Now()
				}
				return result, nil
			}
			if event.Type != pkgagent.ModelStreamTextDelta && event.Type != pkgagent.ModelStreamToolDelta {
				continue
			}
			if _, err := r.AppendEvent(ctx, pkgagent.AgentEvent{
				SessionID: run.SessionID,
				RunID:     run.ID,
				TurnID:    &turn.ID,
				Type:      pkgagent.EventAssistantDelta,
				Payload: pkgagent.MarshalPayload(pkgagent.AssistantDeltaPayload{
					MessageID: messageID,
					Delta:     event.Delta,
				}),
			}); err != nil {
				return pkgagent.ModelStreamResult{}, err
			}
		}
	}
}
