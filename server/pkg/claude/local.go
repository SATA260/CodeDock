package claude

// 本文件从本机 Claude 读数据。会话、实录、模型、权限档和配置都不落本模块的库。

// ReadEngine 从本机 Claude 读是否可用、是否已取得授权。
func ReadEngine() (EngineStatus, error) {
	return EngineStatus{}, nil
}

// ReadModels 从本机 Claude 读模型及各自支持的推理强度。
func ReadModels() ([]ModelInfo, error) {
	return []ModelInfo{}, nil
}

// ReadModes 从本机 Claude 读官方权限档。
func ReadModes() ([]ModeInfo, error) {
	return []ModeInfo{}, nil
}

// ReadSession 从本机 Claude 读一条 session。
func ReadSession(claudeSessionID string) (Session, error) {
	_ = claudeSessionID
	return Session{}, nil
}

// ReadSessions 从本机 Claude 读对话列表。
func ReadSessions() ([]Session, error) {
	return []Session{}, nil
}

// ReadTranscript 从本机 Claude 读给人看的实录。
func ReadTranscript(claudeSessionID string) ([]Progress, error) {
	_ = claudeSessionID
	return []Progress{}, nil
}

// ReadSettings 从本机 Claude 读生效配置。
func ReadSettings(claudeSessionID string) (Settings, error) {
	_ = claudeSessionID
	return Settings{Overridden: []string{}}, nil
}
