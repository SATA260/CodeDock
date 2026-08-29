package tool

// ModeCapabilities 返回该运行模式提供的能力。未知模式只给 read 与 memory。
func ModeCapabilities(mode string) []Capability {
	switch mode {
	case "ask", "plan":
		return []Capability{CapabilityRead, CapabilityMemory}
	case "ask_for_approval", "auto_approve", "yolo":
		return []Capability{CapabilityRead, CapabilityWrite, CapabilityMemory}
	default:
		return []Capability{CapabilityRead, CapabilityMemory}
	}
}

// Covers 判断模式提供的能力是否覆盖工具声明的全部能力。工具未声明能力时始终通过。
func Covers(modeCaps, toolCaps []Capability) bool {
	if len(toolCaps) == 0 {
		return true
	}
	allowed := make(map[Capability]struct{}, len(modeCaps))
	for _, cap := range modeCaps {
		allowed[cap] = struct{}{}
	}
	for _, cap := range toolCaps {
		if _, ok := allowed[cap]; !ok {
			return false
		}
	}
	return true
}

// VisibleDefinitions 先按 Agent 绑定的工具名过滤，再按模式能力覆盖过滤。
// names 为空表示未绑定任何工具。
func VisibleDefinitions(all []Definition, names []string, modeCaps []Capability) []Definition {
	if len(names) == 0 {
		return nil
	}
	byName := make(map[string]Definition, len(all))
	for _, def := range all {
		byName[def.Name] = def
	}
	out := make([]Definition, 0, len(names))
	for _, name := range names {
		def, ok := byName[name]
		if !ok {
			continue
		}
		if Covers(modeCaps, def.Permission.Capabilities) {
			out = append(out, def)
		}
	}
	return out
}
