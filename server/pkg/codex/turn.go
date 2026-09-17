package codex

// InputMode 是这条输入进来时，对当前进行中回合怎么处理。
type InputMode string

const (
	InputStart InputMode = "start" // 对话空闲，直接开一轮。
	InputQueue InputMode = "queue" // 对话正忙，排队等它结束或被手动打断。
)

// TurnStatus 是一轮 Codex 工作的状态。
type TurnStatus string

const (
	TurnQueued          TurnStatus = "queued"
	TurnRunning         TurnStatus = "running"
	TurnWaitingApproval TurnStatus = "waiting_approval"
	TurnCompleted       TurnStatus = "completed"
	TurnFailed          TurnStatus = "failed"
	TurnCancelled       TurnStatus = "cancelled"
)

// Turn 是一次用户请求对应的那一轮 Codex 工作。
type Turn struct {
	ID        string     `json:"id"`
	SessionID string     `json:"session_id"`
	CodexID   string     `json:"codex_id,omitempty"` // 官方 turn id；排队中还没有。
	Status    TurnStatus `json:"status"`
	Error     string     `json:"error,omitempty"`
}

// MapTurnStatus 把官方 turn.status 映到本模块状态。
func MapTurnStatus(status string, waitingAsk bool) TurnStatus {
	if waitingAsk && (status == "" || status == "inProgress") {
		return TurnWaitingApproval
	}
	switch status {
	case "completed":
		return TurnCompleted
	case "interrupted":
		return TurnCancelled
	case "failed":
		return TurnFailed
	case "inProgress", "":
		return TurnRunning
	default:
		return TurnRunning
	}
}
