package claude

import "github.com/google/uuid"

// Require 登记一条 Claude Code 已知的反问。
func Require(turnID string, ask ApprovalAsk) (string, error) {
	id := ask.ExternalRequestID
	if id == "" {
		id = uuid.NewString()
	}
	ask.ExternalRequestID = id
	rt.mu.Lock()
	defer rt.mu.Unlock()
	rt.asks[id] = &memAsk{ID: id, TurnID: turnID, Ask: ask}
	if turn, ok := rt.turns[turnID]; ok {
		turn.Status = TurnWaitingApproval
	}
	return id, nil
}

// Decide 按人对已知反问的作答记下结果。
func Decide(approvalID string, answer AskAnswer) error {
	if err := ReplyAsk(approvalID, answer); err != nil {
		return err
	}
	return Continue("")
}

// Expire 过期按拒绝回给 Claude Code，避免死等。
func Expire(approvalID string) error {
	return ReplyAsk(approvalID, AskAnswer{Approved: false})
}
