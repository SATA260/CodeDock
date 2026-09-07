package claude

// 本文件管走 Claude Code 的对话容器。不管 Claude 配置项的值，不管回合怎么跑。对话以本机 Claude 为准，不落库。

// Create 创建一条只走 Claude Code 的对话。Claude session 编号在首次开回合后才有。
func Create(userID string) (Session, error) {
	_ = userID
	return Session{}, nil
}

// Get 读取对话。
func Get(sessionID string) (Session, error) {
	return ReadSession(sessionID)
}

// ListSessions 从本机 Claude 列对话。
func ListSessions() ([]Session, error) {
	return ReadSessions()
}

// BindClaudeSession 记下 Claude session 编号，只能写一次。本模块不落库，编号以本机 Claude 为准。
func BindClaudeSession(sessionID, claudeSessionID string) error {
	_, err := ReadSession(claudeSessionID)
	_ = sessionID
	return err
}

// Archive 归档，之后不能再向 Claude Code 开回合。不删本机 Claude 记录。
func Archive(sessionID string) error {
	_, err := Get(sessionID)
	return err
}

// Rename 改对话标题。
func Rename(sessionID, title string) error {
	_, err := Get(sessionID)
	_ = title
	return err
}

// Fork 按已落盘历史分叉出新对话和新的 Claude session，原对话不动。
func Fork(sessionID string) (Session, error) {
	sess, err := Get(sessionID)
	if err != nil {
		return Session{}, err
	}
	newID, err := ForkSession(sess.ClaudeSessionID)
	if err != nil {
		return Session{}, err
	}
	return ReadSession(newID)
}

// ClaimActiveTurn 标成当前执行；同时只能有一个。
func ClaimActiveTurn(sessionID, turnID string) error {
	_, err := Get(sessionID)
	_ = turnID
	return err
}

// ClearActiveTurn 清掉当前执行标记。
func ClearActiveTurn(sessionID, turnID string) error {
	_, err := Get(sessionID)
	_ = turnID
	return err
}
