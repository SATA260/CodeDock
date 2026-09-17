package plugin

import "encoding/json"

// MaxPluginContextBytes 是宿主接受的袋子上限；超限则丢掉这次改动、保留上一份。
const MaxPluginContextBytes = 8 << 10

// PluginContext 是插件之间共享的参数袋。不进模型、不进消息表、不换向。
// 键建议写成 plugin.field，避免互相覆盖。宿主按会话暂存，有 Run 后挂到该 Run；进程重启即丢。
type PluginContext struct {
	Values map[string]json.RawMessage // 插件自约定的键值，值是 JSON
}

// DecodePluginContext 把信封上的 JSON 对象解成袋子；空或非法时得到空 Values。
func DecodePluginContext(raw json.RawMessage) PluginContext {
	values := map[string]json.RawMessage{}
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &values)
	}
	if values == nil {
		values = map[string]json.RawMessage{}
	}
	return PluginContext{Values: values}
}

// Raw 把袋子编成 JSON 对象；空袋是 {}。
func (c PluginContext) Raw() json.RawMessage {
	if len(c.Values) == 0 {
		return json.RawMessage(`{}`)
	}
	body, err := json.Marshal(c.Values)
	if err != nil {
		return json.RawMessage(`{}`)
	}
	return body
}

// Clone 复制一份袋子，后续 Set 不会改到原来那份。
func (c PluginContext) Clone() PluginContext {
	out := PluginContext{Values: make(map[string]json.RawMessage, len(c.Values))}
	for key, value := range c.Values {
		out.Values[key] = append(json.RawMessage(nil), value...)
	}
	return out
}

// Set 写入一个键；value 按 JSON 编码。空键或编码失败则忽略。
func (c *PluginContext) Set(key string, value any) {
	if c == nil || key == "" {
		return
	}
	if c.Values == nil {
		c.Values = map[string]json.RawMessage{}
	}
	body, err := json.Marshal(value)
	if err != nil {
		return
	}
	c.Values[key] = body
}

// Get 读取一个键的 JSON；没有则 nil。
func (c PluginContext) Get(key string) json.RawMessage {
	if c.Values == nil {
		return nil
	}
	return c.Values[key]
}

// AcceptableContext 判断 raw 是否可作为袋子保存：空、{} 或未超限的 JSON 对象。
func AcceptableContext(raw json.RawMessage) bool {
	if len(raw) == 0 {
		return true
	}
	if len(raw) > MaxPluginContextBytes {
		return false
	}
	var values map[string]json.RawMessage
	return json.Unmarshal(raw, &values) == nil
}
