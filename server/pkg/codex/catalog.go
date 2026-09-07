package codex

// EngineStatus 是本机 Codex 能不能用的体检结果。
type EngineStatus struct {
	Available  bool   `json:"available"`  // 本机装没装 Codex。
	Authorized bool   `json:"authorized"` // 有没有取得 Codex 授权；没授权可以看选项，但不能开回合。
	Version    string `json:"version"`
	Hint       string `json:"hint,omitempty"` // 不可用时给人看的原因，如未安装或未授权。
}

// ModelInfo 是一条 Codex 模型及其支持的推理强度。
type ModelInfo struct {
	ID            string   `json:"id"`
	DisplayName   string   `json:"display_name,omitempty"`
	Efforts       []string `json:"efforts"` // 该 Codex 模型支持的推理强度。
	DefaultEffort string   `json:"default_effort"`
	Hidden        bool     `json:"hidden"` // Codex 自己不在选择器里列出来的模型。
	IsDefault     bool     `json:"is_default"`
}

// ModeInfo 是一条 Codex 的 Plan 或权限预设。
type ModeInfo struct {
	ID       string `json:"id"` // Codex 的 Plan 或权限预设名，如 plan、read-only。
	Label    string `json:"label,omitempty"`
	Kind     string `json:"kind"`               // collaboration | permission
	Approval string `json:"approval,omitempty"` // Codex 的 approval 值；Plan 可空。
	Sandbox  string `json:"sandbox,omitempty"`  // Codex 的 sandbox 值；Plan 可空。
	Allowed  bool   `json:"allowed"`            // 当前环境是否允许选这一档。
}
