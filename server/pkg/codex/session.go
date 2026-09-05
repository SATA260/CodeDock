package codex

// Session 是一条只走 Codex 的对话。
type Session struct {
	ID           string
	ThreadID     string // Codex thread 编号，首次开回合后才有。
	Title        string
	ActiveTurnID string // 同时只能有一个进行中的回合。
	Archived     bool
}

// Sessions 管走 Codex 的对话容器、Codex thread 编号，以及新建、归档、改标题、分叉。
// 不管 Codex 配置项的值，不管回合怎么跑。
type Sessions struct {
	Codex *Codex
}

// Create 开一条只走 Codex 的对话。刚开出来时还没有 Codex thread，首次开回合才绑上。
// 对话绑死 Codex，中途不能改成本地模型；要换引擎就另开一条。
func (s *Sessions) Create(userID string) (Session, error) {
	return Session{}, nil
}

// Get 读一条对话：它绑的 Codex thread、标题、有没有回合正在跑、归没归档。
func (s *Sessions) Get(sessionID string) (Session, error) {
	return Session{}, nil
}

// BindThread 把 Codex 给的 thread 编号记到这条对话上。只能写一次；
// 这条 thread 以后失效就让回合失败，不静默换一条新的。
func (s *Sessions) BindThread(sessionID, threadID string) error {
	return nil
}

// Archive 归档这条对话，之后不能再向 Codex 开回合。不删本机 Codex 那边的记录。
func (s *Sessions) Archive(sessionID string) error {
	return nil
}

// Rename 改这条对话的标题。
func (s *Sessions) Rename(sessionID, title string) error {
	return nil
}

// Fork 按已落盘历史分叉：新开一条对话，配一条新的 Codex thread，原对话原样不动。
func (s *Sessions) Fork(sessionID string) (Session, error) {
	s.Codex.ForkThread("")
	return Session{}, nil
}

// ClaimActiveTurn 把这一轮标成该对话当前正在执行的回合。一条对话同时只能有一个。
func (s *Sessions) ClaimActiveTurn(sessionID, turnID string) error {
	return nil
}

// ClearActiveTurn 清掉当前执行标记，好让排队里的下一条能被开起来。
func (s *Sessions) ClearActiveTurn(sessionID, turnID string) error {
	return nil
}
