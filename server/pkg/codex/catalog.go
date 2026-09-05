package codex

// EngineStatus 是本机 Codex 能不能用的体检结果。
type EngineStatus struct {
	Available  bool // 本机装没装 Codex。
	Authorized bool // 有没有取得 Codex 授权；没授权可以看选项，但不能开回合。
	Version    string
	Hint       string // 不可用时给人看的原因，如未安装或未授权。
}

// ModelInfo 是一条 Codex 模型及其支持的推理强度。
type ModelInfo struct {
	ID            string
	Efforts       []string // 该 Codex 模型支持的推理强度。
	DefaultEffort string
	Hidden        bool // Codex 自己不在选择器里列出来的模型。
	IsDefault     bool
}

// ModeInfo 是一条 Codex 的 Plan 或权限预设。
type ModeInfo struct {
	ID       string // Codex 的 Plan 或权限预设名，如 auto、read-only、full-access。
	Kind     string // collaboration | permission
	Approval string // Codex 的 approval 值；Plan 可空。
	Sandbox  string // Codex 的 sandbox 值；Plan 可空。
}

// Catalog 管本机有没有 Codex、是否已取得授权、允许选哪些模型与模式。不管对话，不管开回合。
type Catalog struct {
	Session  *Sessions
	Settings *Configs
}

// Probe 是「开一条 Codex 对话」的入口：先体检本机装没装 Codex、有没有取得授权，
// 再把可选的模型与模式、这条对话本身和它的生效配置一并备好。
// 未取得授权时只回 Hint 说明原因，这条对话不该开回合。
func (c *Catalog) Probe() (EngineStatus, error) {
	c.ListModels()
	c.ListModes()
	c.Session.Create("")
	c.Settings.Effective("")
	return EngineStatus{}, nil
}

// ListModels 列出 Codex 允许选的模型，以及每个模型支持哪些推理强度、默认用哪一档。
// 名单来自 Codex，本模块不自造模型名，也不自造强度档位。
func (c *Catalog) ListModels() ([]ModelInfo, error) {
	return nil, nil
}

// ListModes 列出 Codex 的 Plan 与权限预设（如 auto、read-only、full-access），
// 连带各自对应的 approval 与 sandbox 值，供人在对话里切换。不自造官方没有的档。
func (c *Catalog) ListModes() ([]ModeInfo, error) {
	return nil, nil
}
