package claude

// 本文件管本机有没有 Claude Code、是否已取得 Claude 授权、允许选哪些模型与权限档。不管对话，不管开回合。

// Probe 查看本机 Claude Code 是否可用、是否已取得授权。
func Probe() (EngineStatus, error) {
	return ReadEngine()
}

// ListModels 列出 Claude Code 模型及各自支持的推理强度。
func ListModels() ([]ModelInfo, error) {
	return ReadModels()
}

// ListModes 列出 Claude Code 的官方权限档。
func ListModes() ([]ModeInfo, error) {
	return ReadModes()
}
