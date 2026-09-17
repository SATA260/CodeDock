package main

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// maskJSON 走访 JSON 里的字符串；图片 data 跳过；解析失败当整段文本刮。
func maskJSON(raw json.RawMessage) (json.RawMessage, int) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return raw, 0
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		out, n := maskText(string(raw))
		return json.RawMessage(out), n
	}
	nv, n := walk(v)
	if n == 0 {
		return raw, 0
	}
	body, err := json.Marshal(nv)
	if err != nil {
		return raw, 0
	}
	return body, n
}

// walk 递归处理对象和数组；type=image 的 data 不扫。
func walk(v any) (any, int) {
	switch x := v.(type) {
	case map[string]any:
		n := 0
		image := false
		if typ, ok := x["type"].(string); ok && typ == "image" {
			image = true
		}
		for key, val := range x {
			if image && key == "data" {
				continue
			}
			nv, c := walk(val)
			x[key] = nv
			n += c
		}
		return x, n
	case []any:
		n := 0
		for i, val := range x {
			nv, c := walk(val)
			x[i] = nv
			n += c
		}
		return x, n
	case string:
		return maskText(x)
	default:
		return v, 0
	}
}

// rewriteOutput 刮工具回包：删 fullOutputPath，命中则在最后一个 text 块补脚注。
func rewriteOutput(raw json.RawMessage) json.RawMessage {
	out, n := maskJSON(raw)
	out = dropFullOutputPath(out)
	if n == 0 {
		return out
	}
	return appendFooter(out, n)
}

// dropFullOutputPath 从任意嵌套对象里拿掉 fullOutputPath。
func dropFullOutputPath(raw json.RawMessage) json.RawMessage {
	if len(bytes.TrimSpace(raw)) == 0 {
		return raw
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return raw
	}
	nv, dropped := dropPath(v)
	if !dropped {
		return raw
	}
	body, err := json.Marshal(nv)
	if err != nil {
		return raw
	}
	return body
}

// dropPath 删除对象里的 fullOutputPath，并往下找。
func dropPath(v any) (any, bool) {
	switch x := v.(type) {
	case map[string]any:
		dropped := false
		if _, ok := x["fullOutputPath"]; ok {
			delete(x, "fullOutputPath")
			dropped = true
		}
		for key, val := range x {
			nv, d := dropPath(val)
			x[key] = nv
			dropped = dropped || d
		}
		return x, dropped
	case []any:
		dropped := false
		for i, val := range x {
			nv, d := dropPath(val)
			x[i] = nv
			dropped = dropped || d
		}
		return x, dropped
	default:
		return v, false
	}
}

// appendFooter 在最后一个 type=text 的块末尾写 masked 条数。
func appendFooter(raw json.RawMessage, n int) json.RawMessage {
	var root map[string]any
	if err := json.Unmarshal(raw, &root); err != nil {
		return raw
	}
	content, ok := root["content"].([]any)
	if !ok {
		return raw
	}
	note := fmt.Sprintf("\n[redact] masked %d secrets", n)
	for i := len(content) - 1; i >= 0; i-- {
		block, ok := content[i].(map[string]any)
		if !ok {
			continue
		}
		if typ, _ := block["type"].(string); typ != "text" {
			continue
		}
		text, _ := block["text"].(string)
		block["text"] = text + note
		content[i] = block
		root["content"] = content
		body, err := json.Marshal(root)
		if err != nil {
			return raw
		}
		return body
	}
	return raw
}
