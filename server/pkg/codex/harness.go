package codex

// DefaultClientInfo 返回 CodeDock 握手用的客户端信息。
func DefaultClientInfo() ClientInfo {
	return ClientInfo{Name: ClientName, Title: "CodeDock", Version: ClientVersion}
}

// DefaultCapabilities 声明要用的官方能力。Plan 档走 experimentalApi。
func DefaultCapabilities() map[string]any {
	return map[string]any{"experimentalApi": true}
}

// ApplyTurnOverrides 把当前生效的模型、推理强度和用户改过的权限编进 turn/start。
// 模型/强度必须带上：历史 thread 会记住旧模型，自定义供应商对不上就会被拒。
func ApplyTurnOverrides(params TurnStartParams, settings Settings) TurnStartParams {
	over := settings.Override()
	if model := firstNonEmpty(over.Model, settings.Model); model != "" {
		params.Model = model
	}
	if effort := firstNonEmpty(over.Effort, settings.Effort); effort != "" {
		params.Effort = effort
	}
	if over.ApprovalPolicy != "" {
		params.ApprovalPolicy = over.ApprovalPolicy
	}
	if over.Cwd != "" {
		params.Cwd = over.Cwd
	} else if settings.Cwd != "" && params.Cwd == "" {
		params.Cwd = settings.Cwd
	}
	if over.Sandbox != "" {
		params.SandboxPolicy = SandboxPolicy(over.Sandbox)
	}
	if over.CollaborationMode != "" {
		model := over.Model
		if model == "" {
			model = settings.Model
		}
		params.CollaborationMode = CollaborationModeParams(over.CollaborationMode, model)
	}
	return params
}

// ApplyThreadOverrides 把用户改过的配置编进 thread/start 参数。
func ApplyThreadOverrides(params ThreadStartParams, settings Settings) ThreadStartParams {
	over := settings.Override()
	if over.Model != "" {
		params.Model = over.Model
	}
	if over.ApprovalPolicy != "" {
		params.ApprovalPolicy = over.ApprovalPolicy
	}
	if over.Sandbox != "" {
		params.Sandbox = over.Sandbox
	}
	if over.Cwd != "" {
		params.Cwd = over.Cwd
	} else if settings.Cwd != "" && params.Cwd == "" {
		params.Cwd = settings.Cwd
	}
	return params
}
