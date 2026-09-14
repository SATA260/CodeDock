package tool

// VisibleDefinitions 按可执行绑定名过滤工具定义。发给模型用注册表全量，不走这里。
func VisibleDefinitions(all []Definition, names []string) []Definition {
	if len(names) == 0 {
		return nil
	}
	byName := make(map[string]Definition, len(all))
	for _, def := range all {
		byName[def.Name] = def
	}
	out := make([]Definition, 0, len(names))
	for _, name := range names {
		if def, ok := byName[name]; ok {
			out = append(out, def)
		}
	}
	return out
}
