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
	ID        string
	SessionID string
	Status    TurnStatus
}

// Turns 管一轮 Codex 工作的开始、排队和手动打断。进行中再发下一条只排队、不自动插话；
// 要停掉当前这轮只能手动打断。不管实录怎么记，不管人怎么点批准。
type Turns struct {
	Session    *Sessions
	Codex      *Codex
	Transcript *Transcript
	Attachment *Attachment
}

// Start 发出这条输入。对话空闲就向 Codex 开新一轮：取出攒好的附件、占住当前执行位、
// 首次还要开 thread 并把编号记回对话，再把用户这条写进实录。
// 对话正忙则按排队处理，不打断在跑的那一轮。
func (t *Turns) Start(sessionID, content string, input Input, mode InputMode) (turnID string, err error) {
	t.Attachment.Mention(sessionID, "")
	t.Session.ClaimActiveTurn(sessionID, "")
	t.Codex.StartThread("", Settings{})
	t.Session.BindThread(sessionID, "")
	t.Codex.StartTurn("", input, Settings{})
	t.Transcript.AppendUser(sessionID, "", content, input)
	return "", nil
}

// Queue 把这条输入排到当前这轮后面，先写进实录给人看；
// 等当前一轮自己结束或被手动打断，再轮到它开。
func (t *Turns) Queue(sessionID, content string, input Input) (turnID string, err error) {
	t.Transcript.AppendUser(sessionID, "", content, input)
	return "", nil
}

// Cancel 按用户请求手动打断当前这一轮：向 Codex 传播取消、腾出当前执行位，
// 再把排着的下一条开起来。
func (t *Turns) Cancel(turnID string) error {
	t.Codex.Interrupt("", turnID)
	t.Session.ClearActiveTurn("", turnID)
	t.Start("", "", Input{}, InputStart)
	return nil
}

// Continue 在反问有了结果后让这一轮接着跑，不新开回合。
func (t *Turns) Continue(turnID string) error {
	return nil
}
