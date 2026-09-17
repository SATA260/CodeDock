package claude

// Effective 返回 Claude 默认与用户覆盖合并后的生效配置。
func Effective(sessionID string) (Settings, error) {
	base, err := ReadSettings(sessionID)
	if err != nil {
		return Settings{}, err
	}
	if sessionID == "" {
		return base, nil
	}
	rt.mu.Lock()
	defer rt.mu.Unlock()
	sess := internLocked(sessionID)
	if sess.Overrides.Model != "" {
		base.Model = sess.Overrides.Model
	}
	if sess.Overrides.Effort != "" {
		base.Effort = sess.Overrides.Effort
	}
	if sess.Overrides.PermissionMode != "" {
		base.PermissionMode = sess.Overrides.PermissionMode
	}
	if sess.Overrides.Cwd != "" {
		base.Cwd = sess.Overrides.Cwd
	}
	base.Overridden = append([]string{}, sess.Overridden...)
	if base.Overridden == nil {
		base.Overridden = emptyStrings()
	}
	if base.Cwd == "" {
		base.Cwd = defaultCwd()
	}
	return base, nil
}

// Apply 只记下用户改过的 Claude 项。下次开回合再把改过的项交给 Claude Code。
func Apply(sessionID string, patch Settings) (Settings, error) {
	models, err := ListModels()
	if err != nil {
		return Settings{}, err
	}
	modes, err := ListModes()
	if err != nil {
		return Settings{}, err
	}
	if patch.Model != "" && !knownModel(models, patch.Model) {
		return Settings{}, wrapErr(errInvalid, "unknown model %s", patch.Model)
	}
	if patch.PermissionMode != "" {
		patch.PermissionMode = canonicalPermissionMode(patch.PermissionMode)
		if !knownMode(modes, patch.PermissionMode) {
			return Settings{}, wrapErr(errInvalid, "unknown permission mode %s", patch.PermissionMode)
		}
	}
	if patch.Model != "" && patch.Effort != "" && !knownEffort(models, patch.Model, patch.Effort) {
		return Settings{}, wrapErr(errInvalid, "unknown effort %s", patch.Effort)
	}
	if sessionID == "" {
		return Settings{}, wrapErr(errInvalid, "session_id is required")
	}
	rt.mu.Lock()
	sess := internLocked(sessionID)
	if patch.Model != "" {
		sess.Overrides.Model = patch.Model
		sess.Overridden = addOverride(sess.Overridden, "model")
	}
	if patch.Effort != "" {
		sess.Overrides.Effort = patch.Effort
		sess.Overridden = addOverride(sess.Overridden, "effort")
	}
	if patch.PermissionMode != "" {
		sess.Overrides.PermissionMode = patch.PermissionMode
		sess.Overridden = addOverride(sess.Overridden, "permission_mode")
	}
	if patch.Cwd != "" {
		sess.Overrides.Cwd = patch.Cwd
		sess.Overridden = addOverride(sess.Overridden, "cwd")
	}
	rt.mu.Unlock()
	return Effective(sessionID)
}

func knownModel(models []ModelInfo, id string) bool {
	for _, model := range models {
		if model.ID == id {
			return true
		}
	}
	return false
}

// canonicalPermissionMode 把官方别名收成 --permission-mode 用的 id。
func canonicalPermissionMode(id string) string {
	if id == "manual" {
		return "default"
	}
	return id
}

func knownMode(modes []ModeInfo, id string) bool {
	id = canonicalPermissionMode(id)
	for _, mode := range modes {
		if mode.ID == id {
			return true
		}
	}
	return false
}

func knownEffort(models []ModelInfo, modelID, effort string) bool {
	for _, model := range models {
		if model.ID != modelID {
			continue
		}
		for _, item := range model.Efforts {
			if item == effort {
				return true
			}
		}
	}
	return false
}

func addOverride(names []string, name string) []string {
	for _, item := range names {
		if item == name {
			return names
		}
	}
	return append(names, name)
}
