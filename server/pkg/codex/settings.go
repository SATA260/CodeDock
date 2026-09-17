package codex

// Settings 是这条对话里最终生效的 Codex 模型、推理强度、Plan 与权限。
type Settings struct {
	Model             string   `json:"model,omitempty"`
	Effort            string   `json:"effort,omitempty"`
	CollaborationMode string   `json:"collaboration_mode,omitempty"` // Codex 的 Plan；空表示非 Plan。
	ApprovalPolicy    string   `json:"approval_policy,omitempty"`    // Codex 的值，如 on-request。
	Sandbox           string   `json:"sandbox,omitempty"`            // Codex 的值，如 workspace-write。
	Cwd               string   `json:"cwd,omitempty"`                // 由调用方（看板）传入的工作路径；问答可空。
	Overridden        []string `json:"overridden,omitempty"`         // 用户改过、需要交给 Codex 的字段名。
}

// Override 返回只含用户改过字段的配置，没改的不带给 Codex。
func (s Settings) Override() Settings {
	if len(s.Overridden) == 0 {
		return Settings{Cwd: s.Cwd}
	}
	wanted := map[string]bool{}
	for _, name := range s.Overridden {
		wanted[name] = true
	}
	out := Settings{Cwd: s.Cwd, Overridden: append([]string(nil), s.Overridden...)}
	if wanted["model"] {
		out.Model = s.Model
	}
	if wanted["effort"] {
		out.Effort = s.Effort
	}
	if wanted["collaboration_mode"] {
		out.CollaborationMode = s.CollaborationMode
	}
	if wanted["approval_policy"] {
		out.ApprovalPolicy = s.ApprovalPolicy
	}
	if wanted["sandbox"] {
		out.Sandbox = s.Sandbox
	}
	return out
}

// MergeOverride 把补丁记进当前配置，并登记改过的字段名。
func (s Settings) MergeOverride(patch Settings) Settings {
	out := s
	seen := map[string]bool{}
	for _, name := range out.Overridden {
		seen[name] = true
	}
	add := func(name string) {
		if seen[name] {
			return
		}
		seen[name] = true
		out.Overridden = append(out.Overridden, name)
	}
	if patch.Model != "" {
		out.Model = patch.Model
		add("model")
	}
	if patch.Effort != "" {
		out.Effort = patch.Effort
		add("effort")
	}
	if patch.CollaborationMode != "" {
		out.CollaborationMode = patch.CollaborationMode
		add("collaboration_mode")
	}
	if patch.ApprovalPolicy != "" {
		out.ApprovalPolicy = patch.ApprovalPolicy
		add("approval_policy")
	}
	if patch.Sandbox != "" {
		out.Sandbox = patch.Sandbox
		add("sandbox")
	}
	if patch.Cwd != "" {
		out.Cwd = patch.Cwd
	}
	return out
}
