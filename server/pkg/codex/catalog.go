package codex

import "slices"

// EngineStatus 是本机 Codex 能不能用的体检结果。
type EngineStatus struct {
	Available  bool   `json:"available"`  // 本机装没装 Codex。
	Authorized bool   `json:"authorized"` // 有没有取得 Codex 授权；没授权可以看选项，但不能开回合。
	Version    string `json:"version,omitempty"`
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

// MergeConfiguredModel 把 config/read 里当前模型并进 model/list。
// 自定义供应商的模型通常不在官方目录里，选择器仍要能选到。
func MergeConfiguredModel(models []ModelInfo, cfg ConfigReadResult) []ModelInfo {
	id := ConfigString(cfg, "model")
	if id == "" {
		return models
	}
	effort := ConfigString(cfg, "model_reasoning_effort")
	out := append([]ModelInfo(nil), models...)
	for i := range out {
		if out[i].ID != id {
			continue
		}
		out[i].IsDefault = true
		if effort != "" && !slices.Contains(out[i].Efforts, effort) {
			out[i].Efforts = append(append([]string{}, out[i].Efforts...), effort)
		}
		if out[i].DefaultEffort == "" {
			out[i].DefaultEffort = effort
		}
		for j := range out {
			if j != i {
				out[j].IsDefault = false
			}
		}
		return out
	}
	info := ModelInfo{
		ID:            id,
		DisplayName:   id,
		Efforts:       customModelEfforts(effort),
		DefaultEffort: firstNonEmpty(effort, "medium"),
		IsDefault:     true,
	}
	for i := range out {
		out[i].IsDefault = false
	}
	return append([]ModelInfo{info}, out...)
}

func customModelEfforts(effort string) []string {
	out := []string{"low", "medium", "high", "xhigh"}
	if effort != "" && !slices.Contains(out, effort) {
		out = append(out, effort)
	}
	return out
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
