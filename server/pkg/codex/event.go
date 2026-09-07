package codex

// EventType 是本模块对外的短事件名。
type EventType string

const (
	EventTurnQueued    EventType = "turn.queued"
	EventTurnStarted   EventType = "turn.started"
	EventTurnCompleted EventType = "turn.completed"
	EventTurnFailed    EventType = "turn.failed"
	EventTurnCancelled EventType = "turn.cancelled"
	EventProgress      EventType = "progress"
	EventAskRequired   EventType = "ask.required"
	EventAskResolved   EventType = "ask.resolved"
	EventNotice        EventType = "notice"
	EventReset         EventType = "reset"
)

// Event 是给 HTTP/SSE 的一条 Codex 领域事件。只存在当前进程里。
type Event struct {
	Seq       int64        `json:"seq"`
	Type      EventType    `json:"type"`
	SessionID string       `json:"session_id"`
	TurnID    string       `json:"turn_id,omitempty"`
	Progress  *Progress    `json:"progress,omitempty"`
	Turn      *Turn        `json:"turn,omitempty"`
	Ask       *ApprovalAsk `json:"ask,omitempty"`
	Notice    string       `json:"notice,omitempty"`
}
