package codex

// DefaultClientInfo 返回 CodeDock 握手用的客户端信息。
func DefaultClientInfo() ClientInfo {
	return ClientInfo{Name: ClientName, Title: "CodeDock", Version: ClientVersion}
}

// ApplyTurnOverrides 把用户改过的配置编进 turn/start 参数。
func ApplyTurnOverrides(params TurnStartParams, settings Settings) TurnStartParams {
	over := settings.Override()
	if over.Model != "" {
		params.Model = over.Model
	}
	if over.Effort != "" {
		params.Effort = over.Effort
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
