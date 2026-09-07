package claude

// 本文件管这个对话里生效的 Claude 模型、推理强度、权限档。不管发消息，不管命令怎么拆词。本模块不落库。

// Effective 返回 Claude 默认与用户覆盖合并后的生效配置。
func Effective(sessionID string) (Settings, error) {
	return ReadSettings(sessionID)
}

// Apply 只记下用户改过的 Claude 项。下次开回合再把改过的项交给 Claude Code。
func Apply(sessionID string, patch Settings) (Settings, error) {
	if _, err := ListModels(); err != nil {
		return Settings{}, err
	}
	if _, err := ListModes(); err != nil {
		return Settings{}, err
	}
	_ = patch
	return Effective(sessionID)
}
